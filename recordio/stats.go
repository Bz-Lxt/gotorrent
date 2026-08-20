package recordio

import (
	"fmt"
	"sort"
)

type KindStat struct {
	Kind  uint32
	Count int
	Bytes int
}

func Stats(path string) ([]KindStat, error) {
	by := map[uint32]*KindStat{}
	err := Scan(path, func(fr Frame) error {
		st := by[fr.Kind]
		if st == nil {
			st = &KindStat{Kind: fr.Kind}
			by[fr.Kind] = st
		}
		st.Count++
		st.Bytes += len(fr.Payload)
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]KindStat, 0, len(by))
	for _, st := range by {
		out = append(out, *st)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count == out[j].Count {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Count > out[j].Count
	})
	return out, nil
}

func FormatStats(st []KindStat) string {
	s := ""
	for _, item := range st {
		s += fmt.Sprintf("kind=%d count=%d bytes=%d\n", item.Kind, item.Count, item.Bytes)
	}
	return s
}

func TotalCount(st []KindStat) int {
	n := 0
	for _, item := range st {
		n += item.Count
	}
	return n
}

func TotalBytes(st []KindStat) int {
	n := 0
	for _, item := range st {
		n += item.Bytes
	}
	return n
}

func KindCount(st []KindStat, kind uint32) int {
	for _, item := range st {
		if item.Kind == kind {
			return item.Count
		}
	}
	return 0
}
