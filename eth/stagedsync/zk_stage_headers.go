package stagedsync

import (
	"context"

	"fmt"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon/eth/stagedsync/stages"
)

type ZkProgress struct {
	HighestL1Block          uint64
	HighestL2VerifiedBatch  uint64
	HighestL2SequencedBatch uint64

	LocalSyncedL1Block          uint64
	LocalSyncedL2VerifiedBatch  uint64
	LocalSyncedL2SequencedBatch uint64
}

// HeadersPOW progresses Headers stage for Proof-of-Work headers
func HeadersZK(
	s *StageState,
	u Unwinder,
	ctx context.Context,
	tx kv.RwTx,
	cfg HeadersCfg,
	initialCycle bool,
	test bool,
) error {

	useExternalTx := tx != nil

	manageTx := func(currentTx kv.RwTx) (kv.RwTx, error) {
		// don't mess with the tx if using external one
		if useExternalTx {
			return nil, nil
		}
		// otherwise commit it, and start a new one
		if currentTx != nil {
			if err := currentTx.Commit(); err != nil {
				currentTx.Rollback()
				return nil, err
			}
		}
		return cfg.db.BeginRw(ctx)
	}

	if !useExternalTx {
		var err error
		tx, err = manageTx(nil)
		if err != nil {
			return err
		}
	}

	chunkSize := uint64(2000)
	saveEvery := 5
	count := 0

	for {
		select {
		case <-ctx.Done():
			_, err := manageTx(tx)
			return err
		default:
		}
		count++

		prg := cfg.zkSynchronizer.GetProgress(tx)
		fmt.Printf("zk stage headers.go: prg: %+v\n", prg)

		if prg.LocalSyncedL2VerifiedBatch < prg.HighestL2VerifiedBatch {
			l1Block, err := cfg.zkSynchronizer.SyncPreTip(tx, chunkSize, prg)
			if err != nil {
				if !useExternalTx {
					tx.Rollback()
				}
				return err
			}
			err = stages.SaveStageProgress(tx, stages.L1Blocks, l1Block)
			if err != nil {
				if !useExternalTx {
					tx.Rollback()
				}
				return err
			}
		}

		//if prg.LocalSyncedL2SequencedBatch < prg.HighestL2SequencedBatch {
		//	// sync the tip from the l2
		//	l2block, err := cfg.zkSynchronizer.SyncTip(tx, chunkSize, prg)
		//	if err != nil {
		//
		//	}
		//}

		if count%saveEvery == 0 && !useExternalTx {
			var err error
			tx, err = manageTx(tx)
			if err != nil {
				return err
			}
		}
	}
}
