package recordio

import "io"

// Scan calls fn for every intact frame. fn may return io.EOF to stop early.
func Scan(path string, fn func(Frame) error) error {
	r, err := OpenReader(path)
	if err != nil {
		return err
	}
	defer r.Close()
	for {
		fr, err := r.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := fn(*fr); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// LastKind returns the last frame of the given kind, or nil.
func LastKind(path string, kind uint32) (*Frame, error) {
	var found *Frame
	err := Scan(path, func(fr Frame) error {
		if fr.Kind == kind {
			cp := fr
			found = &cp
		}
		return nil
	})
	return found, err
}
