package recordio

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.rec")
	w, err := Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Append(7, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := w.Sync(); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	fr, err := LastKind(path, 7)
	if err != nil {
		t.Fatal(err)
	}
	if fr == nil || string(fr.Payload) != "hello" {
		t.Fatalf("got %#v", fr)
	}
	if !Exists(path) {
		t.Fatal("missing")
	}
	_ = os.Remove(path)
}
