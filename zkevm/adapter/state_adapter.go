package adapter

import (
	"context"

	"github.com/jackc/pgx/v4"
	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon/zkevm/state"
	"github.com/ledgerwatch/erigon/zkevm/state/metrics"
	"github.com/ledgerwatch/erigon/zkevm/state/runtime/executor/pb"

	ethTypes "github.com/ledgerwatch/erigon/core/types"
)

// write me an adapter mock for stateInterface from zk_synchronizer.go
// Interface
type stateInterface interface {
	GetLastBlock(ctx context.Context, dbTx pgx.Tx) (*state.Block, error)
	AddGlobalExitRoot(ctx context.Context, exitRoot *state.GlobalExitRoot, dbTx pgx.Tx) error
	AddForcedBatch(ctx context.Context, forcedBatch *state.ForcedBatch, dbTx pgx.Tx) error
	AddBlock(ctx context.Context, block *state.Block, dbTx pgx.Tx) error
	AddVirtualBatch(ctx context.Context, virtualBatch *state.VirtualBatch, dbTx pgx.Tx) error
	GetPreviousBlock(ctx context.Context, offset uint64, dbTx pgx.Tx) (*state.Block, error)
	GetLastBatchNumber(ctx context.Context, dbTx pgx.Tx) (uint64, error)
	GetBatchByNumber(ctx context.Context, batchNumber uint64, dbTx pgx.Tx) (*state.Batch, error)
	ResetTrustedState(ctx context.Context, batchNumber uint64, dbTx pgx.Tx) error
	GetNextForcedBatches(ctx context.Context, nextForcedBatches int, dbTx pgx.Tx) ([]state.ForcedBatch, error)
	AddVerifiedBatch(ctx context.Context, verifiedBatch *state.VerifiedBatch, dbTx pgx.Tx) error
	ProcessAndStoreClosedBatch(ctx context.Context, processingCtx state.ProcessingContext, encodedTxs []byte, dbTx pgx.Tx, caller metrics.CallerLabel) (common.Hash, error)
	OpenBatch(ctx context.Context, processingContext state.ProcessingContext, dbTx pgx.Tx) error
	CloseBatch(ctx context.Context, receipt state.ProcessingReceipt, dbTx pgx.Tx) error
	ProcessSequencerBatch(ctx context.Context, batchNumber uint64, batchL2Data []byte, caller metrics.CallerLabel, dbTx pgx.Tx) (*state.ProcessBatchResponse, error)
	StoreTransactions(ctx context.Context, batchNum uint64, processedTxs []*state.ProcessTransactionResponse, dbTx pgx.Tx) error
	GetStateRootByBatchNumber(ctx context.Context, batchNum uint64, dbTx pgx.Tx) (common.Hash, error)
	ExecuteBatch(ctx context.Context, batch state.Batch, updateMerkleTree bool, dbTx pgx.Tx) (*pb.ProcessBatchResponse, error)
	GetLastVerifiedBatch(ctx context.Context, dbTx pgx.Tx) (*state.VerifiedBatch, error)
	GetLastVirtualBatchNum(ctx context.Context, dbTx pgx.Tx) (uint64, error)
	AddSequence(ctx context.Context, sequence state.Sequence, dbTx pgx.Tx) error
	AddAccumulatedInputHash(ctx context.Context, batchNum uint64, accInputHash common.Hash, dbTx pgx.Tx) error
	AddTrustedReorg(ctx context.Context, trustedReorg *state.TrustedReorg, dbTx pgx.Tx) error
	GetReorgedTransactions(ctx context.Context, batchNumber uint64, dbTx pgx.Tx) ([]ethTypes.Transaction, error)
	ResetForkID(ctx context.Context, batchNumber, forkID uint64, version string, dbTx pgx.Tx) error
	GetForkIDTrustedReorgCount(ctx context.Context, forkID uint64, version string, dbTx pgx.Tx) (uint64, error)
	UpdateForkIDIntervals(intervals []state.ForkIDInterval)

	BeginStateTransaction(ctx context.Context) (pgx.Tx, error)
}

type StateInterfaceAdapter struct{}

var _ stateInterface = (*StateInterfaceAdapter)(nil)

func NewStateAdapter() stateInterface {
	return &StateInterfaceAdapter{}
}

func (m *StateInterfaceAdapter) GetLastBlock(ctx context.Context, dbTx pgx.Tx) (*state.Block, error) {
	panic("implement me")
}

func (m *StateInterfaceAdapter) AddGlobalExitRoot(ctx context.Context, exitRoot *state.GlobalExitRoot, dbTx pgx.Tx) error {
	panic("implement me")
}

func (m *StateInterfaceAdapter) AddForcedBatch(ctx context.Context, forcedBatch *state.ForcedBatch, dbTx pgx.Tx) error {
	panic("implement me")
}

func (m *StateInterfaceAdapter) AddBlock(ctx context.Context, block *state.Block, dbTx pgx.Tx) error {
	panic("implement me")
}

func (m *StateInterfaceAdapter) AddVirtualBatch(ctx context.Context, virtualBatch *state.VirtualBatch, dbTx pgx.Tx) error {
	panic("implement me")
}

func (m *StateInterfaceAdapter) GetPreviousBlock(ctx context.Context, offset uint64, dbTx pgx.Tx) (*state.Block, error) {
	panic("implement me")
}

func (m *StateInterfaceAdapter) GetLastBatchNumber(ctx context.Context, dbTx pgx.Tx) (uint64, error) {
	panic("implement me")
}

func (m *StateInterfaceAdapter) GetBatchByNumber(ctx context.Context, batchNumber uint64, dbTx pgx.Tx) (*state.Batch, error) {
	panic("implement me")
}

func (m *StateInterfaceAdapter) ResetTrustedState(ctx context.Context, batchNumber uint64, dbTx pgx.Tx) error {
	panic("implement me")
}

func (m *StateInterfaceAdapter) GetNextForcedBatches(ctx context.Context, nextForcedBatches int, dbTx pgx.Tx) ([]state.ForcedBatch, error) {
	panic("implement me")
}

func (m *StateInterfaceAdapter) AddVerifiedBatch(ctx context.Context, verifiedBatch *state.VerifiedBatch, dbTx pgx.Tx) error {
	panic("implement me")
}

func (m *StateInterfaceAdapter) ProcessAndStoreClosedBatch(ctx context.Context, processingCtx state.ProcessingContext, encodedTxs []byte, dbTx pgx.Tx, caller metrics.CallerLabel) (common.Hash, error) {
	panic("implement me")
}

func (m *StateInterfaceAdapter) OpenBatch(ctx context.Context, processingContext state.ProcessingContext, dbTx pgx.Tx) error {
	panic("implement me")
}

func (m *StateInterfaceAdapter) CloseBatch(ctx context.Context, receipt state.ProcessingReceipt, dbTx pgx.Tx) error {
	panic("implement me")
}

func (m *StateInterfaceAdapter) ProcessSequencerBatch(ctx context.Context, batchNumber uint64, batchL2Data []byte, caller metrics.CallerLabel, dbTx pgx.Tx) (*state.ProcessBatchResponse, error) {
	panic("implement me")
}

func (m *StateInterfaceAdapter) StoreTransactions(ctx context.Context, batchNum uint64, processedTxs []*state.ProcessTransactionResponse, dbTx pgx.Tx) error {
	panic("implement me")
}

func (m *StateInterfaceAdapter) GetStateRootByBatchNumber(ctx context.Context, batchNum uint64, dbTx pgx.Tx) (common.Hash, error) {
	panic("implement me")
}

func (m *StateInterfaceAdapter) ExecuteBatch(ctx context.Context, batch state.Batch, updateMerkleTree bool, dbTx pgx.Tx) (*pb.ProcessBatchResponse, error) {
	panic("implement me")
}

func (m *StateInterfaceAdapter) GetLastVerifiedBatch(ctx context.Context, dbTx pgx.Tx) (*state.VerifiedBatch, error) {
	panic("implement me")
}

func (m *StateInterfaceAdapter) GetLastVirtualBatchNum(ctx context.Context, dbTx pgx.Tx) (uint64, error) {
	panic("implement me")
}

func (m *StateInterfaceAdapter) AddSequence(ctx context.Context, sequence state.Sequence, dbTx pgx.Tx) error {
	panic("implement me")
}

func (m *StateInterfaceAdapter) AddAccumulatedInputHash(ctx context.Context, batchNum uint64, accInputHash common.Hash, dbTx pgx.Tx) error {
	panic("implement me")
}

func (m *StateInterfaceAdapter) AddTrustedReorg(ctx context.Context, trustedReorg *state.TrustedReorg, dbTx pgx.Tx) error {
	panic("implement me")
}

func (m *StateInterfaceAdapter) GetReorgedTransactions(ctx context.Context, batchNumber uint64, dbTx pgx.Tx) ([]ethTypes.Transaction, error) {
	panic("implement me")
}

func (m *StateInterfaceAdapter) ResetForkID(ctx context.Context, batchNumber, forkID uint64, version string, dbTx pgx.Tx) error {
	panic("implement me")
}

func (m *StateInterfaceAdapter) GetForkIDTrustedReorgCount(ctx context.Context, forkID uint64, version string, dbTx pgx.Tx) (uint64, error) {
	panic("implement me")
}

func (m *StateInterfaceAdapter) UpdateForkIDIntervals(intervals []state.ForkIDInterval) {
	panic("implement me")
}

func (m *StateInterfaceAdapter) BeginStateTransaction(ctx context.Context) (pgx.Tx, error) {
	panic("implement me")
}
