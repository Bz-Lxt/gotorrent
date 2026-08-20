package recordio

import (
	"os"
	"sync"
)

// Writer appends frames to a single file and can fsync on request.
type Writer struct {
	mu   sync.Mutex
	file *os.File
	path string
	n    int64
}

func Create(path string) (*Writer, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &Writer{file: f, path: path, n: st.Size()}, nil
}

func (w *Writer) Path() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.path
}

func (w *Writer) Size() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.n
}

func (w *Writer) Append(kind uint32, payload []byte) (int64, error) {
	blob, err := encodeFrame(kind, payload)
	if err != nil {
		return 0, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	off := w.n
	n, err := w.file.Write(blob)
	if err != nil {
		return 0, err
	}
	w.n += int64(n)
	return off, nil
}

func (w *Writer) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Sync()
}

func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}
