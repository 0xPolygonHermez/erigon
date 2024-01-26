package stages

import (
	"context"
	"fmt"
	"github.com/iden3/go-iden3-crypto/keccak256"
	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon/eth/ethconfig"
	"github.com/ledgerwatch/erigon/eth/stagedsync"
	"github.com/ledgerwatch/erigon/eth/stagedsync/stages"
	"github.com/ledgerwatch/erigon/zk/hermez_db"
	"github.com/ledgerwatch/erigon/zk/syncer"
	"github.com/ledgerwatch/erigon/zk/types"
	"github.com/ledgerwatch/log/v3"
)

type L1InfoTreeCfg struct {
	db     kv.RwDB
	zkCfg  *ethconfig.Zk
	syncer *syncer.L1Syncer
}

func StageL1InfoTreeCfg(db kv.RwDB, zkCfg *ethconfig.Zk, sync *syncer.L1Syncer) L1InfoTreeCfg {
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
	initialCycle bool,
	quiet bool,
) (err error) {
	logPrefix := s.LogPrefix()
	log.Info(fmt.Sprintf("[%s] Starting L1 Info Tree stage", logPrefix))
	defer log.Info(fmt.Sprintf("[%s] Finished L1 Info Tree stage", logPrefix))

	freshTx := tx == nil
	if freshTx {
		tx, err = cfg.db.BeginRw(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}

	hermezDb, err := hermez_db.NewHermezDb(tx)
	if err != nil {
		return err
	}

	progress, err := stages.GetStageProgress(tx, stages.L1InfoTree)
	if err != nil {
		return err
	}
	if progress == 0 {
		progress = cfg.zkCfg.L1FirstBlock - 1
	}

	latestUpdate, found, err := hermezDb.GetLatestL1InfoTreeUpdate()
	if err != nil {
		return err
	}

	if !cfg.syncer.IsSyncStarted() {
		cfg.syncer.Run(progress)
	}

	logChan := cfg.syncer.GetLogsChan()
	progressChan := cfg.syncer.GetProgressMessageChan()

Loop:
	for {
		select {
		case l := <-logChan:
			if len(l.Topics) != 3 {
				log.Warn("Received log for info tree that did not have 3 topics")
				continue
			}
			mainnetExitRoot := l.Topics[1]
			rollupExitRoot := l.Topics[2]
			combined := append(mainnetExitRoot.Bytes(), rollupExitRoot.Bytes()...)
			ger := keccak256.Hash(combined)
			update := types.L1InfoTreeUpdate{
				GER:             common.BytesToHash(ger),
				MainnetExitRoot: mainnetExitRoot,
				RollupExitRoot:  rollupExitRoot,
			}

			if !found {
				// starting from a fresh db here, so we need to create index 0
				update.Index = 0
				found = true
			} else {
				// increment the index from the previous entry
				update.Index = latestUpdate.Index + 1
			}

			// now we need the block timestamp and the parent hash information for the block tied
			// to this event
			block, err := cfg.syncer.GetBlock(l.BlockNumber)
			if err != nil {
				return err
			}
			update.ParentHash = block.ParentHash()
			update.Timestamp = block.Time()

			latestUpdate = update

			if err = hermezDb.WriteL1InfoTreeUpdate(update); err != nil {
				return err
			}

		case progMsg := <-progressChan:
			log.Info(fmt.Sprintf("[%s] %s", logPrefix, progMsg))
		default:
			if !cfg.syncer.IsDownloading() {
				break Loop
			}
		}
	}

	progress = cfg.syncer.GetLastCheckedL1Block()
	if err = stages.SaveStageProgress(tx, stages.L1InfoTree, progress); err != nil {
		return err
	}

	if freshTx {
		if err = tx.Commit(); err != nil {
			return err
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
