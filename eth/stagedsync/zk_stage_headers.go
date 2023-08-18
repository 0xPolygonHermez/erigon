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

func commitAndReturnNewTx(ctx context.Context, db kv.RwDB, tx kv.RwTx) (kv.RwTx, error) {
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	tx, err := db.BeginRw(ctx)
	if err != nil {
		return nil, err
	}

	return tx, err
}

// HeadersPOW progresses Headers stage for Proof-of-Work headers
func HeadersZK(
	s *StageState,
	u Unwinder,
	ctx context.Context,
	tx kv.RwTx,
	cfg HeadersCfg,
	initialCycle bool,
	test bool, // Set to true in tests, allows the stage to fail rather than wait indefinitely
) error {

	useExternalTx := tx != nil
	if !useExternalTx {
		var err error
		tx, err = cfg.db.BeginRw(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}

	// add to config - chunk size is how many l1 blocks to take
	chunkSize := uint64(100)
	restrictAtBatch := uint64(0)
	saveEvery := 1

	// probably we should loop here with our commit logic etc. - and then the synchronizer should be a bit dumb without loop logic etc

	count := 0

	// loop for (desired amount of progress before commit):
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		count++

		if count%saveEvery == 0 && !useExternalTx {
			var err error
			err = tx.Commit()
			if err != nil {
				return err
			}
			tx, err = cfg.db.BeginRw(ctx)
			if err != nil {
				return err
			}
		}

		// helper func gets network + local progress for l1 l2 and verified/sequenced batches
		prg := cfg.zkSynchronizer.GetProgress(tx)
		fmt.Printf("zk stage headers.go: prg: %+v\n", prg)

		// DEBUG: don't go to etherman if we're already at the restriction point
		if restrictAtBatch != 0 && prg.LocalSyncedL2VerifiedBatch >= restrictAtBatch {
			break
		}

		// sync verified batches behind the tip
		if prg.LocalSyncedL2VerifiedBatch < prg.HighestL2VerifiedBatch {
			l1Block, err := cfg.zkSynchronizer.SyncPreTip(tx, chunkSize, prg)
			if err != nil {
				return err
			}
			err = stages.SaveStageProgress(tx, stages.L1Blocks, l1Block)
			if err != nil {
				return err
			}
		}

		// TODO:
		// this next bit should call the trusted batch sync to get batches from the trusted sequencer and put them into the db tables (batches, txs)
		// we can then execute those txs and update the state trie in memory - on reorg we can unwind execution, and these tables (naively re-sync them from common ancestor of the forks)
		// next loop iteration will pick up any that have been verified and execute/calculate state trie into the DB

		// update progress
		prg = cfg.zkSynchronizer.GetProgress(tx)

		// we're at the tip - i.e. sequenced batches are coming in - we should ingest them, and execute them
		if prg.LocalSyncedL2VerifiedBatch == prg.HighestL2VerifiedBatch {
			if err := cfg.zkSynchronizer.SyncTip(tx, prg); err != nil {
				return err
			}
		}
	}

	if !useExternalTx {
		if err := tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}
