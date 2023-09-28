package types

import (
	"encoding/binary"
	"fmt"
)

const (
	l2TxMinDataLength = 14
	// EntryTypeL2Tx represents a L2 transaction
	EntryTypeL2Tx EntryType = 2
)

// L2Transaction represents a zkEvm transaction
type L2Transaction struct {
	BatchNumber                 uint64 // 8 bytes
	EffectiveGasPricePercentage uint8  // 1 byte
	IsValid                     uint8  // 1 byte
	EncodedLength               uint32 // 4 bytes
	Encoded                     []byte
}

func DecodeL2Transaction(data []byte) (*L2Transaction, error) {
	dataLen := len(data)
	if dataLen < l2TxMinDataLength {
		return &L2Transaction{}, fmt.Errorf("expected minimum data length: %d, got: %d", l2TxMinDataLength, len(data))
	}

	encodedLength := binary.LittleEndian.Uint32(data[10:14])
	encoded := data[14:]
	if encodedLength != uint32(len(encoded)) {
		return &L2Transaction{}, fmt.Errorf("expected encoded length: %d, got: %d", encodedLength, len(encoded))
	}

	return &L2Transaction{
		BatchNumber:                 binary.LittleEndian.Uint64(data[:8]),
		EffectiveGasPricePercentage: data[8],
		IsValid:                     data[9],
		EncodedLength:               encodedLength,
		Encoded:                     encoded,
	}, nil
}
