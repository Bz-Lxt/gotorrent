package recordio

import (
	"fmt"
	"hash/crc32"
	"io"
	"os"
)

// Verify scans a file and returns frame count plus a rolling CRC of payloads.
func Verify(path string) (n int, sum uint32, err error) {
	r, err := OpenReader(path)
	if err != nil {
		return 0, 0, err
	}
	defer r.Close()
	h := crc32.NewIEEE()
	for {
		fr, nerr := r.Next()
		if nerr == io.EOF {
			return n, h.Sum32(), nil
		}
		if nerr != nil {
			return n, h.Sum32(), nerr
		}
		n++
		_, _ = h.Write(fr.Payload)
	}
}

func MustIntact(path string) error {
	n, _, err := Verify(path)
	if err != nil {
		return err
	}
	if n == 0 && Exists(path) {
		st, err := os.Stat(path)
		if err != nil {
			return err
		}
		if st.Size() > 0 {
			return fmt.Errorf("recordio: %s has bytes but zero intact frames", path)
		}
	}
	return nil
}
