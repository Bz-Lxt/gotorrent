package recordio

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
)

const (
	magic     = 0x31564c57 // "WLV1" little-endian tag
	headerLen = 16
	maxPayload = 16 << 20
)

// Frame is a length-prefixed record with a CRC of the payload.
type Frame struct {
	Kind    uint32
	Payload []byte
}

func encodeFrame(kind uint32, payload []byte) ([]byte, error) {
	if len(payload) > maxPayload {
		return nil, fmt.Errorf("recordio: payload %d exceeds %d", len(payload), maxPayload)
	}
	out := make([]byte, headerLen+len(payload))
	binary.LittleEndian.PutUint32(out[0:4], magic)
	binary.LittleEndian.PutUint32(out[4:8], kind)
	binary.LittleEndian.PutUint32(out[8:12], uint32(len(payload)))
	binary.LittleEndian.PutUint32(out[12:16], crc32.ChecksumIEEE(payload))
	copy(out[headerLen:], payload)
	return out, nil
}

func decodeHeader(hdr []byte) (kind, n, crc uint32, err error) {
	if len(hdr) < headerLen {
		return 0, 0, 0, fmt.Errorf("recordio: short header")
	}
	if binary.LittleEndian.Uint32(hdr[0:4]) != magic {
		return 0, 0, 0, fmt.Errorf("recordio: bad magic")
	}
	kind = binary.LittleEndian.Uint32(hdr[4:8])
	n = binary.LittleEndian.Uint32(hdr[8:12])
	crc = binary.LittleEndian.Uint32(hdr[12:16])
	if n > maxPayload {
		return 0, 0, 0, fmt.Errorf("recordio: declared length %d too large", n)
	}
	return kind, n, crc, nil
}

func checkCRC(payload []byte, want uint32) error {
	got := crc32.ChecksumIEEE(payload)
	if got != want {
		return fmt.Errorf("recordio: crc mismatch want=%d got=%d", want, got)
	}
	return nil
}
