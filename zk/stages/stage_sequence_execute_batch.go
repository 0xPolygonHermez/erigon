package stages

import (
	"fmt"
	"time"

	"github.com/ledgerwatch/erigon/core/rawdb"
	"github.com/ledgerwatch/erigon/core/vm"
	"github.com/ledgerwatch/erigon/eth/stagedsync"
	"github.com/ledgerwatch/erigon/eth/stagedsync/stages"
	"github.com/ledgerwatch/erigon/zk/utils"
	"github.com/ledgerwatch/log/v3"
)

func prepareBatchNumber(lastBatch uint64, isLastBatchPariallyProcessed bool) uint64 {
	if isLastBatchPariallyProcessed {
		return lastBatch
	}

	return lastBatch + 1
}

func prepareBatchCounters(batchContext *BatchContext, batchState *BatchState, isLastBatchPariallyProcessed bool) (*vm.BatchCounterCollector, error) {
	var intermediateUsedCounters *vm.Counters
	if isLastBatchPariallyProcessed {
		intermediateCountersMap, found, err := batchContext.sdb.hermezDb.GetBatchCounters(batchState.batchNumber)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("intermediate counters not found for batch %d", batchState.batchNumber)
		}

		intermediateUsedCounters = vm.NewCountersFromUsedMap(intermediateCountersMap)
	}

	return vm.NewBatchCounterCollector(batchContext.sdb.smt.GetDepth(), uint16(batchState.forkId), batchContext.cfg.zk.VirtualCountersSmtReduction, batchContext.cfg.zk.ShouldCountersBeUnlimited(batchState.isL1Recovery()), intermediateUsedCounters), nil
}

func doCheckForBadBatch(batchContext *BatchContext, batchState *BatchState, thisBlock uint64) (bool, error) {
	infoTreeIndex, err := batchState.batchL1RecoveryData.getInfoTreeIndex(batchContext.sdb)
	if err != nil {
		return false, err
	}

	// now let's detect a bad batch and skip it if we have to
	currentBlock, err := rawdb.ReadBlockByNumber(batchContext.sdb.tx, thisBlock)
	if err != nil {
		return false, err
	}

	badBatch, err := checkForBadBatch(batchState.batchNumber, batchContext.sdb.hermezDb, currentBlock.Time(), infoTreeIndex, batchState.batchL1RecoveryData.recoveredBatchData.LimitTimestamp, batchState.batchL1RecoveryData.recoveredBatchData.DecodedData)
	if err != nil {
		return false, err
	}

	if !badBatch {
		return false, nil
	}

	log.Info(fmt.Sprintf("[%s] Skipping bad batch %d...", batchContext.s.LogPrefix(), batchState.batchNumber))
	// store the fact that this batch was invalid during recovery - will be used for the stream later
	if err = batchContext.sdb.hermezDb.WriteInvalidBatch(batchState.batchNumber); err != nil {
		return false, err
	}
	if err = batchContext.sdb.hermezDb.WriteBatchCounters(batchState.batchNumber, map[string]int{}); err != nil {
		return false, err
	}
	if err = batchContext.sdb.hermezDb.DeleteIsBatchPartiallyProcessed(batchState.batchNumber); err != nil {
		return false, err
	}
	if err = stages.SaveStageProgress(batchContext.sdb.tx, stages.HighestSeenBatchNumber, batchState.batchNumber); err != nil {
		return false, err
	}
	if err = batchContext.sdb.hermezDb.WriteForkId(batchState.batchNumber, batchState.forkId); err != nil {
		return false, err
	}
	if err = batchContext.sdb.tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func updateStreamAndCheckRollback(
	batchContext *BatchContext,
	batchState *BatchState,
	streamWriter *SequencerBatchStreamWriter,
	u stagedsync.Unwinder,
) (bool, error) {
	checkedVerifierBundles, err := streamWriter.CommitNewUpdates()
	if err != nil {
		return false, err
	}

	infiniteLoop := func(batchNumber uint64) {
		// this infinite loop will make the node to print the error once every minute therefore preventing it for creating new blocks
		for {
			log.Error(fmt.Sprintf("[%s] identified an invalid batch with number %d", batchContext.s.LogPrefix(), batchNumber))
			time.Sleep(time.Minute)
		}
	}

	for _, verifierBundle := range checkedVerifierBundles {
		if verifierBundle.Response.Valid {
			continue
		}

		// updateStreamAndCheckRollback cannot be invoked during l1 recovery so no point to check it
		if batchState.isL1Recovery() || !batchContext.cfg.zk.Limbo {
			infiniteLoop(verifierBundle.Request.BatchNumber)
		}

		if err = handleLimbo(batchContext, batchState, verifierBundle); err != nil {
			return false, err
		}

		unwindTo := verifierBundle.Request.GetLastBlockNumber() - 1

		// for unwind we supply the block number X-1 of the block we want to remove, but supply the hash of the block
		// causing the unwind.
		unwindHeader := rawdb.ReadHeaderByNumber(batchContext.sdb.tx, verifierBundle.Request.GetLastBlockNumber())
		if unwindHeader == nil {
			return false, fmt.Errorf("could not find header for block %d", verifierBundle.Request.GetLastBlockNumber())
		}

		log.Warn(fmt.Sprintf("[%s] Block is invalid - rolling back", batchContext.s.LogPrefix()), "badBlock", verifierBundle.Request.GetLastBlockNumber(), "unwindTo", unwindTo, "root", unwindHeader.Root)

		u.UnwindTo(unwindTo, unwindHeader.Hash())
		streamWriter.legacyVerifier.CancelAllRequests()
		return true, nil
	}

	return false, nil
}

func runBatchLastSteps(
	batchContext *BatchContext,
	thisBatch uint64,
	lastStartedBn uint64,
	batchCounters *vm.BatchCounterCollector,
) error {
	l1InfoIndex, err := batchContext.sdb.hermezDb.GetBlockL1InfoTreeIndex(lastStartedBn)
	if err != nil {
		return err
	}

	counters, err := batchCounters.CombineCollectors(l1InfoIndex != 0)
	if err != nil {
		return err
	}

	log.Info(fmt.Sprintf("[%s] counters consumed", batchContext.s.LogPrefix()), "batch", thisBatch, "counts", counters.UsedAsString())

	if err = batchContext.sdb.hermezDb.WriteBatchCounters(thisBatch, counters.UsedAsMap()); err != nil {
		return err
	}
	if err := batchContext.sdb.hermezDb.DeleteIsBatchPartiallyProcessed(thisBatch); err != nil {
		return err
	}

	// Local Exit Root (ler): read s/c storage every batch to store the LER for the highest block in the batch
	ler, err := utils.GetBatchLocalExitRootFromSCStorage(thisBatch, batchContext.sdb.hermezDb.HermezDbReader, batchContext.sdb.tx)
	if err != nil {
		return err
	}
	// write ler to hermezdb
	if err = batchContext.sdb.hermezDb.WriteLocalExitRootForBatchNo(thisBatch, ler); err != nil {
		return err
	}

	lastBlock, err := batchContext.sdb.hermezDb.GetHighestBlockInBatch(thisBatch)
	if err != nil {
		return err
	}
	block, err := rawdb.ReadBlockByNumber(batchContext.sdb.tx, lastBlock)
	if err != nil {
		return err
	}
	blockRoot := block.Root()
	if err = batchContext.cfg.datastreamServer.WriteBatchEnd(batchContext.sdb.hermezDb, thisBatch, &blockRoot, &ler); err != nil {
		return err
	}

	log.Info(fmt.Sprintf("[%s] Finish batch %d...", batchContext.s.LogPrefix(), thisBatch))

	return nil
}
