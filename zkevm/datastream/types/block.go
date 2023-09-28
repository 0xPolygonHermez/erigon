package types

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

const (
	l2BlockDataLength = 76

	// EntryTypeL2Block represents a L2 block
	EntryTypeL2Block EntryType = 1
)

// L2Block represents a zkEvm block
type L2Block struct {
	BatchNumber    uint64         // 8 bytes
	L2BlockNumber  uint64         // 8 bytes
	Timestamp      int64          // 8 bytes
	GlobalExitRoot common.Hash    // 32 bytes
	Coinbase       common.Address // 20 bytes
}

func DecodeL2Block(data []byte) (*L2Block, error) {
	if len(data) != l2BlockDataLength {
		return &L2Block{}, fmt.Errorf("expected data length: %d, got: %d", l2BlockDataLength, len(data))
	}

	var ts int64
	buf := bytes.NewBuffer(data[16:24])
	if err := binary.Read(buf, binary.LittleEndian, &ts); err != nil {
		return &L2Block{}, err
	}

	return &L2Block{
		BatchNumber:    binary.LittleEndian.Uint64(data[:8]),
		L2BlockNumber:  binary.LittleEndian.Uint64(data[8:16]),
		Timestamp:      ts,
		GlobalExitRoot: common.BytesToHash(data[24:56]),
		Coinbase:       common.BytesToAddress(data[56:76]),
	}, nil
}
