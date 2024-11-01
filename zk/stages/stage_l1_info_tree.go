package stages

import (
	"context"
	"fmt"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon/eth/ethconfig"
	"github.com/ledgerwatch/erigon/eth/stagedsync"
	"github.com/ledgerwatch/erigon/zk/l1infotree"
	"github.com/ledgerwatch/log/v3"
)

type L1InfoTreeCfg struct {
	db     kv.RwDB
	zkCfg  *ethconfig.Zk
	syncer IL1Syncer
}

func StageL1InfoTreeCfg(db kv.RwDB, zkCfg *ethconfig.Zk, sync IL1Syncer) L1InfoTreeCfg {
	return L1InfoTreeCfg{
		db:     db,
		zkCfg:  zkCfg,
		syncer: sync,
	}
}

func SpawnL1InfoTreeStage(
	s *stagedsync.StageState,
	u stagedsync.Unwinder,
	tx kv.RwTx,
	cfg L1InfoTreeCfg,
	ctx context.Context,
	logger log.Logger,
) (funcErr error) {
	logPrefix := s.LogPrefix()
	log.Info(fmt.Sprintf("[%s] Starting L1 Info Tree stage", logPrefix))
	defer log.Info(fmt.Sprintf("[%s] Finished L1 Info Tree stage", logPrefix))

	freshTx := tx == nil
	if freshTx {
		var err error
		tx, err = cfg.db.BeginRw(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}

	updater := l1infotree.NewUpdater(cfg.zkCfg, cfg.syncer)
	if err := updater.WarmUp(tx); err != nil {
		return err
	}

	defer func() {
		if funcErr != nil {
			cfg.syncer.StopQueryBlocks()
			cfg.syncer.ConsumeQueryBlocks()
			cfg.syncer.WaitQueryBlocksToFinish()
		}
	}()

	allLogs, err := updater.CheckForInfoTreeUpdates(logPrefix, tx)
	if err != nil {
		return err
	}

	log.Info(fmt.Sprintf("[%s] Info tree updates", logPrefix), "count", len(allLogs))

	if freshTx {
		if funcErr = tx.Commit(); funcErr != nil {
			return funcErr
		}
	}

	return nil
}

func UnwindL1InfoTreeStage(u *stagedsync.UnwindState, tx kv.RwTx, cfg L1InfoTreeCfg, ctx context.Context) error {
	return nil
}

func PruneL1InfoTreeStage(s *stagedsync.PruneState, tx kv.RwTx, cfg L1InfoTreeCfg, ctx context.Context) error {
	return nil
}
