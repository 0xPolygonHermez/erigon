package types

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/log/v3"
)

const (
	startL2BlockDataLength                = 142
	startL2BlockDataLengthPreEtrogForkId7 = 78
	endL2BlockDataLength                  = 72

	// EntryTypeL2Block represents a L2 block
	EntryTypeStartL2Block EntryType = 1
	EntryTypeEndL2Block   EntryType = 3
)

// StartL2Block represents a zkEvm block
type StartL2Block struct {
	BatchNumber    uint64         // 8 bytes
	L2BlockNumber  uint64         // 8 bytes
	Timestamp      int64          // 8 bytes
	L1BlockHash    common.Hash    // 32 bytes
	GlobalExitRoot common.Hash    // 32 bytes
	L1InfoRoot     common.Hash    // 32 bytes
	Coinbase       common.Address // 20 bytes
	ForkId         uint16         // 2 bytes
}

// decodes a StartL2Block from a byte array
func DecodeStartL2Block(data []byte) (*StartL2Block, error) {
	if len(data) != startL2BlockDataLength {
		if len(data) == startL2BlockDataLengthPreEtrogForkId7 {
			log.Info("stream sent start l2 block length 78: pre etrog fork id 7", "blockNo", binary.LittleEndian.Uint64(data[8:16]))
			return decodeStartL2BlockPreEtrogForkId7(data)
		}
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
		L1BlockHash:    common.BytesToHash(data[24:56]),
		GlobalExitRoot: common.BytesToHash(data[56:88]),
		L1InfoRoot:     common.BytesToHash(data[88:120]),
		Coinbase:       common.BytesToAddress(data[120:140]),
		ForkId:         binary.LittleEndian.Uint16(data[140:142]),
	}, nil
}

func decodeStartL2BlockPreEtrogForkId7(data []byte) (*StartL2Block, error) {
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

func EncodeStartL2Block(block *StartL2Block) []byte {
	b := make([]byte, 0)
	b = binary.LittleEndian.AppendUint64(b, block.BatchNumber)
	b = binary.LittleEndian.AppendUint64(b, block.L2BlockNumber)
	b = binary.LittleEndian.AppendUint64(b, uint64(block.Timestamp))
	b = append(b, block.L1BlockHash.Bytes()...)
	b = append(b, block.GlobalExitRoot.Bytes()...)
	b = append(b, block.L1InfoRoot.Bytes()...)
	b = append(b, block.Coinbase.Bytes()...)
	b = binary.LittleEndian.AppendUint16(b, block.ForkId)
	return b
}

type EndL2Block struct {
	L2BlockNumber uint64      // 8 bytes
	L2Blockhash   common.Hash // 32 bytes
	StateRoot     common.Hash // 32 bytes
}

// DecodeEndL2Block decodes a EndL2Block from a byte array
func DecodeEndL2Block(data []byte) (*EndL2Block, error) {
	if len(data) != endL2BlockDataLength {
		return &EndL2Block{}, fmt.Errorf("expected data length: %d, got: %d", endL2BlockDataLength, len(data))
	}

	return &EndL2Block{
		L2BlockNumber: binary.LittleEndian.Uint64(data[:8]),
		L2Blockhash:   common.BytesToHash(data[8:40]),
		StateRoot:     common.BytesToHash(data[40:72]),
	}, nil
}

func EncodeEndL2Block(end *EndL2Block) []byte {
	bytes := make([]byte, 0)
	bytes = binary.LittleEndian.AppendUint64(bytes, end.L2BlockNumber)
	bytes = append(bytes, end.L2Blockhash[:]...)
	bytes = append(bytes, end.StateRoot[:]...)
	return bytes
}

type FullL2Block struct {
	BatchNumber    uint64
	L2BlockNumber  uint64
	Timestamp      int64
	GlobalExitRoot common.Hash
	Coinbase       common.Address
	ForkId         uint16
	L1BlockHash    common.Hash
	L1InfoRoot     common.Hash
	L2Blockhash    common.Hash
	StateRoot      common.Hash
	L2Txs          []L2Transaction
	ParentHash     common.Hash
}

// ParseFullL2Block parses a FullL2Block from a StartL2Block, EndL2Block and a slice of L2Transactions
func ParseFullL2Block(startL2Block *StartL2Block, endL2Block *EndL2Block, l2Txs *[]L2Transaction) *FullL2Block {
	return &FullL2Block{
		BatchNumber:    startL2Block.BatchNumber,
		L2BlockNumber:  startL2Block.L2BlockNumber,
		Timestamp:      startL2Block.Timestamp,
		GlobalExitRoot: startL2Block.GlobalExitRoot,
		Coinbase:       startL2Block.Coinbase,
		ForkId:         startL2Block.ForkId,
		L1BlockHash:    startL2Block.L1BlockHash,
		L1InfoRoot:     startL2Block.L1InfoRoot,
		L2Blockhash:    endL2Block.L2Blockhash,
		StateRoot:      endL2Block.StateRoot,
		L2Txs:          *l2Txs,
	}
}
