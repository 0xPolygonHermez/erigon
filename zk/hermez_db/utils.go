package hermez_db

import (
	"encoding/binary"
	"fmt"
)

func UintBytes(n uint64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, n)
	return b
}

func BytesUint(b []byte) uint64 {
	return binary.LittleEndian.Uint64(b)
}

func SplitKey(data []byte) (uint64, uint64, error) {
	if len(data) != 16 {
		return 0, 0, fmt.Errorf("data length is not 16 bytes")
	}

	l1blockno := BytesUint(data[:8])
	batchno := BytesUint(data[8:])

	return l1blockno, batchno, nil
}

func ConcatKey(l1blockno, batchno uint64) []byte {
	return append(UintBytes(l1blockno), UintBytes(batchno)...)
}
