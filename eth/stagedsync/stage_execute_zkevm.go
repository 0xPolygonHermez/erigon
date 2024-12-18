package stagedsync

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/c2h5oh/datasize"
	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/common/cmp"
	"github.com/ledgerwatch/erigon-lib/config3"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/wrap"

	"github.com/ledgerwatch/erigon-lib/kv/membatch"
	"github.com/ledgerwatch/log/v3"

	"github.com/ledgerwatch/erigon/zk/erigon_db"
	"github.com/ledgerwatch/erigon/zk/hermez_db"

	"os"

	"github.com/ledgerwatch/erigon/common/math"
	"github.com/ledgerwatch/erigon/core/rawdb"
	"github.com/ledgerwatch/erigon/eth/stagedsync/stages"
	rawdbZk "github.com/ledgerwatch/erigon/zk/rawdb"
	"github.com/ledgerwatch/erigon/zk/utils"
)

func SpawnExecuteBlocksStageZk(
	s *StageState,
	u Unwinder,
	tx kv.RwTx,
	toBlock uint64,
	ctx context.Context,
	cfg ExecuteBlockCfg,
	initialCycle bool,
) (err error) {
	if cfg.historyV3 {
		if err = ExecBlockV3(s, u, wrap.TxContainer{Tx: tx}, toBlock, ctx, cfg, initialCycle, log.New()); err != nil {
			return fmt.Errorf("ExecBlockV3: %w", err)
		}
		return nil
	}

	///// DEBUG BISECT /////
	highestBlockExecuted := s.BlockNumber
	defer func() {
		if cfg.zk.DebugLimit > 0 {
			if err != nil {
				log.Error("Execution Failed", "err", err, "block", highestBlockExecuted)
				os.Exit(2)
			}
		}
	}()
	///// DEBUG BISECT /////

	quit := ctx.Done()
	useExternalTx := tx != nil
	if !useExternalTx {
		if tx, err = cfg.db.BeginRw(context.Background()); err != nil {
			return fmt.Errorf("beginRw: %w", err)
		}
		defer tx.Rollback()
	}

	// state is stored through ethdb batches
	var batch kv.PendingMutations = membatch.NewHashBatch(tx, quit, cfg.dirs.Tmp, log.New())
	// avoids stacking defers within the loop
	defer func() {
		batch.Close()
	}()

	if err := utils.UpdateZkEVMBlockCfg(cfg.chainConfig, hermezDb, s.LogPrefix(), cfg.zk.LogLevel == log.LvlTrace); err != nil {
		return fmt.Errorf("UpdateZkEVMBlockCfg: %w", err)
	}

	to, total, err := getExecRange(cfg, tx, s.BlockNumber, toBlock, s.LogPrefix())
	if err != nil {
		return fmt.Errorf("getExecRange: %w", err)
	}

	log.Info(fmt.Sprintf("[%s] Blocks execution", s.LogPrefix()), "from", s.BlockNumber, "to", to)

	// Transform batch_size limit into Ggas
	gasState := uint64(cfg.batchSize) * uint64(datasize.KB) * 2
	logger := utils.NewTxGasLogger(
		logInterval,
		s.BlockNumber,
		total,
		gasState,
		s.LogPrefix(),
		&batch,
		tx,
		stages.SyncMetrics[stages.Execution],
	)
	logger.Start()
	defer logger.Stop()

	nextStageProgress, err := stages.GetStageProgress(tx, stages.HashState)
	if err != nil {
		return fmt.Errorf("getStageProgress: %w", err)
	}

	blockExecutor := NewBlockExecutor(
		ctx,
		s.LogPrefix(),
		cfg,
		tx,
		batch,
		initialCycle,
		nextStageProgress > 0, // Incremental move of next stages depend on fully written ChangeSets, Receipts, CallTraceSet
	)
	blockExecutor.Init(s.BlockNumber, to)

	var stoppedErr error

	for blockNum := s.BlockNumber + 1; blockNum <= to; blockNum++ {
		if cfg.zk.SyncLimit > 0 && blockNum > cfg.zk.SyncLimit {
			log.Info(fmt.Sprintf("[%s] Sync limit reached", s.LogPrefix()), "block", blockNum)
			break
		}

		if stoppedErr = common.Stopped(quit); stoppedErr != nil {
			break
		}

		if err := blockExecutor.ExecuteBlock(blockNum); err != nil {
			if !errors.Is(err, ErrExecutionError) {
				return fmt.Errorf("executeBlock: %w", err)
			}

			u.UnwindTo(blockNum-1, UnwindReason{Block: &blockExecutor.datastreamBlockHash})
			break
		}

		logger.AddBlock(
			uint64(blockExecutor.block.Transactions().Len()),
			blockExecutor.GetProgress(),
			blockExecutor.currentStateGas,
			blockNum,
		)

		// should update progress
		if batch.BatchSize() >= int(cfg.batchSize) {
			log.Info("Committed State", "gas reached", blockExecutor.currentStateGas, "gasTarget", gasState)
			blockExecutor.currentStateGas = 0
			if err = s.Update(batch, blockExecutor.GetProgress()); err != nil {
				return fmt.Errorf("s.Update: %w", err)
			}
			if err = batch.Flush(ctx, tx); err != nil {
				return fmt.Errorf("batch.Flush: %w", err)
			}
			if !useExternalTx {
				if err = tx.Commit(); err != nil {
					return fmt.Errorf("tx.Commit: %w", err)
				}
				tx, err = cfg.db.BeginRw(context.Background())
				if err != nil {
					return fmt.Errorf("cfg.db.BeginRw: %w", err)
				}
				defer tx.Rollback()

				logger.SetTx(tx)
				batch = membatch.NewHashBatch(tx, quit, cfg.dirs.Tmp, log.New())
				blockExecutor.SetNewTx(tx, batch)
			}
		}
	}

	if err = s.Update(batch, blockExecutor.GetProgress()); err != nil {
		return fmt.Errorf("s.Update: %w", err)
	}

	// we need to artificially update the headers stage here as well to ensure that notifications
	// can fire at the end of the stage loop and inform RPC subscriptions of new blocks for example
	if err = stages.SaveStageProgress(tx, stages.Headers, blockExecutor.GetProgress()); err != nil {
		return fmt.Errorf("SaveStageProgress: %w", err)
	}

	if err = batch.Flush(ctx, tx); err != nil {
		return fmt.Errorf("batch.Flush: %w", err)
	}

	// stageProgress is latest processsed block number
	if _, err = rawdb.IncrementStateVersionByBlockNumberIfNeeded(tx, blockExecutor.GetProgress()); err != nil {
		return fmt.Errorf("IncrementStateVersionByBlockNumberIfNeeded: %w", err)
	}

	if !useExternalTx {
		log.Info(fmt.Sprintf("[%s] Commiting DB transaction...", s.LogPrefix()), "block", blockExecutor.GetProgress())

		if err = tx.Commit(); err != nil {
			return fmt.Errorf("tx.Commit: %w", err)
		}
	}

	log.Info(fmt.Sprintf("[%s] Completed on", s.LogPrefix()), "block", blockExecutor.GetProgress())

	return stoppedErr
}

// returns calculated "to" block number for execution and the total blocks to be executed
func getExecRange(cfg ExecuteBlockCfg, tx kv.RwTx, stageProgress, toBlock uint64, logPrefix string) (uint64, uint64, error) {
	if cfg.zk.DebugLimit > 0 {
		prevStageProgress, err := stages.GetStageProgress(tx, stages.Senders)
		if err != nil {
			return 0, 0, fmt.Errorf("getStageProgress: %w", err)
		}
		to := prevStageProgress
		if cfg.zk.DebugLimit < to {
			to = cfg.zk.DebugLimit
		}
		total := to - stageProgress
		return to, total, nil
	}

	shouldShortCircuit, noProgressTo, err := utils.ShouldShortCircuitExecution(tx, logPrefix, cfg.zk.L2ShortCircuitToVerifiedBatch)
	if err != nil {
		return 0, 0, fmt.Errorf("ShouldShortCircuitExecution: %w", err)
	}
	prevStageProgress, err := stages.GetStageProgress(tx, stages.Senders)
	if err != nil {
		return 0, 0, fmt.Errorf("getStageProgress: %w", err)
	}

	// skip if no progress
	if prevStageProgress == 0 && toBlock == 0 {
		return 0, 0, nil
	}

	to := prevStageProgress
	if toBlock > 0 {
		to = cmp.Min(prevStageProgress, toBlock)
	}

	if shouldShortCircuit {
		to = noProgressTo
	}

	total := to - stageProgress

	return to, total, nil
}

func UnwindExecutionStageZk(
	u *UnwindState,
	s *StageState,
	tx kv.RwTx,
	ctx context.Context,
	cfg ExecuteBlockCfg,
	initialCycle bool,
) (err error) {
	if u.UnwindPoint >= s.BlockNumber {
		return nil
	}
	useExternalTx := tx != nil
	if !useExternalTx {
		if tx, err = cfg.db.BeginRw(context.Background()); err != nil {
			return fmt.Errorf("beginRw: %w", err)
		}
		defer tx.Rollback()
	}
	log.Info(fmt.Sprintf("[%s] Unwind Execution", u.LogPrefix()), "from", s.BlockNumber, "to", u.UnwindPoint)

	logger := log.New()
	if err = unwindExecutionStage(u, s, wrap.TxContainer{Tx: tx}, ctx, cfg, initialCycle, logger); err != nil {
		return fmt.Errorf("unwindExecutionStage: %w", err)
	}
	if err = UnwindExecutionStageDbWrites(ctx, u, s, tx); err != nil {
		return fmt.Errorf("UnwindExecutionStageDbWrites: %w", err)
	}

	// update the headers stage as we mark progress there as part of execution
	if err = stages.SaveStageProgress(tx, stages.Headers, u.UnwindPoint); err != nil {
		return fmt.Errorf("SaveStageProgress: %w", err)
	}

	if err = u.Done(tx); err != nil {
		return fmt.Errorf("u.Done: %w", err)
	}

	if !useExternalTx {
		if err = tx.Commit(); err != nil {
			return fmt.Errorf("tx.Commit: %w", err)
		}
	}
	return nil
}

func UnwindExecutionStageErigon(
	u *UnwindState,
	s *StageState,
	tx kv.RwTx,
	ctx context.Context,
	cfg ExecuteBlockCfg,
	initialCycle bool,
	logger log.Logger,
) error {
	return unwindExecutionStage(u, s, wrap.TxContainer{Tx: tx}, ctx, cfg, initialCycle, logger)
}

func PruneExecutionStageZk(s *PruneState, tx kv.RwTx, cfg ExecuteBlockCfg, ctx context.Context, initialCycle bool) (err error) {
	useExternalTx := tx != nil
	if !useExternalTx {
		if tx, err = cfg.db.BeginRw(ctx); err != nil {
			return fmt.Errorf("beginRw: %w", err)
		}
		defer tx.Rollback()
	}

	logEvery := time.NewTicker(logInterval)
	defer logEvery.Stop()

	if cfg.historyV3 {
		cfg.agg.SetTx(tx)
		if initialCycle {
			if err = cfg.agg.Prune(ctx, config3.HistoryV3AggregationStep/10); err != nil { // prune part of retired data, before commit
				return fmt.Errorf("cfg.agg.prune: %w", err)
			}
		} else {
			if err = cfg.agg.PruneWithTiemout(ctx, 1*time.Second); err != nil { // prune part of retired data, before commit
				return fmt.Errorf("cfg.agg.PruneWithTiemout: %w", err)
			}
		}
	} else {
		if cfg.prune.History.Enabled() {
			if err = rawdb.PruneTableDupSort(tx, kv.AccountChangeSet, s.LogPrefix(), cfg.prune.History.PruneTo(s.ForwardProgress), logEvery, ctx); err != nil {
				return fmt.Errorf("PruneTableDupSort: %w", err)
			}
			if err = rawdb.PruneTableDupSort(tx, kv.StorageChangeSet, s.LogPrefix(), cfg.prune.History.PruneTo(s.ForwardProgress), logEvery, ctx); err != nil {
				return fmt.Errorf("PruneTableDupSort: %w", err)
			}
		}

		if cfg.prune.Receipts.Enabled() {
			for _, table := range []string{kv.Receipts, kv.BorReceipts, kv.Log} {
				if err = rawdb.PruneTable(tx, table, cfg.prune.Receipts.PruneTo(s.ForwardProgress), ctx, math.MaxInt32); err != nil {
					return fmt.Errorf("rawdb.PruneTable %s: %w", table, err)
				}
			}
		}
		if cfg.prune.CallTraces.Enabled() {
			if err = rawdb.PruneTableDupSort(tx, kv.CallTraceSet, s.LogPrefix(), cfg.prune.CallTraces.PruneTo(s.ForwardProgress), logEvery, ctx); err != nil {
				return fmt.Errorf("PruneTableDupSort: %w", err)
			}
		}
	}

	if err = s.Done(tx); err != nil {
		return fmt.Errorf("s.Done: %w", err)
	}
	if !useExternalTx {
		if err = tx.Commit(); err != nil {
			return fmt.Errorf("tx.Commit: %w", err)
		}
	}
	return nil
}

func UnwindExecutionStageDbWrites(ctx context.Context, u *UnwindState, s *StageState, tx kv.RwTx) error {
	// backward values that by default handinged in stage_headers
	// TODO: check for other missing value like - WriteHeader_zkEvm, WriteHeadHeaderHash, WriteCanonicalHash, WriteBody, WriteSenders, WriteTxLookupEntries_zkEvm
	hash, err := rawdb.ReadCanonicalHash(tx, u.UnwindPoint)
	if err != nil {
		return fmt.Errorf("ReadCanonicalHash: %w", err)
	}
	if err := rawdb.WriteHeadHeaderHash(tx, hash); err != nil {
		return fmt.Errorf("WriteHeadHeaderHash: %w", err)
	}

	/*
		unwind EffectiveGasPricePercentage here although it is written in stage batches (RPC) or stage execute (Sequencer)
		EffectiveGasPricePercentage could not be unwound after TruncateBlocks
	*/
	eriDb := erigon_db.NewErigonDb(tx)
	hermezDb := hermez_db.NewHermezDb(tx)

	transactions, err := eriDb.GetBodyTransactions(u.UnwindPoint+1, s.BlockNumber)
	if err != nil {
		return fmt.Errorf("GetBodyTransactions: %w", err)
	}
	transactionHashes := make([]common.Hash, 0, len(*transactions))
	for _, tx := range *transactions {
		transactionHashes = append(transactionHashes, tx.Hash())
	}
	if err := hermezDb.DeleteEffectiveGasPricePercentages(&transactionHashes); err != nil {
		return fmt.Errorf("DeleteEffectiveGasPricePercentages: %w", err)
	}

	if err = rawdbZk.TruncateSenders(tx, u.UnwindPoint+1, s.BlockNumber); err != nil {
		return fmt.Errorf("TruncateSenders: %w", err)
	}
	if err = rawdb.TruncateTxLookupEntries_zkEvm(tx, u.UnwindPoint+1, s.BlockNumber); err != nil {
		return fmt.Errorf("delete tx lookup entires: %w", err)
	}
	if err = rawdb.TruncateBlocks(ctx, tx, u.UnwindPoint+1); err != nil {
		return fmt.Errorf("dTruncateBlocks: %w", err)
	}
	if err = rawdb.TruncateCanonicalHash(tx, u.UnwindPoint+1, true); err != nil {
		return fmt.Errorf("TruncateCanonicalHash: %w", err)
	}
	if err = rawdb.TruncateStateVersion(tx, u.UnwindPoint+1); err != nil {
		return fmt.Errorf("TruncateStateVersion: %w", err)
	}

	if err = hermezDb.DeleteBlockInfoRoots(u.UnwindPoint+1, s.BlockNumber); err != nil {
		return fmt.Errorf("DeleteBlockInfoRoots: %w", err)
	}

	return nil
}
