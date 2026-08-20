package recordio

import (
	"io"
	"os"
)

func CopyFile(src, dst string) (int, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(out, in)
	cerr := out.Close()
	if err != nil {
		return int(n), err
	}
	return int(n), cerr
}

func CopyFrames(src, dst string) (int, error) {
	r, err := OpenReader(src)
	if err != nil {
		return 0, err
	}
	defer r.Close()
	w, err := Create(dst)
	if err != nil {
		return 0, err
	}
	defer w.Close()
	copied := 0
	for {
		fr, nerr := r.Next()
		if nerr == io.EOF {
			break
		}
		if nerr != nil {
			return copied, nerr
		}
		if _, err := w.Append(fr.Kind, fr.Payload); err != nil {
			return copied, err
		}
		copied++
	}
	return copied, w.Sync()
}
