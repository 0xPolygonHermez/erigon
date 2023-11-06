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
	"github.com/ledgerwatch/erigon/zk"
	"github.com/ledgerwatch/erigon/zk/datastream"
	dsclient "github.com/ledgerwatch/erigon/zk/datastream/client"
	"github.com/ledgerwatch/erigon/zk/datastream/types"
	"github.com/ledgerwatch/erigon/zk/erigon_db"
	"github.com/ledgerwatch/erigon/zk/hermez_db"
	txtype "github.com/ledgerwatch/erigon/zk/tx"

	"github.com/ledgerwatch/erigon/eth/ethconfig"
	"github.com/ledgerwatch/log/v3"
)

type ISyncer interface {
}

type ErigonDb interface {
	WriteHeader(batchNo *big.Int, stateRoot, txHash common.Hash, coinbase common.Address, ts uint64) (*ethTypes.Header, error)
	WriteBody(batchNumber *big.Int, headerHash common.Hash, txs []ethTypes.Transaction) error
}

type HermezDb interface {
	WriteForkId(batchNumber uint64, forkId uint64) error
	WriteBlockBatch(l2BlockNumber uint64, batchNumber uint64) error

	DeleteForkIds(fromBatchNum, toBatchNum uint64) error
	DeleteBlockBatches(fromBatchNum, toBatchNum uint64) error

	WriteBlockGlobalExitRoot(l2BlockNo uint64, ger common.Hash) error

	WriteBatchGBatchGlobalExitRoot(batchNumber uint64, ger types.GerUpdate) error
}

type BatchesCfg struct {
	db     kv.RwDB
	syncer ISyncer
	zkCfg  *ethconfig.Zk
}

func StageBatchesCfg(db kv.RwDB, syncer ISyncer, zkCfg *ethconfig.Zk) BatchesCfg {
	return BatchesCfg{
		db:     db,
		syncer: syncer,
		zkCfg:  zkCfg,
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
	logPrefix := s.LogPrefix()
	log.Info(fmt.Sprintf("[%s] Starting batches stage", logPrefix))
	defer log.Info(fmt.Sprintf("[%s] Finished Batches stage", logPrefix))

	if tx == nil {
		log.Debug(fmt.Sprintf("[%s] batches: no tx provided, creating a new one", logPrefix))
		var err error
		tx, err = cfg.db.BeginRw(ctx)
		if err != nil {
			return fmt.Errorf("failed to open tx, %w", err)
		}
		defer tx.Rollback()
	}

	eriDb := erigon_db.NewErigonDb(tx)
	hermezDb, err := hermez_db.NewHermezDb(tx)
	if err != nil {
		return fmt.Errorf("failed to create hermezDb: %v", err)
	}

	log.Info(fmt.Sprintf("[%s] Getting stage progress...", logPrefix))
	batchesProgress, err := sync_stages.GetStageProgress(tx, sync_stages.Batches)
	if err != nil {
		return fmt.Errorf("save stage progress error: %v", err)
	}
	batchesProgress++
	log.Info(fmt.Sprintf("[%s] Starting stream", logPrefix), "startBlock", batchesProgress)

	l2BlockChan := make(chan types.FullL2Block, 100000)
	gerUpdatesChan := make(chan types.GerUpdate, 1000)
	errChan := make(chan error, 2)

	startSyncTime := time.Now()
	// start routine to download blocks and push them in a channel
	go func() {
		log.Info(fmt.Sprintf("[%s] Started downloading L2Blocks routine", logPrefix))
		defer log.Info(fmt.Sprintf("[%s] Finished downloading L2Blocks routine", logPrefix))
		var err error

		for {
			// this will download all blocks from datastream and push them in a channel
			// if no error, break, else continue trying to get them
			if _, _, err = datastream.DownloadAllL2BlocksToChannel(cfg.zkCfg.L2DataStreamerUrl, l2BlockChan, gerUpdatesChan, batchesProgress); err == nil {
				break
			}

			//[zkevm] - this is expected to be returned only when given block number is higher than the highest block number in datastream
			if err == dsclient.ErrBadBookmark {
				log.Debug(fmt.Sprintf("[%s] Invalid bookmark. Probably ahead of stream.", logPrefix))

				errChan <- nil
				return
			}

			log.Warn(fmt.Sprintf("[%s] Error from the datastream client, retrying... Error: %s", logPrefix, err))
		}

		errChan <- err
	}()

	// start a routine to print blocks written progress
	progressChan, stopProgressPrinter := zk.ProgressPrinter(logPrefix, 0)
	defer stopProgressPrinter()

	lastBlockHeight := batchesProgress
	endLoop := false
	blocksWritten := uint64(0)

	highestSeenBatchNo := uint64(0)
	highestHashableL2BlockNo := uint64(0)

	writeThreadFinished := false
	lastGer := common.Hash{}
	lastForkId64, err := sync_stages.GetStageProgress(tx, sync_stages.ForkId)
	lastForkId := uint16(lastForkId64)
	if err != nil {
		return fmt.Errorf("failed to get last fork id, %w", err)
	}
	for {
		// get block
		// if no blocks available should block
		// if download routine finished, should continue to read from channel until it's empty
		// if both download routine stopped and channel empty - stop loop
		var l2Block types.FullL2Block
		select {
		case l2BlockIncomming := <-l2BlockChan:
			l2Block = l2BlockIncomming // writes header, body, forkId and blockBatch
			zeroHash := common.Hash{}

			// update forkid
			if l2Block.ForkId > lastForkId {
				lastForkId = l2Block.ForkId
				err = hermezDb.WriteForkId(l2Block.BatchNumber, uint64(l2Block.ForkId))
				if err != nil {
					return fmt.Errorf("write fork id error: %v", err)
				}
			}

			// update GER
			if l2Block.GlobalExitRoot == zeroHash && l2Block.L2BlockNumber > 0 {
				if lastGer == zeroHash {
					prevGer, err := hermezDb.GetBlockGlobalExitRoot(l2Block.L2BlockNumber - 1)
					if err != nil {
						return fmt.Errorf("failed to get previous GER, %w", err)
					}
					if prevGer == zeroHash {
						//return fmt.Errorf("there is no previous GER saved")
						log.Info("no previous GER saved")
					}

					lastGer = prevGer
				}

				l2Block.GlobalExitRoot = lastGer
			}

			// batch boundary - record the highest hashable block number (last block in last full batch)
			if l2Block.BatchNumber > highestSeenBatchNo {
				highestHashableL2BlockNo = l2Block.L2BlockNumber - 1
			}
			highestSeenBatchNo = l2Block.BatchNumber

			if err := writeL2Block(eriDb, hermezDb, &l2Block); err != nil {
				return fmt.Errorf("writeL2Block error: %v", err)
			}

			lastBlockHeight = l2Block.L2BlockNumber
			blocksWritten++
			progressChan <- blocksWritten
		case gerUpdate := <-gerUpdatesChan:
			if err := hermezDb.WriteBatchGBatchGlobalExitRoot(gerUpdate.BatchNumber, gerUpdate); err != nil {
				return fmt.Errorf("write batch global exit root error: %v", err)
			}
		case err := <-errChan:
			if err != nil {
				return fmt.Errorf("l2blocks download routine error: %v", err)
			}
			writeThreadFinished = true
		default:
			if writeThreadFinished {
				endLoop = true
			}
		}

		if endLoop {
			break
		}
	}

	if lastBlockHeight == batchesProgress {
		log.Info(fmt.Sprintf("[%s] No new progress. Continue.", logPrefix), "lastBlockHeight", lastBlockHeight)
		return nil
	}

	// store the highest hashable block number
	if err := sync_stages.SaveStageProgress(tx, sync_stages.HighestHashableL2BlockNo, highestHashableL2BlockNo); err != nil {
		return fmt.Errorf("save stage progress error: %v", err)
	}

	// store the highest seen forkid
	if err := sync_stages.SaveStageProgress(tx, sync_stages.ForkId, uint64(lastForkId)); err != nil {
		return fmt.Errorf("save stage progress error: %v", err)
	}

	// stop printing blocks written progress routine
	elapsed := time.Since(startSyncTime)
	log.Info(fmt.Sprintf("[%s] Finished writing blocks", logPrefix), "blocksWritten", blocksWritten, "elapsed", elapsed)

	log.Info(fmt.Sprintf("[%s] Saving stage progress", logPrefix), "lastBlockHeight", lastBlockHeight)
	if err := sync_stages.SaveStageProgress(tx, sync_stages.Batches, lastBlockHeight); err != nil {
		return fmt.Errorf("save stage progress error: %v", err)
	}

	if firstCycle {
		log.Debug(fmt.Sprintf("[%s] batches: first cycle, committing tx", logPrefix))
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit tx, %w", err)
		}
	}

	return nil
}

func UnwindBatchesStage(u *sync_stages.UnwindState, tx kv.RwTx, cfg BatchesCfg, ctx context.Context) (err error) {
	logPrefix := u.LogPrefix()

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
	log.Info(fmt.Sprintf("[%s] Unwinding batches stage from block number", logPrefix), "fromBlock", fromBlock, "toBlock", toBlock)
	defer log.Info(fmt.Sprintf("[%s] Unwinding batches complete", logPrefix))

	eriDb := erigon_db.NewErigonDb(tx)
	hermezDb, err := hermez_db.NewHermezDb(tx)
	if err != nil {
		return fmt.Errorf("failed to create hermezDb: %v", err)
	}

	eriDb.DeleteBodies(fromBlock)
	eriDb.DeleteHeaders(fromBlock)
	hermezDb.DeleteForkIds(fromBlock, toBlock)
	hermezDb.DeleteBlockBatches(fromBlock, toBlock)
	hermezDb.DeleteBlockGlobalExitRoots(fromBlock, toBlock)

	log.Info(fmt.Sprintf("[%s] Deleted headers, bodies, forkIds and blockBatches.", logPrefix))
	log.Info(fmt.Sprintf("[%s] Saving stage progress", logPrefix), "fromBlock", fromBlock)

	if err := sync_stages.SaveStageProgress(tx, sync_stages.Batches, fromBlock); err != nil {
		return fmt.Errorf("save stage progress error: %v", err)
	}

	if err := u.Done(tx); err != nil {
		return err
	}
	if !useExternalTx {
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func PruneBatchesStage(s *sync_stages.PruneState, tx kv.RwTx, cfg BatchesCfg, ctx context.Context) (err error) {
	logPrefix := s.LogPrefix()
	useExternalTx := tx != nil
	if !useExternalTx {
		tx, err = cfg.db.BeginRw(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}

	log.Info(fmt.Sprintf("[%s] Pruning barches...", logPrefix))
	defer log.Info(fmt.Sprintf("[%s] Unwinding batches complete", logPrefix))

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
	hermezDb.DeleteBlockGlobalExitRoots(0, toBlock)

	log.Info(fmt.Sprintf("[%s] Deleted headers, bodies, forkIds and blockBatches.", logPrefix))
	log.Info(fmt.Sprintf("[%s] Saving stage progress", logPrefix), "stageProgress", 0)
	if err := sync_stages.SaveStageProgress(tx, sync_stages.Batches, 0); err != nil {
		return fmt.Errorf("save stage progress error: %v", err)
	}

	if !useExternalTx {
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// writeL2Block writes L2Block to ErigonDb and HermezDb
// writes header, body, forkId and blockBatch
func writeL2Block(eriDb ErigonDb, hermezDb HermezDb, l2Block *types.FullL2Block) error {
	bn := new(big.Int).SetUint64(l2Block.L2BlockNumber)
	h, err := eriDb.WriteHeader(bn, l2Block.StateRoot, l2Block.L2Blockhash, l2Block.Coinbase, uint64(l2Block.Timestamp))
	if err != nil {
		return fmt.Errorf("write header error: %v", err)
	}

	if err := hermezDb.WriteBlockGlobalExitRoot(l2Block.L2BlockNumber, l2Block.GlobalExitRoot); err != nil {
		return fmt.Errorf("write block global exit root error: %v", err)
	}

	txs := []ethTypes.Transaction{}
	for _, transaction := range l2Block.L2Txs {
		ltx, _, err := txtype.DecodeTx(transaction.Encoded, transaction.EffectiveGasPricePercentage, l2Block.ForkId)
		if err != nil {
			return fmt.Errorf("decode tx error: %v", err)
		}
		txs = append(txs, ltx)
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
