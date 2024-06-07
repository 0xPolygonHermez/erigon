package mdbxdatabrowser

import (
	"github.com/gateway-fm/cdk-erigon-lib/kv"

	"github.com/ledgerwatch/erigon/core/rawdb"
	"github.com/ledgerwatch/erigon/core/state"
	"github.com/ledgerwatch/erigon/zk/hermez_db"
	rpcTypes "github.com/ledgerwatch/erigon/zk/rpcdaemon"
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
func (z *DbDataRetriever) GetBatchByNumber(batchNum uint64, verboseOutput bool) (*rpcTypes.Batch, error) {
	// highest block in batch
	blockNum, err := z.dbReader.GetHighestBlockInBatch(batchNum)
	if err != nil {
		return nil, err
	}

	blockHash, err := rawdb.ReadCanonicalHash(z.tx, blockNum)
	if err != nil {
		return nil, err
	}

	block := rawdb.ReadBlock(z.tx, blockHash, blockNum)

	// last block in batch data
	batch := &rpcTypes.Batch{
		Number:    rpcTypes.ArgUint64(batchNum),
		Coinbase:  block.Coinbase(),
		StateRoot: block.Root(),
		Timestamp: rpcTypes.ArgUint64(block.Time()),
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

	// L1 info tree (exit roots)
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

// GetBlockByNumber reads block based on its block number from the database
func (z *DbDataRetriever) GetBlockByNumber(blockNum uint64, includeTxs, includeReceipts bool) (*rpcTypes.Block, error) {
	blockHash, err := rawdb.ReadCanonicalHash(z.tx, blockNum)
	if err != nil {
		return nil, err
	}

	block := rawdb.ReadBlock(z.tx, blockHash, blockNum)
	receipts := rawdb.ReadReceipts(z.tx, block, block.Body().SendersFromTxs())

	rpcBlock, err := rpcTypes.NewBlock(block, receipts.ToSlice(), includeTxs, includeReceipts)
	if err != nil {
		return nil, err
	}

	return rpcBlock, nil
}
