package stages

import (
	"context"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon/sync_stages"
	"github.com/ledgerwatch/log/v3"
)

type ISyncer interface {
}

type BatchesCfg struct {
	db     kv.RwDB
	syncer ISyncer
}

func StageBatchesCfg(db kv.RwDB, syncer ISyncer) BatchesCfg {
	return BatchesCfg{
		db:     db,
		syncer: syncer,
	}
}

func SpawnStageBatches(
	s *sync_stages.StageState,
	u sync_stages.Unwinder,
	ctx context.Context,
	tx kv.RwTx,
	cfg BatchesCfg,
	firstCycle bool,
	quiet bool,
) error {
	// TODO: implement batches stage!

	log.Info("Batches stage")
	return nil
}

func UnwindBatchesStage(u *sync_stages.UnwindState, tx kv.RwTx, cfg BatchesCfg, ctx context.Context) (err error) {
	useExternalTx := tx != nil
	if !useExternalTx {
		tx, err = cfg.db.BeginRw(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}

	// TODO: implement unwind batches stage!

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

func PruneBatchesStage(s *sync_stages.PruneState, tx kv.RwTx, cfg BatchesCfg, ctx context.Context) (err error) {
	useExternalTx := tx != nil
	if !useExternalTx {
		tx, err = cfg.db.BeginRw(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}

	// TODO: implement prune batches stage! (if required)

	if !useExternalTx {
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}
