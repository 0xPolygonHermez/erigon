package stages

import (
	"context"
	"fmt"
	"time"

	"github.com/gateway-fm/cdk-erigon-lib/common"
	"github.com/gateway-fm/cdk-erigon-lib/kv"
	"github.com/ledgerwatch/log/v3"

	"github.com/ledgerwatch/erigon/common/math"
	"github.com/ledgerwatch/erigon/core"
	"github.com/ledgerwatch/erigon/core/rawdb"
	"github.com/ledgerwatch/erigon/core/state"
	"github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/eth/stagedsync"
	"github.com/ledgerwatch/erigon/eth/stagedsync/stages"
	"github.com/ledgerwatch/erigon/zk"
	"github.com/ledgerwatch/erigon/zk/utils"
)

func SpawnSequencingStage(
	s *stagedsync.StageState,
	u stagedsync.Unwinder,
	rootTx kv.RwTx,
	ctx context.Context,
	cfg SequenceBlockCfg,
	quiet bool,
) (err error) {
	logPrefix := s.LogPrefix()
	log.Info(fmt.Sprintf("[%s] Starting sequencing stage", logPrefix))
	defer log.Info(fmt.Sprintf("[%s] Finished sequencing stage", logPrefix))

	sdb, err := newStageDb(ctx, rootTx, cfg.db)
	if err != nil {
		return err
	}
	defer sdb.tx.Rollback()

	executionAt, err := s.ExecutionAt(sdb.tx)
	if err != nil {
		return err
	}

	lastBatch, err := stages.GetStageProgress(sdb.tx, stages.HighestSeenBatchNumber)
	if err != nil {
		return err
	}

	isLastBatchPariallyProcessed, err := sdb.hermezDb.GetIsBatchPartiallyProcessed(lastBatch)
	if err != nil {
		return err
	}

	forkId, err := prepareForkId(lastBatch, executionAt, sdb.hermezDb)
	if err != nil {
		return err
	}

	batchContext := newBatchContext(ctx, &cfg, s, sdb)
	batchState := newBatchState(forkId, prepareBatchNumber(lastBatch, isLastBatchPariallyProcessed), !isLastBatchPariallyProcessed && cfg.zk.HasExecutors(), cfg.zk.L1SyncStartBlock > 0)

	// injected batch
	if executionAt == 0 {
		if err = processInjectedInitialBatch(batchContext, batchState); err != nil {
			return err
		}

		if err = cfg.datastreamServer.WriteWholeBatchToStream(logPrefix, sdb.tx, sdb.hermezDb.HermezDbReader, lastBatch, injectedBatchBatchNumber); err != nil {
			return err
		}

		return sdb.tx.Commit()
	}

	// handle case where batch wasn't closed properly
	// close it before starting a new one
	// this occurs when sequencer was switched from syncer or sequencer datastream files were deleted
	// and datastream was regenerated
	if err = finalizeLastBatchInDatastreamIfNotFinalized(batchContext, batchState, executionAt); err != nil {
		return err
	}

	if err := utils.UpdateZkEVMBlockCfg(cfg.chainConfig, sdb.hermezDb, logPrefix); err != nil {
		return err
	}

	batchCounters, err := prepareBatchCounters(batchContext, batchState, isLastBatchPariallyProcessed)
	if err != nil {
		return err
	}

	// check if we just unwound from a bad executor response and if we did just close the batch here
	handled, err := doInstantCloseIfNeeded(batchContext, batchState, batchCounters)
	if err != nil || handled {
		return err
	}

	batchTicker := time.NewTicker(cfg.zk.SequencerBatchSealTime)
	defer batchTicker.Stop()
	nonEmptyBatchTimer := time.NewTicker(cfg.zk.SequencerNonEmptyBatchSealTime)
	defer nonEmptyBatchTimer.Stop()

	runLoopBlocks := true
	blockDataSizeChecker := NewBlockDataChecker()
	batchDataOverflow := false

	batchVerifier := NewBatchVerifier(cfg.zk, batchState.hasExecutorForThisBatch, cfg.legacyVerifier, batchState.forkId)
	streamWriter := &SequencerBatchStreamWriter{
		ctx:           ctx,
		logPrefix:     logPrefix,
		batchVerifier: batchVerifier,
		sdb:           sdb,
		streamServer:  cfg.datastreamServer,
		hasExecutors:  batchState.hasExecutorForThisBatch,
		lastBatch:     lastBatch,
	}

	limboHeaderTimestamp, limboTxHash := cfg.txPool.GetLimboTxHash(batchState.batchNumber)
	limboRecovery := limboTxHash != nil
	isAnyRecovery := batchState.isL1Recovery() || limboRecovery

	// if not limbo set the limboHeaderTimestamp to the "default" value for "prepareHeader" function
	if !limboRecovery {
		limboHeaderTimestamp = math.MaxUint64
	}

	if batchState.isL1Recovery() {
		if cfg.zk.L1SyncStopBatch > 0 && batchState.batchNumber > cfg.zk.L1SyncStopBatch {
			log.Info(fmt.Sprintf("[%s] L1 recovery has completed!", logPrefix), "batch", batchState.batchNumber)
			time.Sleep(1 * time.Second)
			return nil
		}

		// let's check if we have any L1 data to recover
		if err = batchState.batchL1RecoveryData.loadBatchData(sdb, batchState.batchNumber, batchState.forkId); err != nil {
			return err
		}

		if !batchState.batchL1RecoveryData.hasAnyDecodedBlocks() {
			log.Info(fmt.Sprintf("[%s] L1 recovery has completed!", logPrefix), "batch", batchState.batchNumber)
			time.Sleep(1 * time.Second)
			return nil
		}

		if handled, err := doCheckForBadBatch(batchContext, batchState, executionAt); err != nil || handled {
			return err
		}
	}

	if !isLastBatchPariallyProcessed {
		log.Info(fmt.Sprintf("[%s] Starting batch %d...", logPrefix, batchState.batchNumber))
	} else {
		log.Info(fmt.Sprintf("[%s] Continuing unfinished batch %d from block %d", logPrefix, batchState.batchNumber, executionAt))
	}

	var block *types.Block
	for blockNumber := executionAt + 1; runLoopBlocks; blockNumber++ {
		log.Info(fmt.Sprintf("[%s] Starting block %d (forkid %v)...", logPrefix, blockNumber, batchState.forkId))

		if batchState.isL1Recovery() {
			didLoadedAnyDataForRecovery := batchState.loadBlockL1RecoveryData(blockNumber - (executionAt + 1))
			if !didLoadedAnyDataForRecovery {
				runLoopBlocks = false
				break
			}
		}

		l1InfoIndex, err := sdb.hermezDb.GetBlockL1InfoTreeIndex(blockNumber - 1)
		if err != nil {
			return err
		}

		header, parentBlock, err := prepareHeader(sdb.tx, blockNumber-1, batchState.blockState.getDeltaTimestamp(), limboHeaderTimestamp, batchState.forkId, batchState.getCoinbase(&cfg))
		if err != nil {
			return err
		}

		if batchDataOverflow = blockDataSizeChecker.AddBlockStartData(); batchDataOverflow {
			log.Info(fmt.Sprintf("[%s] BatchL2Data limit reached. Stopping.", logPrefix), "blockNumber", blockNumber)
			break
		}

		// timer: evm + smt
		t := utils.StartTimer("stage_sequence_execute", "evm", "smt")

		overflowOnNewBlock, err := batchCounters.StartNewBlock(l1InfoIndex != 0)
		if err != nil {
			return err
		}
		if !isAnyRecovery && overflowOnNewBlock {
			break
		}

		infoTreeIndexProgress, l1TreeUpdate, l1TreeUpdateIndex, l1BlockHash, ger, shouldWriteGerToContract, err := prepareL1AndInfoTreeRelatedStuff(sdb, batchState, header.Time)
		if err != nil {
			return err
		}

		var anyOverflow bool
		ibs := state.New(sdb.stateReader)
		getHashFn := core.GetHashFn(header, func(hash common.Hash, number uint64) *types.Header { return rawdb.ReadHeader(sdb.tx, hash, number) })
		blockContext := core.NewEVMBlockContext(header, getHashFn, cfg.engine, &cfg.zk.AddressSequencer, parentBlock.ExcessDataGas())
		batchState.blockState.builtBlockElements.resetBlockBuildingArrays()

		parentRoot := parentBlock.Root()
		if err = handleStateForNewBlockStarting(
			cfg.chainConfig,
			sdb.hermezDb,
			ibs,
			blockNumber,
			batchState.batchNumber,
			header.Time,
			&parentRoot,
			l1TreeUpdate,
			shouldWriteGerToContract,
		); err != nil {
			return err
		}

		// start waiting for a new transaction to arrive
		if !isAnyRecovery {
			log.Info(fmt.Sprintf("[%s] Waiting for txs from the pool...", logPrefix))
		}

		// we don't care about defer order here we just need to make sure the tickers are stopped to
		// avoid a leak
		logTicker := time.NewTicker(10 * time.Second)
		defer logTicker.Stop()
		blockTicker := time.NewTicker(cfg.zk.SequencerBlockSealTime)
		defer blockTicker.Stop()

	LOOP_TRANSACTIONS:
		for {
			select {
			case <-logTicker.C:
				if !isAnyRecovery {
					log.Info(fmt.Sprintf("[%s] Waiting some more for txs from the pool...", logPrefix))
				}
			case <-blockTicker.C:
				if !isAnyRecovery {
					break LOOP_TRANSACTIONS
				}
			case <-batchTicker.C:
				if !isAnyRecovery {
					runLoopBlocks = false
					break LOOP_TRANSACTIONS
				}
			case <-nonEmptyBatchTimer.C:
				if !isAnyRecovery && batchState.hasAnyTransactionsInThisBatch {
					runLoopBlocks = false
					break LOOP_TRANSACTIONS
				}
			default:
				if limboRecovery {
					batchState.blockState.transactionsForInclusion, err = getLimboTransaction(ctx, cfg, limboTxHash)
					if err != nil {
						return err
					}
				} else if !batchState.isL1Recovery() {
					batchState.blockState.transactionsForInclusion, err = getNextPoolTransactions(ctx, cfg, executionAt, batchState.forkId, batchState.yieldedTransactions)
					if err != nil {
						return err
					}
				}

				if len(batchState.blockState.transactionsForInclusion) == 0 {
					time.Sleep(250 * time.Millisecond)
				} else {
					log.Trace(fmt.Sprintf("[%s] Yielded transactions from the pool", logPrefix), "txCount", len(batchState.blockState.transactionsForInclusion))
				}

				var receipt *types.Receipt
				var execResult *core.ExecutionResult
				for i, transaction := range batchState.blockState.transactionsForInclusion {
					txHash := transaction.Hash()
					effectiveGas := batchState.blockState.getL1EffectiveGases(cfg, i)

					// The copying of this structure is intentional
					backupDataSizeChecker := *blockDataSizeChecker
					if receipt, execResult, anyOverflow, err = attemptAddTransaction(cfg, sdb, ibs, batchCounters, &blockContext, header, transaction, effectiveGas, batchState.isL1Recovery(), batchState.forkId, l1InfoIndex, &backupDataSizeChecker); err != nil {
						if limboRecovery {
							panic("limbo transaction has already been executed once so they must not fail while re-executing")
						}

						// if we are in recovery just log the error as a warning.  If the data is on the L1 then we should consider it as confirmed.
						// The executor/prover would simply skip a TX with an invalid nonce for example so we don't need to worry about that here.
						if batchState.isL1Recovery() {
							log.Warn(fmt.Sprintf("[%s] error adding transaction to batch during recovery: %v", logPrefix, err),
								"hash", txHash,
								"to", transaction.GetTo(),
							)
							continue
						}

						// if running in normal operation mode and error != nil then just allow the code to continue
						// It is safe because this approach ensures that the problematic transaction (the one that caused err != nil to be returned) is kept in yielded
						// Each transaction in yielded will be reevaluated at the end of each batch
					}

					if anyOverflow {
						if limboRecovery {
							panic("limbo transaction has already been executed once so they must not overflow counters while re-executing")
						}

						if !batchState.isL1Recovery() {
							log.Info(fmt.Sprintf("[%s] overflowed adding transaction to batch", logPrefix), "batch", batchState.batchNumber, "tx-hash", txHash, "has any transactions in this batch", batchState.hasAnyTransactionsInThisBatch)
							/*
								There are two cases when overflow could occur.
								1. The block DOES not contains any transactions.
									In this case it means that a single tx overflow entire zk-counters.
									In this case we mark it so. Once marked it will be discarded from the tx-pool async (once the tx-pool process the creation of a new batch)
									NB: The tx SHOULD not be removed from yielded set, because if removed, it will be picked again on next block. That's why there is i++. It ensures that removing from yielded will start after the problematic tx
								2. The block contains transactions.
									In this case, we just have to remove the transaction that overflowed the zk-counters and all transactions after it, from the yielded set.
									This removal will ensure that these transaction could be added in the next block(s)
							*/
							if !batchState.hasAnyTransactionsInThisBatch {
								cfg.txPool.MarkForDiscardFromPendingBest(txHash)
								log.Trace(fmt.Sprintf("single transaction %s overflow counters", txHash))
							}
						}

						//TODO: Why do we break the loop in case of l1Recovery?!
						break LOOP_TRANSACTIONS
					}

					if err == nil {
						blockDataSizeChecker = &backupDataSizeChecker
						//TODO: Does no make any sense to remove last added tx
						batchState.yieldedTransactions.Remove(txHash)
						batchState.onAddedTransaction(transaction, receipt, execResult, effectiveGas)

						nonEmptyBatchTimer.Reset(cfg.zk.SequencerNonEmptyBatchSealTime)
					}
				}

				if batchState.isL1Recovery() {
					// just go into the normal loop waiting for new transactions to signal that the recovery
					// has finished as far as it can go
					if batchState.isThereAnyTransactionsToRecover() {
						log.Info(fmt.Sprintf("[%s] L1 recovery no more transactions to recover", logPrefix))
					}

					break LOOP_TRANSACTIONS
				}

				if limboRecovery {
					runLoopBlocks = false
					break LOOP_TRANSACTIONS
				}
			}
		}

		if err = sdb.hermezDb.WriteBlockL1InfoTreeIndex(blockNumber, l1TreeUpdateIndex); err != nil {
			return err
		}

		block, err = doFinishBlockAndUpdateState(batchContext, ibs, header, parentBlock, batchState, ger, l1BlockHash, infoTreeIndexProgress)
		if err != nil {
			return err
		}

		if limboRecovery {
			stateRoot := block.Root()
			cfg.txPool.UpdateLimboRootByTxHash(limboTxHash, &stateRoot)
			return fmt.Errorf("[%s] %w: %s = %s", s.LogPrefix(), zk.ErrLimboState, limboTxHash.Hex(), stateRoot.Hex())
		}

		t.LogTimer()
		gasPerSecond := float64(0)
		elapsedSeconds := t.Elapsed().Seconds()
		if elapsedSeconds != 0 {
			gasPerSecond = float64(block.GasUsed()) / elapsedSeconds
		}

		if gasPerSecond != 0 {
			log.Info(fmt.Sprintf("[%s] Finish block %d with %d transactions... (%d gas/s)", logPrefix, blockNumber, len(batchState.blockState.builtBlockElements.transactions), int(gasPerSecond)))
		} else {
			log.Info(fmt.Sprintf("[%s] Finish block %d with %d transactions...", logPrefix, blockNumber, len(batchState.blockState.builtBlockElements.transactions)))
		}

		err = sdb.hermezDb.WriteBatchCounters(batchState.batchNumber, batchCounters.CombineCollectorsNoChanges().UsedAsMap())
		if err != nil {
			return err
		}

		err = sdb.hermezDb.WriteIsBatchPartiallyProcessed(batchState.batchNumber)
		if err != nil {
			return err
		}

		if err = sdb.CommitAndStart(); err != nil {
			return err
		}
		defer sdb.tx.Rollback()

		// add a check to the verifier and also check for responses
		batchState.onBuiltBlock(blockNumber)
		batchVerifier.AddNewCheck(batchState.batchNumber, blockNumber, block.Root(), batchCounters.CombineCollectorsNoChanges().UsedAsMap(), batchState.builtBlocks)

		// check for new responses from the verifier
		needsUnwind, _, err := updateStreamAndCheckRollback(batchContext, batchState, streamWriter, u)
		if err != nil || needsUnwind {
			return err
		}
	}

	for {
		needsUnwind, remaining, err := updateStreamAndCheckRollback(batchContext, batchState, streamWriter, u)
		if err != nil || needsUnwind {
			return err
		}
		if remaining == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if err = runBatchLastSteps(batchContext, batchState.batchNumber, block.NumberU64(), batchCounters); err != nil {
		return err
	}

	return sdb.tx.Commit()
}
