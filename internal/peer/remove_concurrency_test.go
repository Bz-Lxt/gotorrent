package peer_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gotorrent/internal/announce"
	"gotorrent/internal/config"
	"gotorrent/internal/metainfo"
	"gotorrent/internal/peer"
)

func TestControlRemainsResponsiveWhileStoppedAnnouncePending(t *testing.T) {
	stoppedSeen := make(chan struct{})
	releaseStopped := make(chan struct{})
	var stoppedOnce sync.Once
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseStopped) })
	}

	trackerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("event") == "stopped" {
			stoppedOnce.Do(func() { close(stoppedSeen) })
			<-releaseStopped
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(announce.Response{Interval: 60, Peers: []announce.Peer{}})
	}))
	defer trackerServer.Close()

	root := t.TempDir()
	sourcePath := filepath.Join(root, "fixture.bin")
	if err := os.WriteFile(sourcePath, []byte("control-plane-lock"), 0o644); err != nil {
		t.Fatal(err)
	}
	tf, err := metainfo.Create(sourcePath, trackerServer.URL, "", 4)
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.DefaultPeer()
	cfg.Dir = filepath.Join(root, "downloads")
	p, err := peer.NewWithConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	session, err := p.AddDownload(tf)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		release()
		session.Stop()
	}()

	listArrived := make(chan struct{})
	var listOnce sync.Once
	controlHandler := peer.NewControlServer(p, trackerServer.URL).Handler()
	controlServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/torrents" {
			listOnce.Do(func() { close(listArrived) })
		}
		controlHandler.ServeHTTP(w, r)
	}))
	defer func() {
		release()
		controlServer.Close()
	}()
	client := &http.Client{Timeout: 3 * time.Second}
	defer client.CloseIdleConnections()

	type httpResult struct {
		status int
		body   []byte
		err    error
	}
	do := func(method, url, body string) <-chan httpResult {
		result := make(chan httpResult, 1)
		go func() {
			req, err := http.NewRequest(method, url, strings.NewReader(body))
			if err != nil {
				result <- httpResult{err: err}
				return
			}
			if body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			resp, err := client.Do(req)
			if err != nil {
				result <- httpResult{err: err}
				return
			}
			defer resp.Body.Close()
			data, err := io.ReadAll(resp.Body)
			result <- httpResult{status: resp.StatusCode, body: data, err: err}
		}()
		return result
	}

	removeBody := `{"info_hash":"` + session.InfoHashHex() + `"}`
	removeResult := do(http.MethodPost, controlServer.URL+"/api/remove", removeBody)
	select {
	case <-stoppedSeen:
	case <-time.After(2 * time.Second):
		release()
		t.Fatal("remove did not reach the stopped announce")
	}

	listResult := do(http.MethodGet, controlServer.URL+"/api/torrents", "")
	select {
	case <-listArrived:
	case <-time.After(2 * time.Second):
		release()
		t.Fatal("GET /api/torrents did not reach the control server")
	}
	var listed httpResult
	select {
	case listed = <-listResult:
	case <-time.After(500 * time.Millisecond):
		release()
		<-listResult
		<-removeResult
		t.Fatal("GET /api/torrents blocked behind a pending stopped announce")
	}

	release()
	removed := <-removeResult
	if listed.err != nil {
		t.Fatalf("list torrents: %v", listed.err)
	}
	if listed.status != http.StatusOK {
		t.Fatalf("list torrents status = %d, body = %s", listed.status, listed.body)
	}
	var payload struct {
		Torrents []map[string]any `json:"torrents"`
	}
	if err := json.Unmarshal(listed.body, &payload); err != nil {
		t.Fatalf("decode torrents response: %v", err)
	}
	if len(payload.Torrents) != 0 {
		t.Fatalf("removed torrent remained visible: %+v", payload.Torrents)
	}
	if removed.err != nil {
		t.Fatalf("remove torrent: %v", removed.err)
	}
	if removed.status != http.StatusOK {
		t.Fatalf("remove torrent status = %d, body = %s", removed.status, removed.body)
	}
}
