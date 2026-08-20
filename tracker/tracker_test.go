package tracker_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Bz-Lxt/gotorrent/tracker"
)

// TestOpenLoadsSnapshots 验证 Open 从数据目录恢复 Swarm 注册表。
func TestOpenLoadsSnapshots(t *testing.T) {
	dir := t.TempDir()
	key := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // 40 位十六进制
	meta := `{"info_hash":"` + key + `","name":"debian.iso","length":314572800}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, key+".json"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}

	tr, err := tracker.Open(90*time.Second, dir)
	if err != nil {
		t.Fatalf("Open 失败: %v", err)
	}
	swarms := tr.Swarms()
	if len(swarms) != 1 {
		t.Fatalf("恢复后 Swarm 数 = %d, 期望 1", len(swarms))
	}
	if swarms[0].Name != "debian.iso" || swarms[0].Length != 314572800 {
		t.Errorf("恢复的元信息不一致: %+v", swarms[0])
	}
}

// TestNewMemoryOnly 验证空 dataDir 的纯内存 Tracker 可正常工作。
func TestNewMemoryOnly(t *testing.T) {
	tr := tracker.New(90 * time.Second)
	if got := tr.Swarms(); len(got) != 0 {
		t.Fatalf("新建 Tracker 应无 Swarm, got %d", len(got))
	}
}
