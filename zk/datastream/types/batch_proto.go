package types

import (
	"github.com/ledgerwatch/erigon/zk/datastream/proto/github.com/0xPolygonHermez/zkevm-node/state/datastream"
	"google.golang.org/protobuf/proto"
	libcommon "github.com/gateway-fm/cdk-erigon-lib/common"
)

type BatchStartProto struct {
	*datastream.BatchStart
}

type BatchEndProto struct {
	*datastream.BatchEnd
}

type BatchStart struct {
	Number  uint64
	ForkId  uint64
	ChainId uint64
}

func (b *BatchStartProto) Marshal() ([]byte, error) {
	return proto.Marshal(b.BatchStart)
}

func (b *BatchStartProto) Type() EntryType {
	return EntryType(datastream.EntryType_ENTRY_TYPE_BATCH_START)
}

type BatchEnd struct {
	LocalExitRoot libcommon.Hash
	StateRoot     libcommon.Hash
}

func (b *BatchEndProto) Marshal() ([]byte, error) {
	return proto.Marshal(b.BatchEnd)
}

func (b *BatchEndProto) Type() EntryType {
	return EntryType(datastream.EntryType_ENTRY_TYPE_BATCH_END)
}

func UnmarshalBatchStart(data []byte) (*BatchStart, error) {
	batch := &datastream.BatchStart{}
	if err := proto.Unmarshal(data, batch); err != nil {
		return nil, err
	}

	return &BatchStart{
		Number:  batch.Number,
		ForkId:  batch.ForkId,
		ChainId: batch.ChainId,
	}, nil
}

func UnmarshalBatchEnd(data []byte) (*BatchEnd, error) {
	batchEnd := &datastream.BatchEnd{}
	if err := proto.Unmarshal(data, batchEnd); err != nil {
		return nil, err
	}

	return &BatchEnd{
		LocalExitRoot: libcommon.BytesToHash(batchEnd.LocalExitRoot),
		StateRoot:     libcommon.BytesToHash(batchEnd.StateRoot),
	}, nil
}
