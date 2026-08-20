package recordio

import (
	"io"
	"os"
	"path/filepath"
)

// Compact copies src to dst dropping frames whose kind is in drop.
func Compact(src, dst string, drop map[uint32]bool) (kept int, err error) {
	r, err := OpenReader(src)
	if err != nil {
		return 0, err
	}
	defer r.Close()
	tmp := dst + ".tmp"
	w, err := Create(tmp)
	if err != nil {
		return 0, err
	}
	defer func() {
		cerr := w.Close()
		if err == nil {
			err = cerr
		}
	}()
	for {
		fr, nerr := r.Next()
		if nerr == io.EOF {
			break
		}
		if nerr != nil {
			return kept, nerr
		}
		if drop[fr.Kind] {
			continue
		}
		if _, err = w.Append(fr.Kind, fr.Payload); err != nil {
			return kept, err
		}
		kept++
	}
	if err = w.Sync(); err != nil {
		return kept, err
	}
	if err = w.Close(); err != nil {
		return kept, err
	}
	w.file = nil
	return kept, os.Rename(tmp, dst)
}

// CompactDir rewrites every .rec under dir into a sidecar "-compact.rec" then replaces.
func CompactDir(dir string, drop map[uint32]bool) error {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range ents {
		if e.IsDir() || filepath.Ext(e.Name()) != ".rec" {
			continue
		}
		src := filepath.Join(dir, e.Name())
		dst := src + ".new"
		if _, err := Compact(src, dst, drop); err != nil {
			return err
		}
		if err := os.Rename(dst, src); err != nil {
			return err
		}
	}
	return nil
}
