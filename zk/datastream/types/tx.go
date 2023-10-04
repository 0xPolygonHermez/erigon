package types

import (
	"encoding/binary"
	"fmt"
)

const (
	l2TxMinDataLength = 6
	// EntryTypeL2Tx represents a L2 transaction
	EntryTypeL2Tx EntryType = 2
)

// L2Transaction represents a zkEvm transaction
type L2Transaction struct {
	EffectiveGasPricePercentage uint8  // 1 byte
	IsValid                     uint8  // 1 byte
	EncodedLength               uint32 // 4 bytes
	Encoded                     []byte
}

// decodes a L2 transaction from a byte array
func DecodeL2Transaction(data []byte) (*L2Transaction, error) {
	dataLen := len(data)
	if dataLen < l2TxMinDataLength {
		return &L2Transaction{}, fmt.Errorf("expected minimum data length: %d, got: %d", l2TxMinDataLength, len(data))
	}

	encodedLength := binary.LittleEndian.Uint32(data[2:6])
	encoded := data[6:]
	if encodedLength != uint32(len(encoded)) {
		return &L2Transaction{}, fmt.Errorf("expected encoded length: %d, got: %d", encodedLength, len(encoded))
	}

	return &L2Transaction{
		EffectiveGasPricePercentage: data[0],
		IsValid:                     data[1],
		EncodedLength:               encodedLength,
		Encoded:                     encoded,
	}, nil
}
