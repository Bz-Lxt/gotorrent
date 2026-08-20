package recordio

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// SegmentSet is a numbered family of record files in one directory.
type SegmentSet struct {
	mu      sync.Mutex
	dir     string
	prefix  string
	maxBytes int64
	cur     *Writer
	seq     int
}

func OpenSet(dir, prefix string, maxBytes int64) (*SegmentSet, error) {
	if maxBytes <= 0 {
		maxBytes = 4 << 20
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &SegmentSet{dir: dir, prefix: prefix, maxBytes: maxBytes}
	names, err := s.list()
	if err != nil {
		return nil, err
	}
	if len(names) == 0 {
		s.seq = 1
	} else {
		s.seq = seqOf(names[len(names)-1])
	}
	w, err := Create(s.path(s.seq))
	if err != nil {
		return nil, err
	}
	s.cur = w
	return s, nil
}

func (s *SegmentSet) list() ([]string, error) {
	ents, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var names []string
	pre := s.prefix + "-"
	for _, e := range ents {
		name := e.Name()
		if strings.HasPrefix(name, pre) && strings.HasSuffix(name, ".rec") {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func seqOf(name string) int {
	base := strings.TrimSuffix(name, ".rec")
	i := strings.LastIndex(base, "-")
	if i < 0 {
		return 1
	}
	n, err := strconv.Atoi(base[i+1:])
	if err != nil {
		return 1
	}
	return n
}

func (s *SegmentSet) path(seq int) string {
	return filepath.Join(s.dir, fmt.Sprintf("%s-%06d.rec", s.prefix, seq))
}

func (s *SegmentSet) Append(kind uint32, payload []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur != nil && s.cur.Size() >= s.maxBytes {
		if err := s.rotateLocked(); err != nil {
			return err
		}
	}
	_, err := s.cur.Append(kind, payload)
	return err
}

func (s *SegmentSet) rotateLocked() error {
	if err := s.cur.Sync(); err != nil {
		return err
	}
	if err := s.cur.Close(); err != nil {
		return err
	}
	s.seq++
	w, err := Create(s.path(s.seq))
	if err != nil {
		return err
	}
	s.cur = w
	return nil
}

func (s *SegmentSet) Sync() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur == nil {
		return nil
	}
	return s.cur.Sync()
}

func (s *SegmentSet) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cur == nil {
		return nil
	}
	err := s.cur.Close()
	s.cur = nil
	return err
}

func (s *SegmentSet) Replay(fn func(Frame) error) error {
	names, err := s.list()
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := Scan(filepath.Join(s.dir, name), fn); err != nil {
			return err
		}
	}
	return nil
}
