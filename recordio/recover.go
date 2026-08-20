package recordio

import (
	"io"
	"os"
)

// Recover copies every intact frame into dst and truncates a torn tail.
func Recover(src, dst string) (copied int, err error) {
	r, err := OpenReader(src)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	defer r.Close()
	w, err := Create(dst)
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
			return copied, nerr
		}
		if _, err = w.Append(fr.Kind, fr.Payload); err != nil {
			return copied, err
		}
		copied++
	}
	if err = w.Sync(); err != nil {
		return copied, err
	}
	return copied, nil
}

// TruncateTorn trims src to the last fully decoded frame.
func TruncateTorn(path string) error {
	r, err := OpenReader(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var keep int64
	for {
		pos, _ := r.file.Seek(0, io.SeekCurrent)
		_, nerr := r.Next()
		if nerr == io.EOF {
			r.Close()
			return os.Truncate(path, keep)
		}
		if nerr != nil {
			r.Close()
			return nerr
		}
		keep, _ = r.file.Seek(0, io.SeekCurrent)
		_ = pos
	}
}
