package stages

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ledgerwatch/erigon-lib/common"

	"github.com/ledgerwatch/erigon-lib/kv"
	ethTypes "github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/sync_stages"
	"github.com/ledgerwatch/erigon/zk/datastream"
	"github.com/ledgerwatch/erigon/zk/datastream/types"
	"github.com/ledgerwatch/erigon/zk/erigon_db"
	"github.com/ledgerwatch/erigon/zk/hermez_db"
	txtype "github.com/ledgerwatch/erigon/zk/tx"
	"github.com/ledgerwatch/log/v3"
)

type ISyncer interface {
}

type ErigonDb interface {
	WriteHeader(batchNo *big.Int, stateRoot, txHash common.Hash, coinbase common.Address, ts uint64) (*ethTypes.Header, error)
	WriteBody(batchNumber *big.Int, headerHash common.Hash, txs []ethTypes.Transaction) error
	DeleteHeaders(blockFrom uint64) error
	DeleteBodies(blockFrom uint64) error
}

type HermezDb interface {
	WriteForkId(batchNumber uint64, forkId uint64) error
	WriteBlockBatch(l2BlockNumber uint64, batchNumber uint64) error
	DeleteForkIds(fromBatchNum, toBatchNum uint64) error
	DeleteBlockBatches(fromBatchNum, toBatchNum uint64) error
}

type BatchesCfg struct {
	db     kv.RwDB
	syncer ISyncer
}

const BatchesEntries = "BatchesEntries"

func StageBatchesCfg(db kv.RwDB, syncer ISyncer) BatchesCfg {
	return BatchesCfg{
		db:     db,
		syncer: syncer,
	}
}

func SpawnStageBatches(
	s *sync_stages.StageState,
	u sync_stages.Unwinder,
	ctx context.Context,
	tx kv.RwTx,
	cfg BatchesCfg,
	firstCycle bool,
	quiet bool,
) error {
	log.Info("Batches stage")
	defer log.Info("Finished Batches stage")

	eriDb := erigon_db.NewErigonDb(tx)
	hermezDb, err := hermez_db.NewHermezDb(tx)
	if err != nil {
		return fmt.Errorf("failed to create hermezDb: %v", err)
	}

	l2BlockChan := make(chan types.FullL2Block, 100000)
	entriesReadChan := make(chan uint64, 2)
	errChan := make(chan error, 2)

	// start routine to download blocks and push them in a channel
	go func() {
		log.Info("Started downloading L2Blocks routine")
		defer log.Info("Finished downloading L2Blocks routine")

		// this will download all blocks from datastream and push them in a channel
		// entriesRead, err := datastream.DownloadHeadersToChannel(datastream.TestDatastreamUrl, l2BlockChan)

		// download a few blocks for test purposes
		l2Blocks, entriesRead, err := datastream.DownloadL2Blocks(datastream.TestDatastreamUrl, 0, 10)
		for _, l2Block := range *l2Blocks {
			l2BlockChan <- l2Block
		}
		// test coe end

		entriesReadChan <- entriesRead
		errChan <- err
	}()

	// start a routine to print blocks written progress
	l2BlockWritten := make(chan uint64)
	ctx, cancelCtx := context.WithCancel(ctx)
	go func(ctx context.Context) {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		log.Info("Started printing blocks written progress routine")
		defer log.Info("Finished printing blocks written progress routine")
		count := uint64(0)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				log.Info("Blocks written: ", count)
			case <-l2BlockWritten:
				count++
			}
		}
	}(ctx)

	writeFinished := false
	stopLoop := false
	entriesRead := uint64(0)
	var routineErr error
	lastBlockHeight := uint64(0)
	for {
		// get block
		// if no blocks available shold block
		// if download routine finished, should continue to read from channel until it's empty
		// if both download routine stopped and channel empty - stop loop
		var l2Block types.FullL2Block
		select {
		case a := <-l2BlockChan:
			l2Block = a
		case entriesReadFromChan := <-entriesReadChan:
			routineErr = <-errChan
			if routineErr != nil {
				return fmt.Errorf("l2blocks download routine error: %v", err)
			}
			entriesRead = entriesReadFromChan
			writeFinished = true
		default:
			if writeFinished {
				stopLoop = true
			}
		}

		if stopLoop {
			break
		}

		// writes header, body, forkId and blockBatch
		if err := writeL2Block(eriDb, hermezDb, &l2Block); err != nil {
			return fmt.Errorf("writeL2Block error: %v", err)
		}

		lastBlockHeight = l2Block.L2BlockNumber
		l2BlockWritten <- 1
	}

	// stop printing blocks written progress routine
	cancelCtx()

	log.Info("Saving stage progress: %d", entriesRead)
	if err := sync_stages.SaveStageProgress(tx, sync_stages.Batches, lastBlockHeight); err != nil {
		return fmt.Errorf("save stage progress error: %v", err)
	}

	log.Info("Saving datastream entries progress: %d", entriesRead)
	if err := sync_stages.SaveStageProgress(tx, BatchesEntries, entriesRead); err != nil {
		return fmt.Errorf("save stage datastream progress error: %v", err)
	}

	return nil
}

func UnwindBatchesStage(u *sync_stages.UnwindState, tx kv.RwTx, cfg BatchesCfg, ctx context.Context) (err error) {
	useExternalTx := tx != nil
	if !useExternalTx {
		tx, err = cfg.db.BeginRw(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}

	fromBlock := u.UnwindPoint
	toBlock := u.CurrentBlockNumber
	log.Info("Unwinding batches stage from block number %d to %d", fromBlock, toBlock)
	defer log.Info("Unwinding batches complete")

	eriDb := erigon_db.NewErigonDb(tx)
	hermezDb, err := hermez_db.NewHermezDb(tx)
	if err != nil {
		return fmt.Errorf("failed to create hermezDb: %v", err)
	}

	eriDb.DeleteBodies(fromBlock)
	eriDb.DeleteHeaders(fromBlock)
	hermezDb.DeleteForkIds(fromBlock, toBlock)
	hermezDb.DeleteBlockBatches(fromBlock, toBlock)

	log.Info("Deleted headers, bodies, forkIds and blockBatches.")
	log.Info("Saving stage progress: %d", fromBlock)
	if err := sync_stages.SaveStageProgress(tx, sync_stages.Batches, fromBlock); err != nil {
		return fmt.Errorf("save stage progress error: %v", err)
	}

	// TODO: get somehow the datastream point of the block we unwinded to
	// current implementation counts on the fact that there is currently only 1 tx per block
	// might not work properly in the future
	blocksUnwound := toBlock - fromBlock
	currentDatastreamPoint, err := sync_stages.GetStageProgress(tx, BatchesEntries)
	if err != nil {
		return fmt.Errorf("get stage datastream progress error: %v", err)
	}

	dup := currentDatastreamPoint - blocksUnwound
	log.Info("Saving datastream entries progress: %d", dup)
	if err := sync_stages.SaveStageProgress(tx, BatchesEntries, dup); err != nil {
		return fmt.Errorf("save stage datastream progress error: %v", err)
	}

	if err = u.Done(tx); err != nil {
		return err
	}
	if !useExternalTx {
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func PruneBatchesStage(s *sync_stages.PruneState, tx kv.RwTx, cfg BatchesCfg, ctx context.Context) (err error) {
	useExternalTx := tx != nil
	if !useExternalTx {
		tx, err = cfg.db.BeginRw(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}

	log.Info("Pruning barches...")
	defer log.Info("Unwinding batches complete")

	eriDb := erigon_db.NewErigonDb(tx)
	hermezDb, err := hermez_db.NewHermezDb(tx)
	if err != nil {
		return fmt.Errorf("failed to create hermezDb: %v", err)
	}

	toBlock, err := sync_stages.GetStageProgress(tx, sync_stages.Batches)
	if err != nil {
		return fmt.Errorf("get stage datastream progress error: %v", err)
	}

	eriDb.DeleteBodies(0)
	eriDb.DeleteHeaders(0)
	hermezDb.DeleteForkIds(0, toBlock)
	hermezDb.DeleteBlockBatches(0, toBlock)

	log.Info("Deleted headers, bodies, forkIds and blockBatches.")
	log.Info("Saving stage progress: %d", 0)
	if err := sync_stages.SaveStageProgress(tx, sync_stages.Batches, 0); err != nil {
		return fmt.Errorf("save stage progress error: %v", err)
	}

	log.Info("Saving datastream entries progress: %d", 0)
	if err := sync_stages.SaveStageProgress(tx, BatchesEntries, 0); err != nil {
		return fmt.Errorf("save stage datastream progress error: %v", err)
	}

	if !useExternalTx {
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// writeL2Block writes L2Block to ErigonDb and HermezDb
// writes header, body, forkId and blockBatch
func writeL2Block(eriDb ErigonDb, hermezDb HermezDb, l2Block *types.FullL2Block) error {
	bn := new(big.Int).SetUint64(l2Block.BatchNumber)
	h, err := eriDb.WriteHeader(bn, l2Block.GlobalExitRoot, l2Block.L2Blockhash, l2Block.Coinbase, uint64(l2Block.Timestamp))
	if err != nil {
		return fmt.Errorf("write header error: %v", err)
	}

	batchTxData := []byte{}
	for _, transaction := range l2Block.L2Txs {
		batchTxData = append(batchTxData, transaction.Encoded...)
	}

	txs, _, _, err := txtype.DecodeTxs(batchTxData, uint64(l2Block.ForkId))
	if err != nil {
		return fmt.Errorf("decode txs error: %v", err)
	}

	if err := eriDb.WriteBody(bn, h.Hash(), txs); err != nil {
		return fmt.Errorf("write body error: %v", err)
	}

	if err := hermezDb.WriteForkId(l2Block.BatchNumber, uint64(l2Block.ForkId)); err != nil {
		return fmt.Errorf("write block batch error: %v", err)
	}

	if err := hermezDb.WriteBlockBatch(l2Block.L2BlockNumber, l2Block.BatchNumber); err != nil {
		return fmt.Errorf("write block batch error: %v", err)
	}

	return nil
}
