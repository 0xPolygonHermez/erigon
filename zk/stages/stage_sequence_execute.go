package stages

import (
	"context"
	"fmt"
	"time"

	"github.com/gateway-fm/cdk-erigon-lib/common"
	"github.com/gateway-fm/cdk-erigon-lib/kv"
	"github.com/ledgerwatch/log/v3"

	mapset "github.com/deckarep/golang-set/v2"
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

	l1Recovery := cfg.zk.L1SyncStartBlock > 0
	thisBatch := prepareBatchNumber(lastBatch, isLastBatchPariallyProcessed)

	// injected batch
	if executionAt == 0 {
		if err = processInjectedInitialBatch(ctx, cfg, s, sdb, forkId, l1Recovery); err != nil {
			return err
		}

		if err = cfg.datastreamServer.WriteWholeBatchToStream(logPrefix, sdb.tx, sdb.hermezDb.HermezDbReader, lastBatch, injectedBatchNumber); err != nil {
			return err
		}

		if err = sdb.tx.Commit(); err != nil {
			return err
		}

		return nil
	}

	// handle case where batch wasn't closed properly
	// close it before starting a new one
	// this occurs when sequencer was switched from syncer or sequencer datastream files were deleted
	// and datastream was regenerated
	if err = finalizeLastBatchInDatastreamIfNotFinalized(logPrefix, sdb, cfg.datastreamServer, lastBatch, executionAt); err != nil {
		return err
	}

	if err := utils.UpdateZkEVMBlockCfg(cfg.chainConfig, sdb.hermezDb, logPrefix); err != nil {
		return err
	}

	batchCounters, err := prepareBatchCounters(&cfg, sdb, thisBatch, forkId, isLastBatchPariallyProcessed, l1Recovery)
	if err != nil {
		return err
	}

	// check if we just unwound from a bad executor response and if we did just close the batch here
	handled, err := doInstantCloseIfNeeded(logPrefix, &cfg, sdb, thisBatch, forkId, batchCounters)
	if err != nil || handled {
		return err // err here could be nil as well
	}

	batchTicker := time.NewTicker(cfg.zk.SequencerBatchSealTime)
	defer batchTicker.Stop()
	nonEmptyBatchTimer := time.NewTicker(cfg.zk.SequencerNonEmptyBatchSealTime)
	defer nonEmptyBatchTimer.Stop()

	var builtBlocks []uint64

	hasExecutorForThisBatch := !isLastBatchPariallyProcessed && cfg.zk.HasExecutors()
	hasAnyTransactionsInThisBatch := false
	runLoopBlocks := true
	yielded := mapset.NewSet[[32]byte]()
	blockState := newBlockState(l1Recovery)

	batchVerifier := NewBatchVerifier(cfg.zk, hasExecutorForThisBatch, cfg.legacyVerifier, forkId)
	streamWriter := &SequencerBatchStreamWriter{
		ctx:           ctx,
		db:            cfg.db,
		logPrefix:     logPrefix,
		batchVerifier: batchVerifier,
		sdb:           sdb,
		streamServer:  cfg.datastreamServer,
		hasExecutors:  hasExecutorForThisBatch,
		lastBatch:     lastBatch,
	}

	blockDataSizeChecker := NewBlockDataChecker()

	prevHeader := rawdb.ReadHeaderByNumber(sdb.tx, executionAt)
	batchDataOverflow := false

	limboHeaderTimestamp, limboTxHash := cfg.txPool.GetLimboTxHash(thisBatch)
	limboRecovery := limboTxHash != nil
	isAnyRecovery := l1Recovery || limboRecovery

	// if not limbo set the limboHeaderTimestamp to the "default" value for "prepareHeader" function
	if !limboRecovery {
		limboHeaderTimestamp = math.MaxUint64
	}

	if l1Recovery {
		if cfg.zk.L1SyncStopBatch > 0 && thisBatch > cfg.zk.L1SyncStopBatch {
			log.Info(fmt.Sprintf("[%s] L1 recovery has completed!", logPrefix), "batch", thisBatch)
			time.Sleep(1 * time.Second)
			return nil
		}

		// let's check if we have any L1 data to recover
		if err = blockState.l1RecoveryData.loadNextBatchData(sdb, thisBatch, forkId); err != nil {
			return err
		}

		if !blockState.l1RecoveryData.hasAnyDecodedBlocks() {
			log.Info(fmt.Sprintf("[%s] L1 recovery has completed!", logPrefix), "batch", thisBatch)
			time.Sleep(1 * time.Second)
			return nil
		}

		if handled, err := doCheckForBadBatch(logPrefix, sdb, blockState.l1RecoveryData, executionAt, thisBatch, forkId); err != nil || handled {
			return err
		}
	}

	if !isLastBatchPariallyProcessed {
		log.Info(fmt.Sprintf("[%s] Starting batch %d...", logPrefix, thisBatch))
	} else {
		log.Info(fmt.Sprintf("[%s] Continuing unfinished batch %d from block %d", logPrefix, thisBatch, executionAt))
	}

	var block *types.Block
	for blockNumber := executionAt + 1; runLoopBlocks; blockNumber++ {
		log.Info(fmt.Sprintf("[%s] Starting block %d (forkid %v)...", logPrefix, blockNumber, forkId))

		if l1Recovery {
			didLoadedAnyDataForRecovery := blockState.loadDataByDecodedBlockIndex(blockNumber - (executionAt + 1))
			if !didLoadedAnyDataForRecovery {
				runLoopBlocks = false
				break
			}
		}

		l1InfoIndex, err := sdb.hermezDb.GetBlockL1InfoTreeIndex(blockNumber - 1)
		if err != nil {
			return err
		}

		header, parentBlock, err := prepareHeader(sdb.tx, blockNumber-1, blockState.getDeltaTimestamp(), limboHeaderTimestamp, forkId, blockState.getCoinbase(&cfg))
		if err != nil {
			return err
		}

		if batchDataOverflow = blockDataSizeChecker.AddBlockStartData(uint32(prevHeader.Time-header.Time), uint32(l1InfoIndex)); batchDataOverflow {
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

		infoTreeIndexProgress, l1TreeUpdate, l1TreeUpdateIndex, l1BlockHash, ger, shouldWriteGerToContract, err := prepareL1AndInfoTreeRelatedStuff(sdb, blockState, header.Time)
		if err != nil {
			return err
		}

		var anyOverflow bool
		ibs := state.New(sdb.stateReader)
		getHashFn := core.GetHashFn(header, func(hash common.Hash, number uint64) *types.Header { return rawdb.ReadHeader(sdb.tx, hash, number) })
		blockContext := core.NewEVMBlockContext(header, getHashFn, cfg.engine, &cfg.zk.AddressSequencer, parentBlock.ExcessDataGas())
		blockState.usedBlockElements.resetBlockBuildingArrays()

		parentRoot := parentBlock.Root()
		if err = handleStateForNewBlockStarting(
			cfg.chainConfig,
			sdb.hermezDb,
			ibs,
			blockNumber,
			thisBatch,
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
				if !isAnyRecovery && hasAnyTransactionsInThisBatch {
					runLoopBlocks = false
					break LOOP_TRANSACTIONS
				}
			default:
				if limboRecovery {
					blockState.blockTransactions, err = getLimboTransaction(ctx, cfg, limboTxHash)
					if err != nil {
						return err
					}
				} else if !l1Recovery {
					blockState.blockTransactions, err = getNextPoolTransactions(ctx, cfg, executionAt, forkId, yielded)
					if err != nil {
						return err
					}
				}

				if len(blockState.blockTransactions) == 0 {
					time.Sleep(250 * time.Millisecond)
				} else {
					log.Trace(fmt.Sprintf("[%s] Yielded transactions from the pool", logPrefix), "txCount", len(blockState.blockTransactions))
				}

				var receipt *types.Receipt
				var execResult *core.ExecutionResult
				for i, transaction := range blockState.blockTransactions {
					txHash := transaction.Hash()
					effectiveGas := blockState.getL1EffectiveGases(cfg, i)

					// The copying of this structure is intentional
					backupDataSizeChecker := *blockDataSizeChecker
					if receipt, execResult, anyOverflow, err = attemptAddTransaction(cfg, sdb, ibs, batchCounters, &blockContext, header, transaction, effectiveGas, l1Recovery, forkId, l1InfoIndex, &backupDataSizeChecker); err != nil {
						if limboRecovery {
							panic("limbo transaction has already been executed once so they must not fail while re-executing")
						}

						// if we are in recovery just log the error as a warning.  If the data is on the L1 then we should consider it as confirmed.
						// The executor/prover would simply skip a TX with an invalid nonce for example so we don't need to worry about that here.
						if l1Recovery {
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

						if !l1Recovery {
							log.Info(fmt.Sprintf("[%s] overflowed adding transaction to batch", logPrefix), "batch", thisBatch, "tx-hash", txHash, "has any transactions in this batch", hasAnyTransactionsInThisBatch)
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
							if !hasAnyTransactionsInThisBatch {
								cfg.txPool.MarkForDiscardFromPendingBest(txHash)
								log.Trace(fmt.Sprintf("single transaction %s overflow counters", txHash))
							}
						}

						//TODO: Why do we break the loop in case of l1Recovery?!
						break LOOP_TRANSACTIONS
					}

					if err == nil {
						blockDataSizeChecker = &backupDataSizeChecker
						yielded.Remove(txHash)
						blockState.usedBlockElements.onFinishAddingTransaction(transaction, receipt, execResult, effectiveGas)

						hasAnyTransactionsInThisBatch = true
						nonEmptyBatchTimer.Reset(cfg.zk.SequencerNonEmptyBatchSealTime)
						log.Debug(fmt.Sprintf("[%s] Finish block %d with %s transaction", logPrefix, blockNumber, txHash.Hex()))
					}
				}

				if l1Recovery {
					// just go into the normal loop waiting for new transactions to signal that the recovery
					// has finished as far as it can go
					if blockState.isThereAnyTransactionsToRecover() {
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

		block, err = doFinishBlockAndUpdateState(ctx, cfg, s, sdb, ibs, header, parentBlock, forkId, thisBatch, ger, l1BlockHash, &blockState.usedBlockElements, infoTreeIndexProgress, l1Recovery)
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
			log.Info(fmt.Sprintf("[%s] Finish block %d with %d transactions... (%d gas/s)", logPrefix, blockNumber, len(blockState.usedBlockElements.transactions), int(gasPerSecond)))
		} else {
			log.Info(fmt.Sprintf("[%s] Finish block %d with %d transactions...", logPrefix, blockNumber, len(blockState.usedBlockElements.transactions)))
		}

		err = sdb.hermezDb.WriteBatchCounters(thisBatch, batchCounters.CombineCollectorsNoChanges().UsedAsMap())
		if err != nil {
			return err
		}

		err = sdb.hermezDb.WriteIsBatchPartiallyProcessed(thisBatch)
		if err != nil {
			return err
		}

		if err = sdb.CommitAndStart(); err != nil {
			return err
		}
		defer sdb.tx.Rollback()

		// add a check to the verifier and also check for responses
		builtBlocks = append(builtBlocks, blockNumber)
		batchVerifier.AddNewCheck(thisBatch, blockNumber, block.Root(), batchCounters.CombineCollectorsNoChanges().UsedAsMap(), builtBlocks)

		// check for new responses from the verifier
		needsUnwind, _, err := updateStreamAndCheckRollback(logPrefix, sdb, streamWriter, thisBatch, forkId, u)
		if err != nil {
			return err
		}
		if needsUnwind {
			return nil
		}
	}

	for {
		needsUnwind, remaining, err := updateStreamAndCheckRollback(logPrefix, sdb, streamWriter, thisBatch, forkId, u)
		if err != nil {
			return err
		}
		if needsUnwind {
			return nil
		}
		if remaining == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if err = runBatchLastSteps(logPrefix, cfg.datastreamServer, sdb, thisBatch, block.NumberU64(), batchCounters); err != nil {
		return err
	}

	if err = sdb.tx.Commit(); err != nil {
		return err
	}

	return nil
}
