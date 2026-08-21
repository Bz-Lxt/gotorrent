package eventlog_test

import (
	"testing"

	"gotorrent/internal/eventlog"
)

func TestRecentSnapshotRemainsStable(t *testing.T) {
	log := eventlog.New(4)
	log.Append(eventlog.KindPeer, "download.bin", "peer one connected")
	log.Append(eventlog.KindPiece, "download.bin", "piece two completed")

	snapshot := log.Recent(0)
	if len(snapshot) != 2 {
		t.Fatalf("Recent returned %d events, want 2", len(snapshot))
	}

	log.Append(eventlog.KindPeer, "download.bin", "peer three connected")
	log.Append(eventlog.KindState, "download.bin", "download paused")
	log.Append(eventlog.KindPeer, "download.bin", "peer five connected")

	if snapshot[0].Message != "peer one connected" || snapshot[1].Message != "piece two completed" {
		t.Fatalf("previous Recent result changed after later appends: %#v", snapshot)
	}
}
