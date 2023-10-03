package types

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/holiman/uint256"
)

const (
	startL2BlockDataLength = 78
	endL2BlockDataLength   = 64

	// EntryTypeL2Block represents a L2 block
	EntryTypeStartL2Block EntryType = 1
	EntryTypeEndL2Block   EntryType = 3
)

// StartL2Block represents a zkEvm block
type StartL2Block struct {
	BatchNumber    uint64         // 8 bytes
	L2BlockNumber  uint64         // 8 bytes
	Timestamp      int64          // 8 bytes
	GlobalExitRoot common.Hash    // 32 bytes
	Coinbase       common.Address // 20 bytes
	ForkId         uint16         // 2 bytes
}

func DecodeStartL2Block(data []byte) (*StartL2Block, error) {
	if len(data) != startL2BlockDataLength {
		return &StartL2Block{}, fmt.Errorf("expected data length: %d, got: %d", startL2BlockDataLength, len(data))
	}

	var ts int64
	buf := bytes.NewBuffer(data[16:24])
	if err := binary.Read(buf, binary.LittleEndian, &ts); err != nil {
		return &StartL2Block{}, err
	}

	return &StartL2Block{
		BatchNumber:    binary.LittleEndian.Uint64(data[:8]),
		L2BlockNumber:  binary.LittleEndian.Uint64(data[8:16]),
		Timestamp:      ts,
		GlobalExitRoot: common.BytesToHash(data[24:56]),
		Coinbase:       common.BytesToAddress(data[56:76]),
		ForkId:         binary.LittleEndian.Uint16(data[76:78]),
	}, nil
}

type EndL2Block struct {
	L2Blockhash uint256.Int // 32 bytes
	StateRoot   uint256.Int // 32 bytes
}

func DecodeEndL2Block(data []byte) (*EndL2Block, error) {
	if len(data) != endL2BlockDataLength {
		return &EndL2Block{}, fmt.Errorf("expected data length: %d, got: %d", endL2BlockDataLength, len(data))
	}
	i1 := binary.LittleEndian.Uint64(data[:8])
	i2 := binary.LittleEndian.Uint64(data[8:16])
	i3 := binary.LittleEndian.Uint64(data[16:24])
	i4 := binary.LittleEndian.Uint64(data[24:32])
	i5 := binary.LittleEndian.Uint64(data[32:40])
	i6 := binary.LittleEndian.Uint64(data[40:48])
	i7 := binary.LittleEndian.Uint64(data[48:56])
	i8 := binary.LittleEndian.Uint64(data[56:64])

	return &EndL2Block{
		L2Blockhash: uint256.Int{i1, i2, i3, i4},
		StateRoot:   uint256.Int{i5, i6, i7, i8},
	}, nil
}

type FullL2Block struct {
	BatchNumber    uint64
	L2BlockNumber  uint64
	Timestamp      int64
	GlobalExitRoot common.Hash
	Coinbase       common.Address
	ForkId         uint16
	L2Blockhash    uint256.Int
	StateRoot      uint256.Int
	L2Txs          []L2Transaction
}

func ParseFullL2Block(startL2Block *StartL2Block, endL2Block *EndL2Block, l2Txs *[]L2Transaction) FullL2Block {
	return FullL2Block{
		BatchNumber:    startL2Block.BatchNumber,
		L2BlockNumber:  startL2Block.L2BlockNumber,
		Timestamp:      startL2Block.Timestamp,
		GlobalExitRoot: startL2Block.GlobalExitRoot,
		Coinbase:       startL2Block.Coinbase,
		ForkId:         startL2Block.ForkId,
		L2Blockhash:    endL2Block.L2Blockhash,
		StateRoot:      endL2Block.StateRoot,
		L2Txs:          *l2Txs,
	}
}
