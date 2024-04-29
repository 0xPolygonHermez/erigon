package types

import (
	"github.com/ledgerwatch/erigon/zk/datastream/proto/github.com/0xPolygonHermez/zkevm-node/state/datastream"
	"google.golang.org/protobuf/proto"
)

type L2BlockProto struct {
	*datastream.L2Block
}

func (b *L2BlockProto) Marshal() ([]byte, error) {
	return proto.Marshal(b.L2Block)
}

func (b *L2BlockProto) Type() (datastream.EntryType, datastream.BookmarkType) {
	return datastream.EntryType_ENTRY_TYPE_L2_BLOCK, datastream.BookmarkType_BOOKMARK_TYPE_UNSPECIFIED
}
