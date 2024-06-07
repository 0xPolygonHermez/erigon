package mdbxdatabrowser

import (
	"github.com/gateway-fm/cdk-erigon-lib/kv"

	"github.com/ledgerwatch/erigon/core/rawdb"
	"github.com/ledgerwatch/erigon/core/state"
	"github.com/ledgerwatch/erigon/zk/hermez_db"
	types "github.com/ledgerwatch/erigon/zk/rpcdaemon"
)

type DbDataRetriever struct {
	tx       kv.Tx
	dbReader state.ReadOnlyHermezDb
}

// newDbDataRetriever instantiates DbDataRetriever instance
func newDbDataRetriever(tx kv.Tx) *DbDataRetriever {
	return &DbDataRetriever{
		tx:       tx,
		dbReader: hermez_db.NewHermezDbReader(tx),
	}
}

// GetBatchByNumber reads batch by number from the database
func (z *DbDataRetriever) GetBatchByNumber(batchNum uint64, verboseOutput bool) (*types.Batch, error) {
	// highest block in batch
	blockNo, err := z.dbReader.GetHighestBlockInBatch(batchNum)
	if err != nil {
		return nil, err
	}

	blockHash, err := rawdb.ReadCanonicalHash(z.tx, blockNo)
	if err != nil {
		return nil, err
	}

	block := rawdb.ReadBlock(z.tx, blockHash, blockNo)

	// last block in batch data
	batch := &types.Batch{
		Number:    types.ArgUint64(batchNum),
		Coinbase:  block.Coinbase(),
		StateRoot: block.Root(),
		Timestamp: types.ArgUint64(block.Time()),
	}

	// block numbers in batch
	blocksInBatch, err := z.dbReader.GetL2BlockNosByBatch(batchNum)
	if err != nil {
		return nil, err
	}

	// collect blocks in batch
	// TODO: REMOVE
	// batch.Blocks = []interface{}{}
	// batch.Transactions = []interface{}{}
	// handle genesis - not in the hermez tables so requires special treament
	if batchNum == 0 {
		blk, err := rawdb.ReadBlockByNumber(z.tx, 0)
		if err != nil {
			return nil, err
		}
		batch.Blocks = append(batch.Blocks, blk.Hash())
	}

	for _, blkNo := range blocksInBatch {
		blk, err := rawdb.ReadBlockByNumber(z.tx, blkNo)
		if err != nil {
			return nil, err
		}

		if !verboseOutput {
			batch.Blocks = append(batch.Blocks, blk.Hash())
		} else {
			batch.Blocks = append(batch.Blocks, blk)
		}

		for _, tx := range blk.Transactions() {
			if !verboseOutput {
				batch.Transactions = append(batch.Transactions, tx)
			} else {
				batch.Transactions = append(batch.Transactions, tx.Hash())
			}
		}
	}

	// global exit root of batch
	ger, err := z.dbReader.GetBatchGlobalExitRoot(batchNum)
	if err != nil {
		return nil, err
	}
	if ger != nil {
		batch.GlobalExitRoot = ger.GlobalExitRoot
	}

	// sequence
	seq, err := z.dbReader.GetSequenceByBatchNo(batchNum)
	if err != nil {
		return nil, err
	}
	if seq != nil {
		batch.SendSequencesTxHash = &seq.L1TxHash
	}

	// sequenced, genesis or injected batch 1 - special batches 0,1 will always be closed
	batch.Closed = (seq != nil || batchNum <= 1)

	// verification
	ver, err := z.dbReader.GetVerificationByBatchNo(batchNum)
	if err != nil {
		return nil, err
	}
	if ver != nil {
		batch.VerifyBatchTxHash = &ver.L1TxHash
	}

	// batch l2 data
	batchL2Data, err := z.dbReader.GetL1BatchData(batchNum)
	if err != nil {
		return nil, err
	}
	batch.BatchL2Data = batchL2Data

	// exit roots
	l1InfoTree, err := z.dbReader.GetL1InfoTreeUpdateByGer(batch.GlobalExitRoot)
	if err != nil {
		return nil, err
	}

	if l1InfoTree != nil {
		batch.MainnetExitRoot = l1InfoTree.MainnetExitRoot
		batch.RollupExitRoot = l1InfoTree.RollupExitRoot
	}

	return batch, nil
}

func (z *DbDataRetriever) GetBlockByNumber(blockNum uint64, includeTxs bool) {
	// TODO: zkevm_api.go, GetFullBlockByNumber implementation
}
