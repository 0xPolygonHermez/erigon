package types

import (
	"github.com/ledgerwatch/erigon/zk/datastream/proto/github.com/0xPolygonHermez/zkevm-node/state/datastream"
	"google.golang.org/protobuf/proto"
)

type TxProto struct {
	*datastream.Transaction
}

func (t *TxProto) Marshal() ([]byte, error) {
	return proto.Marshal(t.Transaction)
}

func (t *TxProto) Type() (datastream.EntryType, datastream.BookmarkType) {
	return datastream.EntryType_ENTRY_TYPE_TRANSACTION, datastream.BookmarkType_BOOKMARK_TYPE_UNSPECIFIED
}
