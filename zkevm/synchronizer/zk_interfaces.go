package synchronizer

import (
	"context"
	"math/big"

	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/kv"

	ethTypes "github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/zkevm/etherman"
	"github.com/ledgerwatch/erigon/zkevm/jsonrpc/types"
	"github.com/ledgerwatch/erigon/zkevm/state"
	"github.com/ledgerwatch/erigon/zkevm/state/metrics"
	"github.com/ledgerwatch/erigon/zkevm/state/runtime/executor/pb"
)

// ethermanInterface contains the methods required to interact with ethereum.
type ethermanInterface interface {
	HeaderByNumber(ctx context.Context, number *big.Int) (*ethTypes.Header, error)
	GetRollupInfoByBlockRange(ctx context.Context, fromBlock uint64, toBlock *uint64) ([]etherman.Block, map[common.Hash][]etherman.Order, error)
	EthBlockByNumber(ctx context.Context, blockNumber uint64) (*ethTypes.Block, error)
	GetLatestBatchNumber() (uint64, error)
	GetTrustedSequencerURL() (string, error)
	VerifyGenBlockNumber(ctx context.Context, genBlockNumber uint64) (bool, error)
	GetForks(ctx context.Context) ([]state.ForkIDInterval, error)
}

// stateInterface gathers the methods required to interact with the state.
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

type ethTxManager interface {
	Reorg(ctx context.Context, fromBlockNumber uint64, dbTx kv.RwTx) error
}

type poolInterface interface {
	DeleteReorgedTransactions(ctx context.Context, txs []ethTypes.Transaction) error
	StoreTx(ctx context.Context, tx ethTypes.Transaction, ip string, isWIP bool) error
}

type zkEVMClientInterface interface {
	BatchNumber(ctx context.Context) (uint64, error)
	BatchByNumber(ctx context.Context, number *big.Int) (*types.Batch, error)
}
