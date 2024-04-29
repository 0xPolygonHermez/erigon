package types

import (
	"github.com/ledgerwatch/erigon/zk/datastream/proto/github.com/0xPolygonHermez/zkevm-node/state/datastream"
	"google.golang.org/protobuf/proto"
)

type BatchProto struct {
	*datastream.Batch
}

func (b *BatchProto) Marshal() ([]byte, error) {
	return proto.Marshal(b.Batch)
}

func (b *BatchProto) Type() (datastream.EntryType, datastream.BookmarkType) {
	return datastream.EntryType_ENTRY_TYPE_BATCH, datastream.BookmarkType_BOOKMARK_TYPE_UNSPECIFIED
}
