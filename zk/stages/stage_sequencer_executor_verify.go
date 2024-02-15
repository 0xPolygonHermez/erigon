package stages

import (
	"context"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon/cmd/rpcdaemon/commands"
	"github.com/ledgerwatch/erigon/eth/stagedsync"
	"github.com/ledgerwatch/erigon/rpc"
	"github.com/ledgerwatch/erigon/zk/erigon_db"
	"github.com/ledgerwatch/erigon/zk/legacy_executor_verifier"
	"github.com/ledgerwatch/erigon/zk/txpool"
)

type SequencerExecutorVerifyCfg struct {
	db       kv.RwDB
	verifier *legacy_executor_verifier.LegacyExecutorVerifier
	api      commands.ZkEvmAPI
	txPool   *txpool.TxPool
}

func StageSequencerExecutorVerifyCfg(
	db kv.RwDB,
	verifier *legacy_executor_verifier.LegacyExecutorVerifier,
	api commands.ZkEvmAPI,
) SequencerExecutorVerifyCfg {
	return SequencerExecutorVerifyCfg{
		db:       db,
		verifier: verifier,
		api:      api,
	}
}

func SpawnSequencerExecutorVerifyStage(
	s *stagedsync.StageState,
	u stagedsync.Unwinder,
	tx kv.RwTx,
	ctx context.Context,
	cfg SequencerExecutorVerifyCfg,
	initialCycle bool,
	quiet bool,
) error {
	var err error
	freshTx := tx == nil
	if freshTx {
		tx, err = cfg.db.BeginRw(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}

	// come here and verify what's happened
	// on error you throw everything out - unwind to tx[startBatch]
	// stage should short-circuit back to sequence on error, removing from the txpool all but the 1st tx in the batch
	// set next stage as sequencer
	// otherwise proceed to datastream stage and let it create the stream of the created batch

	// test the batch building (this could be done async)
	// tx1
	// tx1, tx2
	// tx1, tx2, tx3
	// when it errors - then we know that is the tx to remove

	// if someone finds a tx which fails they could launch an attack

	// how do i remove from tx pool here?
	// txpool delete call
	cfg.txPool.

	// TODO: read the channel of 'verified batches'
	// we can't consume a bad tx here

	/*
			1. hold open the TX from the previous stage (sequencer execute)
			2. get the state root from the previous stage, get the witness etc. etc.
			3. build payload including witness, stream data etc.
			4. call legacyExecutorVerifier.VerifyWithAvailableExecutor(p, sr)
			5. if verified - commit the tx, update the state stage (to be used in the next stage of populating the datastream)
			6. if not verified (we are now trying to find the bad tx)
				- unwind everything and begin processing the batch 1tx at a time
		        -
	*/

	// create batches
	// -- async: check them against the old executors
	// stage pre-datastream -> read from the chan and confirm the results

	// WITNESS - get the witness (hack: has to re-execute using trie_db.go rather than IBS)
	startBlock := rpc.BlockNumberOrHashWithNumber(0)
	endBlock := rpc.BlockNumberOrHashWithNumber(10)
	debug := true
	witness, err := cfg.api.GetBlockRangeWitness(ctx, startBlock, endBlock, &debug)
	if err != nil {
		return err
	}

	// get datastream

	// build payload including witness, stream data etc.

	// TODO: finish implementing this call (build payload, get expected stateroot)
	success, err := cfg.verifier.VerifyWithAvailableExecutor(p, sr)
	if err != nil {
		return err
	}

	to, err := s.ExecutionAt(tx)
	if err != nil {
		return err
	}

	erigonDb := erigon_db.NewErigonDb(tx)

	// TODO: implement

	if freshTx {
		if err = tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}

func UnwindSequencerExecutorVerifyStage(
	u *stagedsync.UnwindState,
	s *stagedsync.StageState,
	tx kv.RwTx,
	ctx context.Context,
	cfg SequencerExecutorVerifyCfg,
	initialCycle bool,
) error {
	return nil
}

func PruneSequencerExecutorVerifyStage(
	s *stagedsync.PruneState,
	tx kv.RwTx,
	cfg SequencerExecutorVerifyCfg,
	ctx context.Context,
	initialCycle bool,
) error {
	return nil
}
