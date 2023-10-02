package stagedsync

import (
	"context"

	"fmt"

	"github.com/ledgerwatch/erigon-lib/kv"
	state2 "github.com/ledgerwatch/erigon/core/state"
	"github.com/ledgerwatch/erigon/eth/stagedsync/stages"
	"github.com/ledgerwatch/erigon/zkevm/adapter"
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

	// MODIFY THIS
	/*
		- get batches from the stream
		- if we get a reorg - we should unwind the whole of erigon to the unwind point then resume forward
		- if we detect a fork we should enable features (this should just happen at point of requirement by calling the hermez_db funcs)
		- for a range of batches we should also retrieve the l1 logs and store sequences, verifications, and global exit roots
	*/

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
	restrictAtBatch := uint64(20) //(858670)

	for {
		select {
		case <-ctx.Done():
			_, err := manageTx(tx, false)
			return err
		default:
		}

		chunkSize := uint64(5000)
		count++

		if restrictAtBatch != 0 {
			chunkSize = 500
		}

		prg := cfg.zkSynchronizer.GetProgress(tx)
		fmt.Printf("zk stage headers.go: prg: %+v\n", prg)

		// move along when at the tip
		if prg.HighestL2SequencedBatch == prg.LocalSyncedL2SequencedBatch {
			_, err := manageTx(tx, false)
			return err
		}

		// DEBUG: don't go to etherman if we're already at the restriction point
		if restrictAtBatch != 0 && prg.LocalSyncedL2VerifiedBatch >= restrictAtBatch {

			// TODO: remove unwind test code
			//up := &UnwindState{
			//	UnwindPoint: restrictAtBatch - 20,
			//}
			//
			//u.UnwindTo(up.UnwindPoint, up.BadBlock)

			_, err := manageTx(tx, false)
			return err
		}

		// sync verified batches first
		if !useExternalTx && prg.LocalSyncedL2VerifiedBatch <= prg.HighestL2VerifiedBatch &&
			prg.LocalSyncedL1Block < prg.HighestL1Block {

			//if we're at the tip, move erigon to the next stage
			//tip will be defined here by being at the highest verified l2 batch
			if prg.LocalSyncedL2VerifiedBatch == prg.HighestL2VerifiedBatch {
				_, err := manageTx(tx, false)
				return err
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

			if count%saveEvery == 0 && !useExternalTx {
				var err error
				tx, err = manageTx(tx, true)
				if err != nil {
					return err
				}
			}

			continue // ensure we get all verified before we move to checking for reorgs
		}

		// sync beyond verified batches, checking for reorgs
		//if prg.HighestL2SequencedBatch > prg.LocalSyncedL2SequencedBatch {
		//	l1Block, reorg, err := cfg.zkSynchronizer.SyncTip(tx, chunkSize, prg)
		//	if err != nil {
		//		log.Error("failed to sync tip", "err", err)
		//	}
		//
		//	if reorg {
		//		unwindBlock := l1Block
		//		unwindBatch := &state.Batch{}
		//		// not so efficient but should be rare
		//		err = tx.ForEach("HermezBatch", []byte{}, func(k, v []byte) error {
		//
		//			_, l1BlockNo := adapter.ParseBatchKey(k)
		//			if l1BlockNo != unwindBlock {
		//				return nil
		//			}
		//
		//			b := &state.Batch{}
		//			err := json.Unmarshal(v, b)
		//			if err != nil {
		//				return err
		//			}
		//
		//			unwindBatch = b
		//
		//			return nil
		//		})
		//
		//		u.UnwindTo(unwindBatch.BatchNumber, unwindBatch.StateRoot)
		//
		//		fmt.Println("reorg: unwinding stages to l1block: ", l1Block)
		//		return nil
		//	}
		//
		//	err = stages.SaveStageProgress(tx, stages.L1Blocks, l1Block)
		//	if err != nil {
		//		if !useExternalTx {
		//			tx.Rollback()
		//		}
		//		return err
		//	}
		//}

		if count%saveEvery == 0 && !useExternalTx {
			var err error
			tx, err = manageTx(tx, true)
			if err != nil {
				return err
			}
		}
	}
}

func ZkHeadersUnwind(u *UnwindState, s *StageState, tx kv.RwTx, cfg HeadersCfg, test bool) (err error) {
	useExternalTx := tx != nil
	if !useExternalTx {
		tx, err = cfg.db.BeginRw(context.Background())
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}

	to := u.UnwindPoint

	iter, err := tx.Prefix("HermezBatch", state2.UintBytes(to))
	if err != nil {
		return err
	}
	k, _, err := iter.Next()
	if err != nil {
		return err
	}

	// get the l1block no from the key
	_, l1 := adapter.ParseBatchKey(k)

	highestVerifiedBatch, err := adapter.GetHermezMeta(tx, adapter.HermezVerifiedBatch)
	if err != nil {
		return err
	}
	if to <= highestVerifiedBatch {
		log.Info("unwinding verified batches", "to", to, "highest verified batch", highestVerifiedBatch)
	}

	log.Info("unwinding headers", "to", to, "l1 block", l1)

	// unwind verified batches if unwind goes below the highest verified batch
	if to < highestVerifiedBatch {
		err = UnwindTable(tx, "HermezVerifiedBatch", to)
		if err != nil {
			return err
		}
		err = adapter.PutHermezMeta(tx, adapter.HermezVerifiedBatch, to)
		if err != nil {
			return err
		}
	}

	// Meta_HermezMeta
	err = adapter.PutHermezMeta(tx, adapter.META_Batch, to)
	if err != nil {
		return err
	}

	// HermezGlobalExitRoot
	err = UnwindTable(tx, "HermezGlobalExitRoot", to)
	if err != nil {
		return err
	}

	// HermezL1Blocks
	err = UnwindTable(tx, "HermezL1Block", l1)
	if err != nil {
		return err
	}

	// HermezBatch Table
	err = UnwindTable(tx, "HermezBatch", to)
	if err != nil {
		return err
	}

	// Headers Table
	err = UnwindTable(tx, kv.Headers, to)
	if err != nil {
		return err
	}

	// Headers Canonical
	err = UnwindTable(tx, kv.HeaderCanonical, to)
	if err != nil {
		return err
	}

	// Bodies Table
	err = UnwindTable(tx, kv.BlockBody, to)
	if err != nil {
		return err
	}

	// Stage progress
	err = stages.SaveStageProgress(tx, stages.L1Blocks, l1)
	if err != nil {
		return err
	}
	err = stages.SaveStageProgress(tx, stages.Headers, to)
	if err != nil {
		return err
	}
	err = stages.SaveStageProgress(tx, stages.Bodies, to)
	if err != nil {
		return err
	}

	if !useExternalTx {
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func UnwindTable(tx kv.RwTx, table string, point uint64) error {
	uwp := state2.UintBytes(point + 1) // +1 as we don't want to unwind the 'to' point
	return tx.ForEach(table, uwp, func(k, v []byte) error {
		if table == "HermezBatch" {
			b, l1 := adapter.ParseBatchKey(k)
			fmt.Println("unwinding batch: ", b, "l1: ", l1)
		}
		return tx.Delete(table, k)
	})
}
