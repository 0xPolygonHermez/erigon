package acc_input_hash

import (
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon/core/rawdb"
	eritypes "github.com/ledgerwatch/erigon/core/types"
)

type RawDbBlockReader struct {
	tx kv.Tx
}

func (r *RawDbBlockReader) ReadBlockByNumber(blockNo uint64) (*eritypes.Block, error) {
	return rawdb.ReadBlockByNumber(r.tx, blockNo)
}
