package stagedsync

import (
	"context"

	"fmt"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon/eth/stagedsync/stages"
	"github.com/ledgerwatch/log/v3"
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

	//tx, _ = cfg.db.BeginRw(ctx)
	//stages.SaveStageProgress(tx, stages.L1Blocks, 17977261)
	//tx.Commit()

	useExternalTx := tx != nil

	manageTx := func(currentTx kv.RwTx, new bool) (kv.RwTx, error) {
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
		if new {
			return cfg.db.BeginRw(ctx)
		}
		return nil, nil
	}

	if !useExternalTx {
		var err error
		tx, err = manageTx(nil, true)
		if err != nil {
			return err
		}
	}

	saveEvery := 1
	count := 0
	restrictAtBatch := uint64(0) // chunkSize may take us further than this batch if set sufficiently high

	for {
		select {
		case <-ctx.Done():
			_, err := manageTx(tx, false)
			return err
		default:
		}

		chunkSize := uint64(500)
		count++

		prg := cfg.zkSynchronizer.GetProgress(tx)
		fmt.Printf("zk stage headers.go: prg: %+v\n", prg)

		// DEBUG: don't go to etherman if we're already at the restriction point
		if restrictAtBatch != 0 && prg.LocalSyncedL2VerifiedBatch >= restrictAtBatch {
			_, err := manageTx(tx, false)
			return err
		}

		// sync from L1
		if prg.LocalSyncedL2VerifiedBatch < prg.HighestL2VerifiedBatch &&
			prg.LocalSyncedL1Block < prg.HighestL1Block {

			// prevent overshooting the tip
			if chunkSize > prg.HighestL1Block-prg.LocalSyncedL1Block {
				chunkSize = 1 // over cautious - update it
			}

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

			// if we're at the tip, move erigon to the next stage
			if l1Block == prg.HighestL1Block {
				_, err := manageTx(tx, false)
				return err
			}
		}

		// sync trusted state from L2 when L1 is synced
		if prg.LocalSyncedL2VerifiedBatch >= prg.HighestL2VerifiedBatch && prg.LocalSyncedL2SequencedBatch < prg.HighestL2SequencedBatch {
			// sync the tip from the l2
			// todo: manage the blocks here
			err := cfg.zkSynchronizer.SyncTip(tx, prg)
			if err != nil {
				log.Error("failed to sync tip", "err", err)
			}
		}

		if count%saveEvery == 0 && !useExternalTx {
			var err error
			tx, err = manageTx(tx, true)
			if err != nil {
				return err
			}
		}
	}
}
