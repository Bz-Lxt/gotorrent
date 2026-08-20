package recordio

import (
	"encoding/json"
	"os"
	"time"
)

var snapZone = time.FixedZone("CST", 8*3600)

type SnapItem struct {
	Kind    uint32 `json:"kind"`
	Size    int    `json:"size"`
	Preview string `json:"preview,omitempty"`
}

type Snapshot struct {
	TS     string     `json:"ts"`
	Source string     `json:"source"`
	Count  int        `json:"count"`
	Items  []SnapItem `json:"items"`
}

func WriteSnapshot(src, dst string, limit int) error {
	if limit <= 0 {
		limit = 256
	}
	var items []SnapItem
	err := Scan(src, func(fr Frame) error {
		item := SnapItem{Kind: fr.Kind, Size: len(fr.Payload)}
		if len(fr.Payload) > 0 && len(fr.Payload) <= 64 {
			item.Preview = string(fr.Payload)
		}
		items = append(items, item)
		if len(items) > limit {
			items = items[len(items)-limit:]
		}
		return nil
	})
	if err != nil {
		return err
	}
	snap := Snapshot{
		TS:     time.Now().In(snapZone).Format(time.RFC3339),
		Source: src,
		Count:  len(items),
		Items:  items,
	}
	blob, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(dst, append(blob, '\n'), 0o644)
}

func ReadSnapshot(path string) (*Snapshot, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var snap Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}
