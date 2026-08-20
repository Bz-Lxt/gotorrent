// Package tracker 实现轻量级 Tracker：记录每个 Swarm（info_hash）下
// 有哪些 Peer、各 Peer 的进度（做种/下载中），并向 Peer 返回邻居列表。
package tracker

import (
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// PeerInfo 表示 Swarm 中的一个节点。
type PeerInfo struct {
	PeerID     string    `json:"peer_id"`
	IP         string    `json:"ip"`
	Port       int       `json:"port"`
	Uploaded   int64     `json:"uploaded"`
	Downloaded int64     `json:"downloaded"`
	Left       int64     `json:"left"` // 剩余字节，0 表示做种者
	Name       string    `json:"name"` // 节点备注名
	LastSeen   time.Time `json:"last_seen"`
}

// Seeder 判断该节点是否为做种者。
func (p *PeerInfo) Seeder() bool { return p.Left == 0 }

// Swarm 表示一个文件的分发群（同一 info_hash 的所有节点）。
type Swarm struct {
	InfoHash string             `json:"info_hash"`
	Name     string             `json:"name"`
	Length   int64              `json:"length"`
	Peers    map[string]*PeerInfo `json:"peers"` // key: peer_id
}

// Stats 返回做种数与下载数。
func (s *Swarm) Stats() (seeders, leechers int) {
	for _, p := range s.Peers {
		if p.Seeder() {
			seeders++
		} else {
			leechers++
		}
	}
	return
}

// flushTask 是一次 swarm 快照落盘请求。done 在落盘完成后关闭。
type flushTask struct {
	key  string
	done chan struct{}
}

// Tracker 是核心状态：info_hash -> Swarm。
type Tracker struct {
	mu      sync.RWMutex
	swarms  map[string]*Swarm
	peerTTL time.Duration // 超过该时间未 announce 的节点被剔除

	dataDir string        // 为空表示不持久化（纯内存）
	flushCh chan flushTask // 快照落盘任务队列，由 flushLoop 串行消费
}

// New 创建纯内存 Tracker（不持久化）。peerTTL 为节点过期时间。
func New(peerTTL time.Duration) *Tracker {
	t, _ := Open(peerTTL, "")
	return t
}

// Open 创建 Tracker 并按 dataDir 恢复 Swarm 注册表；dataDir 为空则纯内存。
// 启动后每个 Swarm 的元信息（info_hash / 名称 / 长度）会落盘到
// <dataDir>/<infohash>.json，重启后由 Open 重新加载；Peer 列表不持久化，
// 节点重新 announce 即可恢复在线状态。
func Open(peerTTL time.Duration, dataDir string) (*Tracker, error) {
	t := &Tracker{
		swarms:  make(map[string]*Swarm),
		peerTTL: peerTTL,
		dataDir: dataDir,
		flushCh: make(chan flushTask, 256),
	}
	if err := t.load(); err != nil {
		return nil, err
	}
	go t.reaper()
	go t.flushLoop()
	return t, nil
}

// Announce 处理一次节点汇报，返回该 Swarm 中的其他节点（最多 maxPeers 个）。
func (t *Tracker) Announce(infoHash [20]byte, p PeerInfo, name string, length int64, maxPeers int) []*PeerInfo {
	key := hex.EncodeToString(infoHash[:])
	t.mu.Lock()
	defer t.mu.Unlock()

	sw, ok := t.swarms[key]
	if !ok {
		sw = &Swarm{InfoHash: key, Name: name, Length: length, Peers: make(map[string]*PeerInfo)}
		t.swarms[key] = sw
	}
	if sw.Name == "" {
		sw.Name = name
	}
	if sw.Length == 0 {
		sw.Length = length
	}
	p.LastSeen = time.Now()
	selfID := p.PeerID
	sw.Peers[selfID] = &p

	out := make([]*PeerInfo, 0, len(sw.Peers))
	for id, peer := range sw.Peers {
		if id == selfID {
			continue
		}
		out = append(out, peer)
		if len(out) >= maxPeers {
			break
		}
	}

	t.persist(key)
	return out
}

// Leave 处理 stopped 事件，将节点从 Swarm 移除。
func (t *Tracker) Leave(infoHash [20]byte, peerID string) {
	key := hex.EncodeToString(infoHash[:])
	t.mu.Lock()
	sw, ok := t.swarms[key]
	empty := false
	if ok {
		delete(sw.Peers, peerID)
		if len(sw.Peers) == 0 {
			delete(t.swarms, key)
			empty = true
		}
	}
	t.mu.Unlock()

	if empty {
		// Swarm 已空，删除落盘快照使注册表与内存一致。
		t.removeSnapshot(key)
	}
}

// Swarms 返回当前所有 Swarm 的快照（供管理页面使用）。
func (t *Tracker) Swarms() []*Swarm {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]*Swarm, 0, len(t.swarms))
	for _, sw := range t.swarms {
		cp := &Swarm{
			InfoHash: sw.InfoHash,
			Name:     sw.Name,
			Length:   sw.Length,
			Peers:    make(map[string]*PeerInfo, len(sw.Peers)),
		}
		for id, p := range sw.Peers {
			pp := *p
			cp.Peers[id] = &pp
		}
		out = append(out, cp)
	}
	return out
}

// reaper 定期清理超时未汇报的节点。
func (t *Tracker) reaper() {
	ticker := time.NewTicker(t.peerTTL / 2)
	defer ticker.Stop()
	for range ticker.C {
		deadline := time.Now().Add(-t.peerTTL)
		t.mu.Lock()
		for key, sw := range t.swarms {
			for id, p := range sw.Peers {
				if p.LastSeen.Before(deadline) {
					log.Printf("[tracker] 节点 %s (%s) 超时剔除", p.PeerID, p.IP)
					delete(sw.Peers, id)
				}
			}
			if len(sw.Peers) == 0 {
				delete(t.swarms, key)
			}
		}
		t.mu.Unlock()
	}
}

// ---- Swarm 快照持久化 ----

// swarmMeta 是落盘的 Swarm 元信息（不含在线 Peer 列表）。
type swarmMeta struct {
	InfoHash string `json:"info_hash"`
	Name     string `json:"name"`
	Length   int64  `json:"length"`
}

// persist 将一个 Swarm 的元信息落盘，并等待落盘完成。
// 调用方不得持有 t.mu：flushLoop 需要以 RLock 读取一致快照。
func (t *Tracker) persist(key string) {
	if t.dataDir == "" {
		return
	}
	done := make(chan struct{})
	t.flushCh <- flushTask{key: key, done: done}
	<-done
}

// flushLoop 串行消费落盘任务，读取快照并写文件。
func (t *Tracker) flushLoop() {
	for task := range t.flushCh {
		t.writeSnapshot(task.key)
		if task.done != nil {
			close(task.done)
		}
	}
}

// writeSnapshot 以 RLock 读取 Swarm 元信息并写入快照文件。
func (t *Tracker) writeSnapshot(key string) error {
	t.mu.RLock()
	sw, ok := t.swarms[key]
	var meta swarmMeta
	if ok {
		meta = swarmMeta{InfoHash: sw.InfoHash, Name: sw.Name, Length: sw.Length}
	}
	t.mu.RUnlock()
	if !ok {
		return nil
	}
	blob, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(t.snapshotPath(key), append(blob, '\n'), 0o644)
}

// removeSnapshot 删除一个 Swarm 的快照文件（Swarm 清空时调用）。
func (t *Tracker) removeSnapshot(key string) {
	if t.dataDir == "" {
		return
	}
	os.Remove(t.snapshotPath(key))
}

// load 在启动时从 dataDir 恢复 Swarm 注册表。
func (t *Tracker) load() error {
	if t.dataDir == "" {
		return nil
	}
	if err := os.MkdirAll(t.dataDir, 0o755); err != nil {
		return err
	}
	ents, err := os.ReadDir(t.dataDir)
	if err != nil {
		return err
	}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(t.dataDir, e.Name()))
		if err != nil {
			continue
		}
		var meta swarmMeta
		if err := json.Unmarshal(raw, &meta); err != nil || meta.InfoHash == "" {
			continue
		}
		t.swarms[meta.InfoHash] = &Swarm{
			InfoHash: meta.InfoHash,
			Name:     meta.Name,
			Length:   meta.Length,
			Peers:    make(map[string]*PeerInfo),
		}
	}
	return nil
}

func (t *Tracker) snapshotPath(key string) string {
	return filepath.Join(t.dataDir, key+".json")
}
