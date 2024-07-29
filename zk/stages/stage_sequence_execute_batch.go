package stages

import (
	"fmt"

	"github.com/ledgerwatch/erigon/core/rawdb"
	"github.com/ledgerwatch/erigon/core/vm"
	"github.com/ledgerwatch/erigon/eth/stagedsync"
	"github.com/ledgerwatch/erigon/eth/stagedsync/stages"
	"github.com/ledgerwatch/erigon/zk/datastream/server"
	"github.com/ledgerwatch/erigon/zk/utils"
	"github.com/ledgerwatch/log/v3"
)

func prepareBatchNumber(lastBatch uint64, isLastBatchPariallyProcessed bool) uint64 {
	if isLastBatchPariallyProcessed {
		return lastBatch
	}

	return lastBatch + 1
}

func prepareBatchCounters(cfg *SequenceBlockCfg, sdb *stageDb, thisBatch, forkId uint64, isLastBatchPariallyProcessed, l1Recovery bool) (*vm.BatchCounterCollector, error) {
	var intermediateUsedCounters *vm.Counters
	if isLastBatchPariallyProcessed {
		intermediateCountersMap, found, err := sdb.hermezDb.GetBatchCounters(thisBatch)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("intermediate counters not found for batch %d", thisBatch)
		}

		intermediateUsedCounters = vm.NewCountersFromUsedMap(intermediateCountersMap)
	}

	return vm.NewBatchCounterCollector(sdb.smt.GetDepth(), uint16(forkId), cfg.zk.VirtualCountersSmtReduction, cfg.zk.ShouldCountersBeUnlimited(l1Recovery), intermediateUsedCounters), nil
}

func doInstantCloseIfNeeded(logPrefix string, cfg *SequenceBlockCfg, sdb *stageDb, thisBatch, forkId uint64, batchCounters *vm.BatchCounterCollector) (bool, error) {
	instantClose, err := sdb.hermezDb.GetJustUnwound(thisBatch)
	if err != nil || !instantClose {
		return false, err // err here could be nil as well
	}

	if err = sdb.hermezDb.DeleteJustUnwound(thisBatch); err != nil {
		return false, err
	}

	// lets first check if we actually wrote any blocks in this batch
	blocks, err := sdb.hermezDb.GetL2BlockNosByBatch(thisBatch)
	if err != nil {
		return false, err
	}

	// only close this batch down if we actually made any progress in it, otherwise
	// just continue processing as normal and recreate the batch from scratch
	if len(blocks) > 0 {
		if err = runBatchLastSteps(logPrefix, cfg.datastreamServer, sdb, thisBatch, blocks[len(blocks)-1], batchCounters); err != nil {
			return false, err
		}
		if err = stages.SaveStageProgress(sdb.tx, stages.HighestSeenBatchNumber, thisBatch); err != nil {
			return false, err
		}
		if err = sdb.hermezDb.WriteForkId(thisBatch, forkId); err != nil {
			return false, err
		}

		if err = sdb.tx.Commit(); err != nil {
			return false, err
		}

		return true, nil
	}

	return false, nil
}

func doCheckForBadBatch(logPrefix string, sdb *stageDb, l1rd *L1RecoveryData, thisBlock, thisBatch, forkId uint64) (bool, error) {
	infoTreeIndex, err := l1rd.getInfoTreeIndex(sdb)
	if err != nil {
		return false, err
	}

	// now let's detect a bad batch and skip it if we have to
	currentBlock, err := rawdb.ReadBlockByNumber(sdb.tx, thisBlock)
	if err != nil {
		return false, err
	}

	badBatch, err := checkForBadBatch(thisBatch, sdb.hermezDb, currentBlock.Time(), infoTreeIndex, l1rd.nextBatchData.LimitTimestamp, l1rd.nextBatchData.DecodedData)
	if err != nil {
		return false, err
	}

	if !badBatch {
		return false, nil
	}

	log.Info(fmt.Sprintf("[%s] Skipping bad batch %d...", logPrefix, thisBatch))
	// store the fact that this batch was invalid during recovery - will be used for the stream later
	if err = sdb.hermezDb.WriteInvalidBatch(thisBatch); err != nil {
		return false, err
	}
	if err = sdb.hermezDb.WriteBatchCounters(thisBatch, map[string]int{}); err != nil {
		return false, err
	}
	if err = sdb.hermezDb.DeleteIsBatchPartiallyProcessed(thisBatch); err != nil {
		return false, err
	}
	if err = stages.SaveStageProgress(sdb.tx, stages.HighestSeenBatchNumber, thisBatch); err != nil {
		return false, err
	}
	if err = sdb.hermezDb.WriteForkId(thisBatch, forkId); err != nil {
		return false, err
	}
	if err = sdb.tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func updateStreamAndCheckRollback(
	logPrefix string,
	sdb *stageDb,
	streamWriter *SequencerBatchStreamWriter,
	batchNumber uint64,
	forkId uint64,
	u stagedsync.Unwinder,
) (bool, int, error) {
	committed, remaining, err := streamWriter.CommitNewUpdates(forkId)
	if err != nil {
		return false, remaining, err
	}
	for _, commit := range committed {
		if !commit.Valid {
			// we are about to unwind so place the marker ready for this to happen
			if err = sdb.hermezDb.WriteJustUnwound(batchNumber); err != nil {
				return false, 0, err
			}
			// capture the fork otherwise when the loop starts again to close
			// off the batch it will detect it as a fork upgrade
			if err = sdb.hermezDb.WriteForkId(batchNumber, forkId); err != nil {
				return false, 0, err
			}

			unwindTo := commit.BlockNumber - 1

			// for unwind we supply the block number X-1 of the block we want to remove, but supply the hash of the block
			// causing the unwind.
			unwindHeader := rawdb.ReadHeaderByNumber(sdb.tx, commit.BlockNumber)
			if unwindHeader == nil {
				return false, 0, fmt.Errorf("could not find header for block %d", commit.BlockNumber)
			}

			if err = sdb.tx.Commit(); err != nil {
				return false, 0, err
			}

			log.Warn(fmt.Sprintf("[%s] Block is invalid - rolling back", logPrefix), "badBlock", commit.BlockNumber, "unwindTo", unwindTo, "root", unwindHeader.Root)

			u.UnwindTo(unwindTo, unwindHeader.Hash())
			return true, 0, nil
		}
	}

	return false, remaining, nil
}

func runBatchLastSteps(
	logPrefix string,
	datastreamServer *server.DataStreamServer,
	sdb *stageDb,
	thisBatch uint64,
	lastStartedBn uint64,
	batchCounters *vm.BatchCounterCollector,
) error {
	l1InfoIndex, err := sdb.hermezDb.GetBlockL1InfoTreeIndex(lastStartedBn)
	if err != nil {
		return err
	}

	counters, err := batchCounters.CombineCollectors(l1InfoIndex != 0)
	if err != nil {
		return err
	}

	log.Info(fmt.Sprintf("[%s] counters consumed", logPrefix), "batch", thisBatch, "counts", counters.UsedAsString())

	if err = sdb.hermezDb.WriteBatchCounters(thisBatch, counters.UsedAsMap()); err != nil {
		return err
	}
	if err := sdb.hermezDb.DeleteIsBatchPartiallyProcessed(thisBatch); err != nil {
		return err
	}

	// Local Exit Root (ler): read s/c storage every batch to store the LER for the highest block in the batch
	ler, err := utils.GetBatchLocalExitRootFromSCStorage(thisBatch, sdb.hermezDb.HermezDbReader, sdb.tx)
	if err != nil {
		return err
	}
	// write ler to hermezdb
	if err = sdb.hermezDb.WriteLocalExitRootForBatchNo(thisBatch, ler); err != nil {
		return err
	}

	lastBlock, err := sdb.hermezDb.GetHighestBlockInBatch(thisBatch)
	if err != nil {
		return err
	}
	block, err := rawdb.ReadBlockByNumber(sdb.tx, lastBlock)
	if err != nil {
		return err
	}
	blockRoot := block.Root()
	if err = datastreamServer.WriteBatchEnd(sdb.hermezDb, thisBatch, thisBatch, &blockRoot, &ler); err != nil {
		return err
	}

	log.Info(fmt.Sprintf("[%s] Finish batch %d...", logPrefix, thisBatch))

	return nil
}
