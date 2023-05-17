package adapter

import (
	"context"

	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon/core/rawdb"
	"github.com/ledgerwatch/erigon/eth/stagedsync/stages"
	"github.com/ledgerwatch/erigon/zkevm/state"
	"github.com/ledgerwatch/erigon/zkevm/state/metrics"
	"github.com/ledgerwatch/erigon/zkevm/state/runtime/executor/pb"

	ethTypes "github.com/ledgerwatch/erigon/core/types"
)

// write me an adapter mock for stateInterface from zk_synchronizer.go
// Interface
type stateInterface interface {
	GetLastBlock(ctx context.Context, dbTx kv.RwTx) (*state.Block, error)
	AddGlobalExitRoot(ctx context.Context, exitRoot *state.GlobalExitRoot, dbTx kv.RwTx) error
	AddForcedBatch(ctx context.Context, forcedBatch *state.ForcedBatch, dbTx kv.RwTx) error
	AddBlock(ctx context.Context, block *state.Block, dbTx kv.RwTx) error
	AddVirtualBatch(ctx context.Context, virtualBatch *state.VirtualBatch, dbTx kv.RwTx) error
	GetPreviousBlock(ctx context.Context, offset uint64, dbTx kv.RwTx) (*state.Block, error)
	GetLastBatchNumber(ctx context.Context, dbTx kv.RwTx) (uint64, error)
	GetBatchByNumber(ctx context.Context, batchNumber uint64, dbTx kv.RwTx) (*state.Batch, error)
	ResetTrustedState(ctx context.Context, batchNumber uint64, dbTx kv.RwTx) error
	GetNextForcedBatches(ctx context.Context, nextForcedBatches int, dbTx kv.RwTx) ([]state.ForcedBatch, error)
	AddVerifiedBatch(ctx context.Context, verifiedBatch *state.VerifiedBatch, dbTx kv.RwTx) error
	ProcessAndStoreClosedBatch(ctx context.Context, processingCtx state.ProcessingContext, encodedTxs []byte, dbTx kv.RwTx, caller metrics.CallerLabel) (common.Hash, error)
	OpenBatch(ctx context.Context, processingContext state.ProcessingContext, dbTx kv.RwTx) error
	CloseBatch(ctx context.Context, receipt state.ProcessingReceipt, dbTx kv.RwTx) error
	ProcessSequencerBatch(ctx context.Context, batchNumber uint64, batchL2Data []byte, caller metrics.CallerLabel, dbTx kv.RwTx) (*state.ProcessBatchResponse, error)
	StoreTransactions(ctx context.Context, batchNum uint64, processedTxs []*state.ProcessTransactionResponse, dbTx kv.RwTx) error
	GetStateRootByBatchNumber(ctx context.Context, batchNum uint64, dbTx kv.RwTx) (common.Hash, error)
	ExecuteBatch(ctx context.Context, batch state.Batch, updateMerkleTree bool, dbTx kv.RwTx) (*pb.ProcessBatchResponse, error)
	GetLastVerifiedBatch(ctx context.Context, dbTx kv.RwTx) (*state.VerifiedBatch, error)
	GetLastVirtualBatchNum(ctx context.Context, dbTx kv.RwTx) (uint64, error)
	AddSequence(ctx context.Context, sequence state.Sequence, dbTx kv.RwTx) error
	AddAccumulatedInputHash(ctx context.Context, batchNum uint64, accInputHash common.Hash, dbTx kv.RwTx) error
	AddTrustedReorg(ctx context.Context, trustedReorg *state.TrustedReorg, dbTx kv.RwTx) error
	GetReorgedTransactions(ctx context.Context, batchNumber uint64, dbTx kv.RwTx) ([]ethTypes.Transaction, error)
	ResetForkID(ctx context.Context, batchNumber, forkID uint64, version string, dbTx kv.RwTx) error
	GetForkIDTrustedReorgCount(ctx context.Context, forkID uint64, version string, dbTx kv.RwTx) (uint64, error)
	UpdateForkIDIntervals(intervals []state.ForkIDInterval)

	BeginStateTransaction(ctx context.Context) (kv.RwTx, error)
}

type StateInterfaceAdapter struct{}

var _ stateInterface = (*StateInterfaceAdapter)(nil)

func NewStateAdapter() stateInterface {
	return &StateInterfaceAdapter{}
}

func (m *StateInterfaceAdapter) GetLastBlock(ctx context.Context, dbTx kv.RwTx) (*state.Block, error) {
	blockHeight, err := stages.GetStageProgress(dbTx, stages.Bodies)
	if err != nil {
		return nil, err
	}
	canonicalHash, err := rawdb.ReadCanonicalHash(dbTx, blockHeight)
	if err != nil {
		return nil, err
	}
	block, _, err := rawdb.ReadBlockWithSenders(dbTx, canonicalHash, blockHeight)
	if err != nil {
		return nil, err
	}

	return &state.Block{
		BlockNumber: block.Header().Number.Uint64(),
		BlockHash:   canonicalHash,
		ParentHash:  block.Header().ParentHash,
	}, nil
}

func (m *StateInterfaceAdapter) AddGlobalExitRoot(ctx context.Context, exitRoot *state.GlobalExitRoot, dbTx kv.RwTx) error {
	panic("AddGlobalExitRoot: implement me")
}

func (m *StateInterfaceAdapter) AddForcedBatch(ctx context.Context, forcedBatch *state.ForcedBatch, dbTx kv.RwTx) error {
	panic("AddForcedBatch: implement me")
}

func (m *StateInterfaceAdapter) AddBlock(ctx context.Context, block *state.Block, dbTx kv.RwTx) error {
	panic("AddBlock: implement me")
}

func (m *StateInterfaceAdapter) AddVirtualBatch(ctx context.Context, virtualBatch *state.VirtualBatch, dbTx kv.RwTx) error {
	panic("AddVirtualBatch: implement me")
}

func (m *StateInterfaceAdapter) GetPreviousBlock(ctx context.Context, offset uint64, dbTx kv.RwTx) (*state.Block, error) {
	panic("GetPreviousBlock: implement me")
}

func (m *StateInterfaceAdapter) GetLastBatchNumber(ctx context.Context, dbTx kv.RwTx) (uint64, error) {
	panic("GetLastBatchNumber: implement me")
}

func (m *StateInterfaceAdapter) GetBatchByNumber(ctx context.Context, batchNumber uint64, dbTx kv.RwTx) (*state.Batch, error) {
	panic("GetBatchByNumber: implement me")
}

func (m *StateInterfaceAdapter) ResetTrustedState(ctx context.Context, batchNumber uint64, dbTx kv.RwTx) error {
	panic("ResetTrustedState: implement me")
}

func (m *StateInterfaceAdapter) GetNextForcedBatches(ctx context.Context, nextForcedBatches int, dbTx kv.RwTx) ([]state.ForcedBatch, error) {
	panic("GetNextForcedBatches: implement me")
}

func (m *StateInterfaceAdapter) AddVerifiedBatch(ctx context.Context, verifiedBatch *state.VerifiedBatch, dbTx kv.RwTx) error {
	panic("AddVerifiedBatch: implement me")
}

func (m *StateInterfaceAdapter) ProcessAndStoreClosedBatch(ctx context.Context, processingCtx state.ProcessingContext, encodedTxs []byte, dbTx kv.RwTx, caller metrics.CallerLabel) (common.Hash, error) {
	panic("ProcessAndStoreClosedBatch: implement me")
}

func (m *StateInterfaceAdapter) OpenBatch(ctx context.Context, processingContext state.ProcessingContext, dbTx kv.RwTx) error {
	panic("OpenBatch: implement me")
}

func (m *StateInterfaceAdapter) CloseBatch(ctx context.Context, receipt state.ProcessingReceipt, dbTx kv.RwTx) error {
	panic("CloseBatch: implement me")
}

func (m *StateInterfaceAdapter) ProcessSequencerBatch(ctx context.Context, batchNumber uint64, batchL2Data []byte, caller metrics.CallerLabel, dbTx kv.RwTx) (*state.ProcessBatchResponse, error) {
	panic("ProcessSequencerBatch: implement me")
}

func (m *StateInterfaceAdapter) StoreTransactions(ctx context.Context, batchNum uint64, processedTxs []*state.ProcessTransactionResponse, dbTx kv.RwTx) error {
	panic("StoreTransactions: implement me")
}

func (m *StateInterfaceAdapter) GetStateRootByBatchNumber(ctx context.Context, batchNum uint64, dbTx kv.RwTx) (common.Hash, error) {
	panic("GetStateRootByBatchNumber: implement me")
}

func (m *StateInterfaceAdapter) ExecuteBatch(ctx context.Context, batch state.Batch, updateMerkleTree bool, dbTx kv.RwTx) (*pb.ProcessBatchResponse, error) {
	panic("ExecuteBatch: implement me")
}

func (m *StateInterfaceAdapter) GetLastVerifiedBatch(ctx context.Context, dbTx kv.RwTx) (*state.VerifiedBatch, error) {
	panic("GetLastVerifiedBatch: implement me")
}

func (m *StateInterfaceAdapter) GetLastVirtualBatchNum(ctx context.Context, dbTx kv.RwTx) (uint64, error) {
	panic("GetLastVirtualBatchNum: implement me")
}

func (m *StateInterfaceAdapter) AddSequence(ctx context.Context, sequence state.Sequence, dbTx kv.RwTx) error {
	panic("AddSequence: implement me")
}

func (m *StateInterfaceAdapter) AddAccumulatedInputHash(ctx context.Context, batchNum uint64, accInputHash common.Hash, dbTx kv.RwTx) error {
	panic("AddAccumulatedInputHash: implement me")
}

func (m *StateInterfaceAdapter) AddTrustedReorg(ctx context.Context, trustedReorg *state.TrustedReorg, dbTx kv.RwTx) error {
	panic("AddTrustedReorg: implement me")
}

func (m *StateInterfaceAdapter) GetReorgedTransactions(ctx context.Context, batchNumber uint64, dbTx kv.RwTx) ([]ethTypes.Transaction, error) {
	panic("GetReorgedTransactions: implement me")
}

func (m *StateInterfaceAdapter) ResetForkID(ctx context.Context, batchNumber, forkID uint64, version string, dbTx kv.RwTx) error {
	panic("ResetForkID: implement me")
}

func (m *StateInterfaceAdapter) GetForkIDTrustedReorgCount(ctx context.Context, forkID uint64, version string, dbTx kv.RwTx) (uint64, error) {
	panic("GetForkIDTrustedReorgCount: implement me")
}

func (m *StateInterfaceAdapter) UpdateForkIDIntervals(intervals []state.ForkIDInterval) {
	panic("UpdateForkIDIntervals: implement me")
}

func (m *StateInterfaceAdapter) BeginStateTransaction(ctx context.Context) (kv.RwTx, error) {
	panic("BeginStateTransaction: implement me")
}
