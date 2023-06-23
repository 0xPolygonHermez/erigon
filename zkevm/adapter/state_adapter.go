package adapter

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"

	"github.com/holiman/uint256"
	"github.com/iden3/go-iden3-crypto/keccak256"
	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/kv"

	"github.com/ledgerwatch/erigon/core/rawdb"
	state2 "github.com/ledgerwatch/erigon/core/state"
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

var headerHashes = map[uint64]common.Hash{}

type batchdb struct {
	verifiedBatches map[uint64]*state.VerifiedBatch
	batches         map[uint64]*state.Batch
}

var bdb = &batchdb{
	verifiedBatches: map[uint64]*state.VerifiedBatch{},
	batches:         map[uint64]*state.Batch{},
}

type StateInterfaceAdapter struct {
}

var _ stateInterface = (*StateInterfaceAdapter)(nil)

func NewStateAdapter() stateInterface {
	return &StateInterfaceAdapter{}
}

const GLOBAL_EXIT_ROOT_STORAGE_POS = 0
const ADDRESS_GLOBAL_EXIT_ROOT_MANAGER_L2 = "0xa40D5f56745a118D0906a34E69aeC8C0Db1cB8fA"

func (m *StateInterfaceAdapter) GetLastBlock(ctx context.Context, dbTx kv.RwTx) (*state.Block, error) {
	blockHeight, err := stages.GetStageProgress(dbTx, stages.L1Blocks)
	if err != nil {
		return nil, err
	}

	// makes no sense to process blocks before the deployment of the smart contract
	if blockHeight < 16896700 {
		blockHeight = 16896700
	}

	// we just need this to make sure from which block to begin parsing in case of restart
	return &state.Block{BlockNumber: blockHeight}, nil
}

func (m *StateInterfaceAdapter) AddGlobalExitRoot(ctx context.Context, exitRoot *state.GlobalExitRoot, dbTx kv.RwTx) error {
	// we should store these, so we can process exits in the bridge contract - this is a rough translation of the JS implementation

	if exitRoot == nil {
		fmt.Println("AddGlobalExitRoot: nil exit root")
		return nil
	}

	// convert GLOBAL_EXIT_ROOT_STORAGE_POS to 32 bytes
	gerb := make([]byte, 32)
	binary.BigEndian.PutUint64(gerb, GLOBAL_EXIT_ROOT_STORAGE_POS)

	// concat global exit root and global_exit_root_storage_pos
	rootPlusStorage := append(exitRoot.GlobalExitRoot[:], gerb...)

	globalExitRootPos := keccak256.Hash(rootPlusStorage)
	addr := common.HexToAddress(ADDRESS_GLOBAL_EXIT_ROOT_MANAGER_L2)

	exitBig := exitRoot.GlobalExitRoot.Big()
	exitUint256, overflow := uint256.FromBig(exitBig)
	if overflow {
		return errors.New("AddGlobalExitRoot: overflow")
	}

	old := common.Hash{}.Big()
	oldUint256, overflow := uint256.FromBig(old)
	if overflow {
		return errors.New("AddGlobalExitRoot: overflow")
	}

	gerp := common.BytesToHash(globalExitRootPos[:])

	// get a db state writer
	psw := state2.NewPlainStateWriter(dbTx, dbTx, exitRoot.BlockNumber)
	// I don't know what 'original' is just yet
	fmt.Printf("addr %x key: %x newVal: %v\n", addr, gerp, exitUint256)
	err := psw.WriteAccountStorage(addr, uint64(1), &gerp, oldUint256, exitUint256)
	if err != nil {
		return err
	}

	return nil
}

func (m *StateInterfaceAdapter) AddForcedBatch(ctx context.Context, forcedBatch *state.ForcedBatch, dbTx kv.RwTx) error {
	panic("AddForcedBatch: implement me")
}

func (m *StateInterfaceAdapter) AddBlock(ctx context.Context, block *state.Block, dbTx kv.RwTx) error {
	fmt.Printf("AddBlock, saving ETH progress block: %d\n", block.BlockNumber)

	return stages.SaveStageProgress(dbTx, stages.L1Blocks, block.BlockNumber)
}

func (m *StateInterfaceAdapter) AddVirtualBatch(ctx context.Context, virtualBatch *state.VirtualBatch, dbTx kv.RwTx) error {
	// [zkevm] - store in temp in mem db for debugging
	return nil
}

func (m *StateInterfaceAdapter) GetPreviousBlock(ctx context.Context, offset uint64, dbTx kv.RwTx) (*state.Block, error) {
	panic("GetPreviousBlock: implement me")
}

func (m *StateInterfaceAdapter) GetLastBatchNumber(ctx context.Context, dbTx kv.RwTx) (uint64, error) {
	return stages.GetStageProgress(dbTx, stages.Bodies)
}

func (m *StateInterfaceAdapter) GetBatchByNumber(ctx context.Context, batchNumber uint64, dbTx kv.RwTx) (*state.Batch, error) {
	b, ok := bdb.batches[batchNumber]
	if !ok {
		return nil, state.ErrNotFound
	}
	return b, nil
}

func (m *StateInterfaceAdapter) ResetTrustedState(ctx context.Context, batchNumber uint64, dbTx kv.RwTx) error {
	panic("ResetTrustedState: implement me")
}

func (m *StateInterfaceAdapter) GetNextForcedBatches(ctx context.Context, nextForcedBatches int, dbTx kv.RwTx) ([]state.ForcedBatch, error) {
	panic("GetNextForcedBatches: implement me")
}

func (m *StateInterfaceAdapter) AddVerifiedBatch(ctx context.Context, verifiedBatch *state.VerifiedBatch, dbTx kv.RwTx) error {
	fmt.Printf("AddVerifiedBatch, saving L2 progress batch: %d\n", verifiedBatch.BatchNumber)

	header, err := WriteHeaderToDb(dbTx, verifiedBatch)
	if err != nil {
		return err
	}
	headerHashes[verifiedBatch.BatchNumber] = header.Hash()

	bdb.verifiedBatches[verifiedBatch.BatchNumber] = verifiedBatch

	vb := bdb.batches[verifiedBatch.BatchNumber]
	if vb == nil {
		fmt.Println("SAVING WITHOUT HEADER!!!!")
	}
	err = WriteBodyToDb(dbTx, vb, header.Hash())
	if err != nil {
		return err
	}

	err = stages.SaveStageProgress(dbTx, stages.Headers, verifiedBatch.BatchNumber)
	if err != nil {
		return err
	}

	err = stages.SaveStageProgress(dbTx, stages.Bodies, verifiedBatch.BatchNumber)
	if err != nil {
		return err
	}

	return nil
}

func (m *StateInterfaceAdapter) ProcessAndStoreClosedBatch(ctx context.Context, processingCtx state.ProcessingContext, encodedTxs []byte, dbTx kv.RwTx, caller metrics.CallerLabel) (common.Hash, error) {
	txs, _, err := state.DecodeTxs(encodedTxs)
	if err != nil {
		return common.Hash{}, err
	}

	bdb.batches[processingCtx.BatchNumber] = &state.Batch{
		BatchNumber:    processingCtx.BatchNumber,
		Coinbase:       processingCtx.Coinbase,
		BatchL2Data:    encodedTxs,
		StateRoot:      common.Hash{},
		LocalExitRoot:  common.Hash{},
		AccInputHash:   common.Hash{},
		Timestamp:      processingCtx.Timestamp,
		Transactions:   txs,
		GlobalExitRoot: processingCtx.GlobalExitRoot,
		ForcedBatchNum: processingCtx.ForcedBatchNum,
	}

	return headerHashes[processingCtx.BatchNumber], nil
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
	// TODO [zkevm] max - make a noop for now

	return nil
}

func (m *StateInterfaceAdapter) GetStateRootByBatchNumber(ctx context.Context, batchNum uint64, dbTx kv.RwTx) (common.Hash, error) {
	panic("GetStateRootByBatchNumber: implement me")
}

func (m *StateInterfaceAdapter) ExecuteBatch(ctx context.Context, batch state.Batch, updateMerkleTree bool, dbTx kv.RwTx) (*pb.ProcessBatchResponse, error) {
	// TODO [zkevm] this isn't implemented for PoC

	pbr := &pb.ProcessBatchResponse{
		NewStateRoot:        batch.StateRoot.Bytes(),
		NewAccInputHash:     batch.AccInputHash.Bytes(),
		NewLocalExitRoot:    batch.LocalExitRoot.Bytes(),
		NewBatchNum:         batch.BatchNumber,
		CntKeccakHashes:     0,
		CntPoseidonHashes:   0,
		CntPoseidonPaddings: 0,
		CntMemAligns:        0,
		CntArithmetics:      0,
		CntBinaries:         0,
		CntSteps:            0,
		CumulativeGasUsed:   0,
		Responses:           nil,
		Error:               0,
		ReadWriteAddresses:  nil,
	}

	return pbr, nil
}

func (m *StateInterfaceAdapter) GetLastVerifiedBatch(ctx context.Context, dbTx kv.RwTx) (*state.VerifiedBatch, error) {
	var maxKey uint64
	for k := range bdb.verifiedBatches {
		if k > maxKey {
			maxKey = k
		}
	}

	vb, ok := bdb.verifiedBatches[maxKey]
	if !ok {
		// return an empty zero batch
		return &state.VerifiedBatch{
			BlockNumber: 0,
			BatchNumber: 0,
			Aggregator:  common.Address{},
			TxHash:      common.Hash{},
			StateRoot:   common.Hash{},
			IsTrusted:   false,
		}, nil
	}

	return vb, nil
}

func (m *StateInterfaceAdapter) GetLastVirtualBatchNum(ctx context.Context, dbTx kv.RwTx) (uint64, error) {
	panic("GetLastVirtualBatchNum: implement me")
}

func (m *StateInterfaceAdapter) AddSequence(ctx context.Context, sequence state.Sequence, dbTx kv.RwTx) error {
	// TODO [max]: maybe we should do something here
	return nil
}

func (m *StateInterfaceAdapter) AddAccumulatedInputHash(ctx context.Context, batchNum uint64, accInputHash common.Hash, dbTx kv.RwTx) error {
	//panic("AddAccumulatedInputHash: implement me")
	return nil
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
	// just pretendint its okay
	return 1, nil
}

func (m *StateInterfaceAdapter) UpdateForkIDIntervals(intervals []state.ForkIDInterval) {
	panic("UpdateForkIDIntervals: implement me")
}

func (m *StateInterfaceAdapter) BeginStateTransaction(ctx context.Context) (kv.RwTx, error) {
	panic("BeginStateTransaction: implement me")
}

func WriteHeaderToDb(dbTx kv.RwTx, vb *state.VerifiedBatch) (*ethTypes.Header, error) {
	if dbTx == nil {
		return nil, fmt.Errorf("dbTx is nil")
	}

	// erigon block number is l2 batch number
	blockNo := new(big.Int).SetUint64(vb.BatchNumber)

	fmt.Println(vb.StateRoot)

	h := &ethTypes.Header{
		Root:       vb.StateRoot,
		TxHash:     vb.TxHash,
		Difficulty: big.NewInt(0),
		Number:     blockNo,
		GasLimit:   30_000_000,
	}
	rawdb.WriteHeader(dbTx, h)
	rawdb.WriteCanonicalHash(dbTx, h.Hash(), blockNo.Uint64())
	return h, nil
}

func WriteBodyToDb(dbTx kv.RwTx, batch *state.Batch, hh common.Hash) error {
	if dbTx == nil {
		return fmt.Errorf("dbTx is nil")
	}

	b := &ethTypes.Body{
		Transactions: batch.Transactions,
	}

	// writes txs to EthTx (canonical table)
	return rawdb.WriteBody(dbTx, hh, batch.BatchNumber, b)
}
