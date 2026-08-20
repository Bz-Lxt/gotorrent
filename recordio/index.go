package recordio

import (
	"encoding/binary"
	"os"
	"path/filepath"
)

// Index maps a monotonic sequence to a file offset so replay can skip.
type Index struct {
	path string
}

func OpenIndex(dir, name string) (*Index, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Index{path: filepath.Join(dir, name+".idx")}, nil
}

func (x *Index) Append(seq uint64, off int64, kind uint32) error {
	var buf [20]byte
	binary.LittleEndian.PutUint64(buf[0:8], seq)
	binary.LittleEndian.PutUint64(buf[8:16], uint64(off))
	binary.LittleEndian.PutUint32(buf[16:20], kind)
	f, err := os.OpenFile(x.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(buf[:])
	return err
}

type IndexEntry struct {
	Seq  uint64
	Off  int64
	Kind uint32
}

func (x *Index) Load() ([]IndexEntry, error) {
	raw, err := os.ReadFile(x.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(raw)%20 != 0 {
		raw = raw[:len(raw)-len(raw)%20]
	}
	out := make([]IndexEntry, 0, len(raw)/20)
	for i := 0; i+20 <= len(raw); i += 20 {
		out = append(out, IndexEntry{
			Seq:  binary.LittleEndian.Uint64(raw[i : i+8]),
			Off:  int64(binary.LittleEndian.Uint64(raw[i+8 : i+16])),
			Kind: binary.LittleEndian.Uint32(raw[i+16 : i+20]),
		})
	}
	return out, nil
}

func (x *Index) Last() (*IndexEntry, error) {
	all, err := x.Load()
	if err != nil || len(all) == 0 {
		return nil, err
	}
	ev := all[len(all)-1]
	return &ev, nil
}
