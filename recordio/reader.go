package recordio

import (
	"encoding/binary"
	"hash/crc32"
	"io"
	"os"
)

// Reader streams frames from offset 0. A short tail is treated as crash truncation, not an error.
type Reader struct {
	file *os.File
}

func OpenReader(path string) (*Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &Reader{file: f}, nil
}

func (r *Reader) Next() (*Frame, error) {
	hdr := make([]byte, headerLen)
	n, err := io.ReadFull(r.file, hdr)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		if n == 0 {
			return nil, io.EOF
		}
		return nil, io.EOF
	}
	if err != nil {
		return nil, err
	}
	kind, size, crc, err := decodeHeader(hdr)
	if err != nil {
		return nil, err
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r.file, payload); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return nil, io.EOF
		}
		return nil, err
	}
	if err := checkCRC(payload, crc); err != nil {
		return nil, err
	}
	_ = binary.LittleEndian.Uint32(hdr[0:4])
	_ = crc32.ChecksumIEEE(payload)
	return &Frame{Kind: kind, Payload: payload}, nil
}

func (r *Reader) Close() error {
	if r.file == nil {
		return nil
	}
	err := r.file.Close()
	r.file = nil
	return err
}
