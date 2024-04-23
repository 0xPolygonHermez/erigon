package stages

import (
	"context"

	"errors"

	"github.com/ledgerwatch/erigon/core/state"
	"github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/core/vm"
	"github.com/ledgerwatch/erigon/eth/stagedsync"
	"github.com/ledgerwatch/erigon/zk/tx"
	zktypes "github.com/ledgerwatch/erigon/zk/types"
)

const (
	injectedBatchNumber      = 1
	injectedBatchBlockNumber = 1
)

func processInjectedInitialBatch(
	ctx context.Context,
	cfg SequenceBlockCfg,
	s *stagedsync.StageState,
	sdb *stageDb,
	forkId uint64,
	header *types.Header,
	parentBlock *types.Block,
) error {
	injected, err := sdb.hermezDb.GetL1InjectedBatch(0)
	if err != nil {
		return err
	}

	fakeL1TreeUpdate := &zktypes.L1InfoTreeUpdate{
		GER:        injected.LastGlobalExitRoot,
		ParentHash: injected.L1ParentHash,
		Timestamp:  injected.Timestamp,
	}

	ibs := state.New(sdb.stateReader)

	// the injected batch block timestamp should also match that of the injected batch
	header.Time = injected.Timestamp

	parentRoot := parentBlock.Root()
	if err = handleStateForNewBlockStarting(cfg.chainConfig, sdb.hermezDb, ibs, injectedBatchBlockNumber, injected.Timestamp, &parentRoot, fakeL1TreeUpdate); err != nil {
		return err
	}

	txn, receipt, err := handleInjectedBatch(cfg, sdb, ibs, injected, header, parentBlock, forkId)
	if err != nil {
		return err
	}

	txns := types.Transactions{*txn}
	receipts := types.Receipts{receipt}
	return doFinishBlockAndUpdateState(ctx, cfg, s, sdb, ibs, header, parentBlock, forkId, injectedBatchNumber, injected.LastGlobalExitRoot, injected.L1ParentHash, txns, receipts, 0)
}

func handleInjectedBatch(
	cfg SequenceBlockCfg,
	sdb *stageDb,
	ibs *state.IntraBlockState,
	injected *zktypes.L1InjectedBatch,
	header *types.Header,
	parentBlock *types.Block,
	forkId uint64,
) (*types.Transaction, *types.Receipt, error) {
	txs, _, _, err := tx.DecodeTxs(injected.Transaction, 5)
	if err != nil {
		return nil, nil, err
	}
	if len(txs) == 0 || len(txs) > 1 {
		return nil, nil, errors.New("expected 1 transaction in the injected batch")
	}

	batchCounters := vm.NewBatchCounterCollector(sdb.smt.GetDepth(), uint16(forkId))

	// process the tx and we can ignore the counters as an overflow at this stage means no network anyway
	effectiveGas := DeriveEffectiveGasPrice(cfg, txs[0])
	receipt, _, err := attemptAddTransaction(cfg, sdb, ibs, batchCounters, header, parentBlock.Header(), txs[0], effectiveGas)
	if err != nil {
		return nil, nil, err
	}

	return &txs[0], receipt, nil
}
