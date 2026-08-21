package peer_test

import (
	"bytes"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"gotorrent/internal/peer"
	"gotorrent/internal/wire"
)

func TestSeededPeerServesRequestedBlock(t *testing.T) {
	payload := bytes.Repeat([]byte("seed-block-"), 256)
	sourcePath := filepath.Join(t.TempDir(), "release.bin")
	if err := os.WriteFile(sourcePath, payload, 0o644); err != nil {
		t.Fatal(err)
	}

	node, err := peer.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := node.Listen("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	session, err := node.AddSeed(sourcePath, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = node.RemoveSession(session.InfoHashHex()) })

	rawHash, err := hex.DecodeString(session.InfoHashHex())
	if err != nil {
		t.Fatal(err)
	}
	var infoHash [20]byte
	if n := copy(infoHash[:], rawHash); n != len(infoHash) {
		t.Fatalf("info hash length = %d, want %d", n, len(infoHash))
	}

	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(node.Port()))
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}

	hs := &wire.Handshake{InfoHash: infoHash}
	copy(hs.PeerID[:], []byte("peer-test-0000000000"))
	if err := hs.Write(conn); err != nil {
		t.Fatal(err)
	}
	reply, err := wire.ReadHandshake(conn)
	if err != nil {
		t.Fatal(err)
	}
	if reply.InfoHash != infoHash {
		t.Fatalf("handshake info hash = %x, want %x", reply.InfoHash, infoHash)
	}
	if err := (&wire.Message{ID: wire.MsgInterested}).Write(conn); err != nil {
		t.Fatal(err)
	}

	gotBitfield, gotUnchoke := false, false
	for !gotBitfield || !gotUnchoke {
		msg, err := wire.Read(conn)
		if err != nil {
			t.Fatalf("waiting for bitfield and unchoke: %v", err)
		}
		switch msg.ID {
		case wire.MsgBitfield:
			if len(msg.Payload) == 0 || msg.Payload[0]&0x80 == 0 {
				t.Fatalf("seed advertised no first piece: %x", msg.Payload)
			}
			gotBitfield = true
		case wire.MsgUnchoke:
			gotUnchoke = true
		}
	}

	if err := wire.NewRequest(0, 0, len(payload)).Write(conn); err != nil {
		t.Fatal(err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	for {
		msg, err := wire.Read(conn)
		if err != nil {
			t.Fatalf("seed advertised and unchoked but did not serve requested block: %v", err)
		}
		if msg.ID != wire.MsgPiece {
			continue
		}
		index, begin, block, err := msg.ParsePiece()
		if err != nil {
			t.Fatal(err)
		}
		if index != 0 || begin != 0 {
			t.Fatalf("piece coordinates = (%d,%d), want (0,0)", index, begin)
		}
		if !bytes.Equal(block, payload) {
			t.Fatalf("piece payload mismatch: got %d bytes, want %d", len(block), len(payload))
		}
		return
	}
}
