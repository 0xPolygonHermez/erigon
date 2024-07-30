package stages

import (
	"context"

	"github.com/gateway-fm/cdk-erigon-lib/kv"
	"github.com/ledgerwatch/erigon/eth/stagedsync"
)

// type SequencerInterhashesCfg struct {
// 	db          kv.RwDB
// 	accumulator *shards.Accumulator
// }

// func StageSequencerInterhashesCfg(
// 	db kv.RwDB,
// 	accumulator *shards.Accumulator,
// ) SequencerInterhashesCfg {
// 	return SequencerInterhashesCfg{
// 		db:          db,
// 		accumulator: accumulator,
// 	}
// }

// This stages does NOTHING while going forward, because its done during execution
// Even this stage progress is updated in execution stage
func SpawnSequencerInterhashesStage(
	s *stagedsync.StageState,
	u stagedsync.Unwinder,
	tx kv.RwTx,
	ctx context.Context,
	cfg ZkInterHashesCfg,
	quiet bool,
) error {
	// var err error

	// freshTx := tx == nil
	// if freshTx {
	// 	tx, err = cfg.db.BeginRw(ctx)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	defer tx.Rollback()
	// }

	// to, err := s.ExecutionAt(tx)
	// if err != nil {
	// 	return err
	// }

	// if err := s.Update(tx, to); err != nil {
	// 	return err
	// }

	// if freshTx {
	// 	if err = tx.Commit(); err != nil {
	// 		return err
	// 	}
	// }

	return nil
}

// The unwind of interhashes must happen separate from execution’s unwind although execution includes interhashes while going forward.
// This is because interhashes MUST be unwound before history/calltraces while execution MUST be after history/calltraces
func UnwindSequencerInterhashsStage(
	u *stagedsync.UnwindState,
	s *stagedsync.StageState,
	tx kv.RwTx,
	ctx context.Context,
	cfg ZkInterHashesCfg,
) error {
	return UnwindZkIntermediateHashesStage(u, s, tx, cfg, ctx)
}

func PruneSequencerInterhashesStage(
	s *stagedsync.PruneState,
	tx kv.RwTx,
	cfg ZkInterHashesCfg,
	ctx context.Context,
) error {
	return nil
}
