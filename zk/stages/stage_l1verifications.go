package stages

import (
	"context"
	"fmt"

	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon/sync_stages"
	"github.com/ledgerwatch/erigon/zk/hermez_db"
	"github.com/ledgerwatch/erigon/zk/types"
	"github.com/ledgerwatch/log/v3"
)

type IVerificationsSyncer interface {
	GetVerifications(startBlock uint64) (verifications []types.L1BatchInfo, highestL1Block uint64, err error)
}

type L1VerificationsCfg struct {
	db     kv.RwDB
	syncer IVerificationsSyncer

	firstL1Block uint64
}

func StageL1VerificationsCfg(db kv.RwDB, syncer IVerificationsSyncer, firstL1Block uint64) L1VerificationsCfg {
	return L1VerificationsCfg{
		db:           db,
		syncer:       syncer,
		firstL1Block: firstL1Block,
	}
}

func SpawnStageL1Verifications(
	s *sync_stages.StageState,
	u sync_stages.Unwinder,
	ctx context.Context,
	tx kv.RwTx,
	cfg L1VerificationsCfg,
	firstCycle bool,
	quiet bool,
) error {

	logPrefix := s.LogPrefix()
	log.Info(fmt.Sprintf("[%s] Starting L1 Verifications download stage", logPrefix))
	defer log.Info(fmt.Sprintf("[%s] Finished L1 Verifications download stage ", logPrefix))

	if tx == nil {
		log.Debug("l1 verifications: no tx provided, creating a new one")
		var err error
		tx, err = cfg.db.BeginRw(ctx)
		if err != nil {
			return fmt.Errorf("failed to open tx, %w", err)
		}
		defer tx.Rollback()
	}

	// pass tx to the hermezdb
	hermezDb, err := hermez_db.NewHermezDb(tx)
	if err != nil {
		return fmt.Errorf("failed to create hermezdb, %w", err)
	}

	// get l1 block progress from this stage's progress
	l1BlockProgress, err := sync_stages.GetStageProgress(tx, s.ID)
	if err != nil {
		return fmt.Errorf("failed to get l1 progress block, %w", err)
	}

	if l1BlockProgress == 0 {
		l1BlockProgress = cfg.firstL1Block - 1
	}

	// get as many verifications as we can from the network
	verifications, highestL1Block, err := cfg.syncer.GetVerifications(l1BlockProgress + 1)
	if err != nil {
		return fmt.Errorf("failed to get l1 verifications from network, %w", err)
	}

	highestL2BatchNo := uint64(0)
	// write verifications to the hermezdb
	for _, verification := range verifications {
		if verification.BatchNo > highestL2BatchNo {
			highestL2BatchNo = verification.BatchNo
		}
		err = hermezDb.WriteVerification(verification.L1BlockNo, verification.BatchNo, verification.L1TxHash, verification.StateRoot)
		if err != nil {
			return fmt.Errorf("failed to write verification for block %s, %w", verification.L1BlockNo, err)
		}
	}

	// update stage progress - highest l1 block containing a verification
	err = sync_stages.SaveStageProgress(tx, s.ID, highestL1Block)
	err = sync_stages.SaveStageProgress(tx, sync_stages.L1VerificationsBatchNo, highestL2BatchNo)

	// TODO: check the latest interhashed block and compare the stateroot if we have one for it

	if firstCycle {
		log.Debug("l1 verifications: first cycle, committing tx")
		err = tx.Commit()
		if err != nil {
			return fmt.Errorf("failed to commit tx, %w", err)
		}
	}

	log.Debug("L1 verifications stage")
	return nil
}

func UnwindL1VerificationsStage(u *sync_stages.UnwindState, tx kv.RwTx, cfg L1VerificationsCfg, ctx context.Context) (err error) {
	useExternalTx := tx != nil
	if !useExternalTx {
		tx, err = cfg.db.BeginRw(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}

	// TODO: implement unwind verifications stage!

	if err = u.Done(tx); err != nil {
		return err
	}
	if !useExternalTx {
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func PruneL1VerificationsStage(s *sync_stages.PruneState, tx kv.RwTx, cfg L1VerificationsCfg, ctx context.Context) (err error) {
	useExternalTx := tx != nil
	if !useExternalTx {
		tx, err = cfg.db.BeginRw(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}

	// TODO: implement prune L1 Verifications stage! (if required)

	if !useExternalTx {
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
