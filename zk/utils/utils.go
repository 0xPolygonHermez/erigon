package utils

import (
	"fmt"

	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon/sync_stages"
	"github.com/ledgerwatch/erigon/zk/hermez_db"
)

func ShouldShortCircuitExecution(tx kv.RwTx) (bool, uint64, error) {
	intersProgress, err := sync_stages.GetStageProgress(tx, sync_stages.IntermediateHashes)
	if err != nil {
		return false, 0, err
	}

	hermezDb, err := hermez_db.NewHermezDb(tx)
	if err != nil {
		return false, 0, err
	}

	// if there is no inters progress - i.e. first sync, don't skip exec, and execute to the highest block in the highest verified batch
	if intersProgress == 0 {
		highestVerifiedBatchNo, err := sync_stages.GetStageProgress(tx, sync_stages.L1VerificationsBatchNo)
		if err != nil {
			return false, 0, err
		}

		// we could find ourselves with a batch with no blocks here, so we want to go back one batch at
		// a time until we find a batch with blocks
		max := uint64(0)
		killSwitch := 0
		for {
			max, err = hermezDb.GetHighestBlockInBatch(highestVerifiedBatchNo)
			if err != nil {
				return false, 0, err
			}
			if max != 0 {
				break
			}
			highestVerifiedBatchNo--
			killSwitch++
			if killSwitch > 100 {
				return false, 0, fmt.Errorf("could not find a batch with blocks when checking short circuit")
			}
		}

		return false, max, nil
	}

	highestHashableL2BlockNo, err := sync_stages.GetStageProgress(tx, sync_stages.HighestHashableL2BlockNo)
	if err != nil {
		return false, 0, err
	}
	highestHashableBatchNo, err := hermezDb.GetBatchNoByL2Block(highestHashableL2BlockNo)
	if err != nil {
		return false, 0, err
	}
	intersProgressBatchNo, err := hermezDb.GetBatchNoByL2Block(intersProgress)
	if err != nil {
		return false, 0, err
	}

	// check to skip execution: 1. there is inters progress, 2. the inters progress is less than the highest hashable, 3. we're in the tip batch range
	if intersProgress != 0 && intersProgress < highestHashableL2BlockNo && highestHashableBatchNo-intersProgressBatchNo <= 1 {
		return true, highestHashableL2BlockNo, nil
	}

	return false, 0, nil
}

func ShouldIncrementInterHashes(tx kv.RwTx) (bool, uint64, error) {
	return ShouldShortCircuitExecution(tx)
}
