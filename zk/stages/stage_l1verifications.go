package stages

import (
	"context"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon/sync_stages"
	"github.com/ledgerwatch/log/v3"
)

type L1VerificationsCfg struct {
	db     kv.RwDB
	syncer ISyncer
}

func StageL1VerificationsCfg(db kv.RwDB, syncer ISyncer) L1VerificationsCfg {
	return L1VerificationsCfg{
		db:     db,
		syncer: syncer,
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

	// TODO: implement l1 verifications stage!
	// Get all l1 verifications as far as we can from the last point we started (stage progress needs saving)
	// check the interhashes progress, and compare the stateroot we hold here with the one we calculated in interhashes. If they don't match we should unwind...

	log.Info("L1 verifications stage")
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
