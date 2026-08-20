package peer

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/Bz-Lxt/gotorrent/metainfo"
	"github.com/Bz-Lxt/gotorrent/storage"
	"github.com/Bz-Lxt/gotorrent/wire"
)

const (
	chokeInterval     = 10 * time.Second // Tit-for-Tat 评估周期
	unchokeCount      = 3                // 每周期固定放行数（另有 1 个乐观放行）
	maxOutboundConns  = 30
	defaultAnnInterval = 15
)

// Session 表示一个种子的下载/做种会话。
type Session struct {
	peer  *Peer
	tf    *metainfo.TorrentFile
	store *storage.Store
	picker *piecePicker

	mu       sync.Mutex
	conns    map[*peerConn]struct{}
	seedMode bool // 创建时即为完整文件（做种）
	finished bool // 是否已发送过 completed 事件

	downRate, upRate int64 // 界面展示的速率（字节/秒）
	lastDown, lastUp int64
	lastSample       time.Time

	stopCh   chan struct{}
	stopOnce sync.Once
}

func newSession(p *Peer, tf *metainfo.TorrentFile, store *storage.Store, seedMode bool) *Session {
	return &Session{
		peer: p, tf: tf, store: store,
		picker:     newPiecePicker(tf.NumPieces(), store.Bitfield()),
		conns:      make(map[*peerConn]struct{}),
		seedMode:   seedMode,
		lastSample: time.Now(),
		stopCh:     make(chan struct{}),
	}
}

// InfoHashHex 返回 info_hash 的十六进制表示。
func (s *Session) InfoHashHex() string { return hex.EncodeToString(s.tf.InfoHash[:]) }

// run 启动会话的后台循环：Tracker 汇报、Tit-for-Tat 阻塞算法、速率采样。
func (s *Session) run() {
	go s.announceLoop()
	go s.chokeLoop()
	go s.rateLoop()
}

// Stop 停止会话并断开所有连接。
func (s *Session) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
		s.announce("stopped")
		s.mu.Lock()
		for c := range s.conns {
			c.close()
		}
		s.mu.Unlock()
		s.store.Close()
	})
}

// ---- 连接管理 ----

func (s *Session) addConn(c *peerConn) {
	s.mu.Lock()
	s.conns[c] = struct{}{}
	s.mu.Unlock()
}

func (s *Session) removeConn(c *peerConn) {
	s.mu.Lock()
	delete(s.conns, c)
	s.mu.Unlock()

	c.mu.Lock()
	bf, work := c.peerBF, c.work
	c.mu.Unlock()
	if bf != nil {
		s.picker.RemovePeer(bf)
	}
	if work != nil {
		s.picker.Release(work.index)
	}
}

// numConns 返回当前连接数。
func (s *Session) numConns() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.conns)
}

// finishPiece 在一个分片集齐后校验、落盘并广播 Have。
func (s *Session) finishPiece(work *pieceWork, from *peerConn) {
	if err := s.store.WritePiece(work.index, work.buf); err != nil {
		log.Printf("[session] 分片 %d 校验失败，重新排队: %v", work.index, err)
		s.picker.Release(work.index)
		return
	}
	s.picker.SetOwned(work.index)
	log.Printf("[session] %s 分片 %d/%d 完成", s.tf.Name, s.store.CompletedPieces(), s.tf.NumPieces())

	// 广播 Have
	s.mu.Lock()
	for c := range s.conns {
		c.send(wire.NewHave(work.index))
	}
	s.mu.Unlock()

	if s.store.Complete() {
		s.onDownloadComplete()
	}
}

// onDownloadComplete 下载完成：通知 Tracker、广播 NotInterested、清理续传状态。
func (s *Session) onDownloadComplete() {
	s.mu.Lock()
	if s.finished {
		s.mu.Unlock()
		return
	}
	s.finished = true
	s.mu.Unlock()

	log.Printf("[session] %s 下载完成，转为做种", s.tf.Name)
	s.store.ClearState()
	s.announce("completed")

	s.mu.Lock()
	for c := range s.conns {
		c.mu.Lock()
		interested := c.amInterested
		c.amInterested = false
		c.mu.Unlock()
		if interested {
			c.send(&wire.Message{ID: wire.MsgNotInterested})
		}
	}
	s.mu.Unlock()
}

// ---- Tracker 汇报 ----

func (s *Session) announceLoop() {
	s.announce("started")
	interval := time.Duration(defaultAnnInterval) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			if iv := s.announce(""); iv > 0 {
				interval = time.Duration(iv) * time.Second
				ticker.Reset(interval)
			}
		}
	}
}

// announce 向 Tracker 汇报一次，返回建议的汇报间隔；并把返回的节点交给拨号器。
func (s *Session) announce(event string) int {
	u, err := url.Parse(s.tf.Announce)
	if err != nil {
		return 0
	}
	q := u.Query()
	q.Set("info_hash", s.InfoHashHex())
	q.Set("peer_id", s.peer.ID())
	q.Set("port", strconv.Itoa(s.peer.Port()))
	q.Set("uploaded", strconv.FormatInt(s.store.Uploaded, 10))
	q.Set("downloaded", strconv.FormatInt(s.store.Downloaded, 10))
	q.Set("left", strconv.FormatInt(s.store.BytesLeft(), 10))
	q.Set("name", s.tf.Name)
	q.Set("length", strconv.FormatInt(s.tf.Length, 10))
	if event != "" {
		q.Set("event", event)
	}
	u.RawQuery = q.Encode()

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(u.String())
	if err != nil {
		log.Printf("[session] announce 失败: %v", err)
		return 0
	}
	defer resp.Body.Close()

	var ar struct {
		Interval int `json:"interval"`
		Peers    []struct {
			PeerID string `json:"peer_id"`
			IP     string `json:"ip"`
			Port   int    `json:"port"`
		} `json:"peers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ar); err != nil {
		return 0
	}

	// 下载未完成时才主动外连新节点
	if !s.store.Complete() {
		for _, p := range ar.Peers {
			if s.numConns() >= maxOutboundConns {
				break
			}
			addr := net.JoinHostPort(p.IP, strconv.Itoa(p.Port))
			go s.peer.dialPeer(s, addr)
		}
	}
	return ar.Interval
}

// ---- Tit-for-Tat 阻塞算法 ----

// chokeLoop 每 10 秒评估一次：放行对我们上传最快的 3 个节点，
// 并周期性乐观放行 1 个随机节点（给新节点机会）；其余全部 Choke。
func (s *Session) chokeLoop() {
	ticker := time.NewTicker(chokeInterval)
	defer ticker.Stop()
	round := 0
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.chokeRound(round)
			round++
		}
	}
}

// maybeFastUnchoke 当放行名额未满时立即放行新表示感兴趣的节点，
// 避免新节点空等一个 Tit-for-Tat 周期。
func (s *Session) maybeFastUnchoke(c *peerConn) {
	s.mu.Lock()
	count := 0
	for other := range s.conns {
		if other != c && other.interestedIn() && !other.isChoking() {
			count++
		}
	}
	s.mu.Unlock()
	if count < unchokeCount {
		c.setChoke(false)
	}
}

func (s *Session) chokeRound(round int) {
	s.mu.Lock()
	conns := make([]*peerConn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()

	type cand struct {
		c    *peerConn
		down int64
	}
	var interested []cand
	for _, c := range conns {
		if c.interestedIn() {
			down, _ := c.snapshotRates()
			interested = append(interested, cand{c, down})
		}
	}

	// 按对我们的贡献（下载速率）降序 —— Tit-for-Tat：你对我好，我才对你好
	sort.Slice(interested, func(i, j int) bool { return interested[i].down > interested[j].down })

	unchoked := make(map[*peerConn]bool)
	for i := 0; i < len(interested) && i < unchokeCount; i++ {
		unchoked[interested[i].c] = true
	}

	// 乐观放行：每 3 轮随机选一个被 Choke 的感兴趣节点
	if round%3 == 0 {
		var chokedList []cand
		for _, cd := range interested {
			if !unchoked[cd.c] {
				chokedList = append(chokedList, cd)
			}
		}
		if len(chokedList) > 0 {
			unchoked[chokedList[rand.Intn(len(chokedList))].c] = true
		}
	}

	for _, cd := range interested {
		cd.c.setChoke(!unchoked[cd.c])
	}
}

// rateLoop 每 3 秒采样一次速率供界面展示。
func (s *Session) rateLoop() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			now := time.Now()
			dt := now.Sub(s.lastSample).Seconds()
			if dt <= 0 {
				continue
			}
			s.mu.Lock()
			s.downRate = int64(float64(s.store.Downloaded-s.lastDown) / dt)
			s.upRate = int64(float64(s.store.Uploaded-s.lastUp) / dt)
			s.lastDown = s.store.Downloaded
			s.lastUp = s.store.Uploaded
			s.lastSample = now
			s.mu.Unlock()
		}
	}
}

// ---- 状态快照（供 API） ----

// Snapshot 返回会话的界面展示数据。
func (s *Session) Snapshot() map[string]any {
	s.mu.Lock()
	downRate, upRate := s.downRate, s.upRate
	s.mu.Unlock()

	state := "downloading"
	if s.store.Complete() {
		state = "seeding"
	}
	return map[string]any{
		"info_hash":        s.InfoHashHex(),
		"name":             s.tf.Name,
		"length":           s.tf.Length,
		"piece_length":     s.tf.PieceLength,
		"num_pieces":       s.tf.NumPieces(),
		"completed_pieces": s.store.CompletedPieces(),
		"downloaded":       s.store.Downloaded,
		"uploaded":         s.store.Uploaded,
		"down_rate":        downRate,
		"up_rate":          upRate,
		"peers":            s.numConns(),
		"state":            state,
		"announce":         s.tf.Announce,
	}
}

var _ = fmt.Sprintf // 保留 fmt 引用（调试）
