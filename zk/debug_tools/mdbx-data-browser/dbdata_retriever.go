package mdbxdatabrowser

import (
	"fmt"

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

// NewDbDataRetriever instantiates DbDataRetriever instance
func NewDbDataRetriever(tx kv.Tx) *DbDataRetriever {
	return &DbDataRetriever{
		tx:       tx,
		dbReader: hermez_db.NewHermezDbReader(tx),
	}
}

// GetBatchByNumber reads batch by number from the database
func (d *DbDataRetriever) GetBatchByNumber(batchNum uint64, verboseOutput bool) (*rpcTypes.Batch, error) {
	// highest block in batch
	blockNum, err := d.dbReader.GetHighestBlockInBatch(batchNum)
	if err != nil {
		return nil, err
	}

	blockHash, err := rawdb.ReadCanonicalHash(d.tx, blockNum)
	if err != nil {
		return nil, err
	}

	latestBlockInBatch := rawdb.ReadBlock(d.tx, blockHash, blockNum)
	if latestBlockInBatch == nil {
		return nil, fmt.Errorf("block %d not found", blockNum)
	}

	// last block in batch data
	batch := &rpcTypes.Batch{
		Number:    rpcTypes.ArgUint64(batchNum),
		Coinbase:  latestBlockInBatch.Coinbase(),
		StateRoot: latestBlockInBatch.Root(),
		Timestamp: rpcTypes.ArgUint64(latestBlockInBatch.Time()),
	}

	// block numbers in batch
	blocksInBatch, err := d.dbReader.GetL2BlockNosByBatch(batchNum)
	if err != nil {
		return nil, err
	}

	// collect blocks in batch
	// handle genesis - not in the hermez tables so requires special treament
	if batchNum == 0 {
		genesisBlock, err := rawdb.ReadBlockByNumber(d.tx, 0)
		if err != nil {
			return nil, err
		}
		batch.Blocks = append(batch.Blocks, genesisBlock.Hash())
	}

	for _, blockNum := range blocksInBatch {
		block, err := rawdb.ReadBlockByNumber(d.tx, blockNum)
		if err != nil {
			return nil, err
		}

		if !verboseOutput {
			batch.Blocks = append(batch.Blocks, block.Hash())
		} else {
			batch.Blocks = append(batch.Blocks, block)
		}

		for _, tx := range block.Transactions() {
			if !verboseOutput {
				batch.Transactions = append(batch.Transactions, tx.Hash())
			} else {
				batch.Transactions = append(batch.Transactions, tx)
			}
		}
	}

	// global exit root of batch
	ger, err := d.dbReader.GetBatchGlobalExitRoot(batchNum)
	if err != nil {
		return nil, err
	}
	if ger != nil {
		batch.GlobalExitRoot = ger.GlobalExitRoot
	}

	// sequence
	seq, err := d.dbReader.GetSequenceByBatchNo(batchNum)
	if err != nil {
		return nil, err
	}
	if seq != nil {
		batch.SendSequencesTxHash = &seq.L1TxHash
	}

	// sequenced, genesis or injected batch 1 - special batches 0,1 will always be closed
	batch.Closed = (seq != nil || batchNum <= 1)

	// verification
	ver, err := d.dbReader.GetVerificationByBatchNo(batchNum)
	if err != nil {
		return nil, err
	}
	if ver != nil {
		batch.VerifyBatchTxHash = &ver.L1TxHash
	}

	// batch l2 data
	batchL2Data, err := d.dbReader.GetL1BatchData(batchNum)
	if err != nil {
		return nil, err
	}
	batch.BatchL2Data = batchL2Data

	// L1 info tree (exit roots)
	l1InfoTree, err := d.dbReader.GetL1InfoTreeUpdateByGer(batch.GlobalExitRoot)
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
func (d *DbDataRetriever) GetBlockByNumber(blockNum uint64, includeTxs, includeReceipts bool) (*rpcTypes.Block, error) {
	blockHash, err := rawdb.ReadCanonicalHash(d.tx, blockNum)
	if err != nil {
		return nil, err
	}

	block := rawdb.ReadBlock(d.tx, blockHash, blockNum)
	if block == nil {
		return nil, fmt.Errorf("block %d not found", blockNum)
	}

	receipts := rawdb.ReadReceipts(d.tx, block, block.Body().SendersFromTxs())
	rpcBlock, err := rpcTypes.NewBlock(block, receipts.ToSlice(), includeTxs, includeReceipts)
	if err != nil {
		return nil, err
	}

	return rpcBlock, nil
}
