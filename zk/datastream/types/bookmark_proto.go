package types

import (
	"github.com/ledgerwatch/erigon/zk/datastream/proto/github.com/0xPolygonHermez/zkevm-node/state/datastream"
	"google.golang.org/protobuf/proto"
)

type BookmarkBatchProto struct {
	*datastream.BookMark
}

func (b *BookmarkBatchProto) Marshal() ([]byte, error) {
	return proto.Marshal(b.BookMark)
}

func (b *BookmarkBatchProto) Type() (datastream.EntryType, datastream.BookmarkType) {
	return datastream.EntryType_ENTRY_TYPE_UNSPECIFIED, datastream.BookmarkType_BOOKMARK_TYPE_BATCH
}

type BookmarkL2BlockProto struct {
	*datastream.BookMark
}

func (b *BookmarkL2BlockProto) Marshal() ([]byte, error) {
	return proto.Marshal(b.BookMark)
}

func (b *BookmarkL2BlockProto) Type() (datastream.EntryType, datastream.BookmarkType) {
	return datastream.EntryType_ENTRY_TYPE_UNSPECIFIED, datastream.BookmarkType_BOOKMARK_TYPE_L2_BLOCK
}
