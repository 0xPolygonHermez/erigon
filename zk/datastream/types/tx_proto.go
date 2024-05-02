package types

import (
	"github.com/ledgerwatch/erigon/zk/datastream/proto/github.com/0xPolygonHermez/zkevm-node/state/datastream"
	"google.golang.org/protobuf/proto"
	libcommon "github.com/gateway-fm/cdk-erigon-lib/common"
)

type TxProto struct {
	*datastream.Transaction
}

func (t *TxProto) Marshal() ([]byte, error) {
	return proto.Marshal(t.Transaction)
}

func (t *TxProto) Type() EntryType {
	return EntryType(datastream.EntryType_ENTRY_TYPE_TRANSACTION)
}

func UnmarshalTx(data []byte) (*L2Transaction, error) {
	tx := datastream.Transaction{}
	err := proto.Unmarshal(data, &tx)
	if err != nil {
		return nil, err
	}

	isValid := uint8(0)
	if tx.IsValid {
		isValid = 1
	}

	l2Tx := &L2Transaction{
		EffectiveGasPricePercentage: uint8(tx.EffectiveGasPricePercentage),
		IsValid:                     isValid,
		StateRoot:                   libcommon.BytesToHash(tx.ImStateRoot),
		EncodedLength:               uint32(len(tx.Encoded)),
		Encoded:                     tx.Encoded,
	}

	return l2Tx, nil
}
