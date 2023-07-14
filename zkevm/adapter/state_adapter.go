package adapter

import (
	"context"
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/iden3/go-iden3-crypto/keccak256"
	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon/core/rawdb"
	"github.com/ledgerwatch/erigon/eth/stagedsync/stages"
	"github.com/ledgerwatch/erigon/zkevm/state"
	"github.com/ledgerwatch/erigon/zkevm/state/metrics"
	"github.com/ledgerwatch/erigon/zkevm/state/runtime/executor/pb"

	"encoding/json"
	ethTypes "github.com/ledgerwatch/erigon/core/types"
	"time"
)

const HermezBatch = "HermezBatch"
const HermezVerifiedBatch = "HermezVerifiedBatch"

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

// gdb is a database of global exit roots
var gdb = map[common.Hash]common.Hash{}

type StateInterfaceAdapter struct {
	currentBatchNumber int64
}

var _ stateInterface = (*StateInterfaceAdapter)(nil)

func NewStateAdapter() stateInterface {
	return &StateInterfaceAdapter{currentBatchNumber: 1}
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

	fmt.Printf("AddGlobalExitRoot: ger: %x\n", exitRoot.GlobalExitRoot[:])

	// convert GLOBAL_EXIT_ROOT_STORAGE_POS to 32 bytes
	gerb := make([]byte, 32)
	binary.BigEndian.PutUint64(gerb, GLOBAL_EXIT_ROOT_STORAGE_POS)

	// concat global exit root and global_exit_root_storage_pos
	rootPlusStorage := append(exitRoot.GlobalExitRoot[:], gerb...)

	globalExitRootPos := keccak256.Hash(rootPlusStorage)

	gdb[exitRoot.GlobalExitRoot] = common.BytesToHash(globalExitRootPos)

	return nil
}

func (m *StateInterfaceAdapter) writeGlobalExitRootToDb(dbTx kv.RwTx, blockNo uint64, gers state.GlobalExitRootDb) error {
	j, err := json.Marshal(gers)
	if err != nil {
		return err
	}
	return dbTx.Put("HermezGlobalExitRoot", UintBytes(blockNo), j)
}

func (m *StateInterfaceAdapter) AddForcedBatch(ctx context.Context, forcedBatch *state.ForcedBatch, dbTx kv.RwTx) error {
	panic("AddForcedBatch: implement me")
}

func (m *StateInterfaceAdapter) AddBlock(ctx context.Context, block *state.Block, dbTx kv.RwTx) error {
	fmt.Printf("AddBlock, saving ETH progress block: %d\n", block.BlockNumber)
	// [zkevm] - max note: we shouldn't save this here - it isn't the true sign of actual progress
	//return stages.SaveStageProgress(dbTx, stages.L1Blocks, block.BlockNumber)
	return nil
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

	batch := &state.Batch{}
	b, err := dbTx.GetOne(HermezBatch, UintBytes(batchNumber))
	if err != nil {
		return nil, err
	}

	if len(b) == 0 {
		return nil, state.ErrNotFound
	}

	err = batch.UnmarshalJSON(b)
	if err != nil {
		return nil, err
	}

	return batch, err
}

func (m *StateInterfaceAdapter) ResetTrustedState(ctx context.Context, batchNumber uint64, dbTx kv.RwTx) error {
	panic("ResetTrustedState: implement me")
}

func (m *StateInterfaceAdapter) GetNextForcedBatches(ctx context.Context, nextForcedBatches int, dbTx kv.RwTx) ([]state.ForcedBatch, error) {
	panic("GetNextForcedBatches: implement me")
}

func (m *StateInterfaceAdapter) AddVerifiedBatch(ctx context.Context, verifiedBatch *state.VerifiedBatch, dbTx kv.RwTx) error {
	fmt.Printf("AddVerifiedBatch, saving L2 progress batch: %d blockNum: %d\n", verifiedBatch.BatchNumber, verifiedBatch.BlockNumber)

	// [zkevm] - restrict progress
	if verifiedBatch.BatchNumber > 500 {
		return nil
	}

	// Get the matching batch (body) for the verified batch (header)
	batch, err := m.GetBatchByNumber(ctx, verifiedBatch.BatchNumber, dbTx)
	if err != nil {
		return err
	}

	header, err := WriteHeaderToDb(dbTx, verifiedBatch, batch.Timestamp)
	if err != nil {
		return err
	}

	vbJson, err := verifiedBatch.ToJSON()
	if err != nil {
		return err
	}

	err = dbTx.Put(HermezVerifiedBatch, UintBytes(verifiedBatch.BatchNumber), []byte(vbJson))
	if err != nil {
		return err
	}

	seqval, err := dbTx.IncrementSequence(HermezVerifiedBatch, 1)
	if err != nil {
		return err
	}

	fmt.Println("AddVerifiedBatch: seqval", seqval)
	fmt.Println("AddVerifiedBatch: batch number", verifiedBatch.BatchNumber)

	err = WriteBodyToDb(dbTx, batch, header.Hash())
	if err != nil {
		return err
	}

	// write the global exit root to state
	gerp, ok := gdb[batch.GlobalExitRoot]
	ts := int64(0)
	if ok {
		ts = batch.Timestamp.Unix()
		delete(gdb, batch.GlobalExitRoot)
	}
	gerdb := state.GlobalExitRootDb{
		GlobalExitRoot:         batch.GlobalExitRoot,
		GlobalExitRootPosition: gerp,
		Timestamp:              ts,
	}
	err = m.writeGlobalExitRootToDb(dbTx, verifiedBatch.BatchNumber, gerdb) // batch no is block no in stage_execute
	if err != nil {
		return err
	}

	err = stages.SaveStageProgress(dbTx, stages.L1Blocks, verifiedBatch.BlockNumber)
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

	batch := &state.Batch{
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

	// serialize and store batch
	batchJson, err := batch.ToJSON()
	if err != nil {
		return common.Hash{}, err
	}
	err = dbTx.Put(HermezBatch, UintBytes(processingCtx.BatchNumber), []byte(batchJson))
	if err != nil {
		return common.Hash{}, err
	}

	// increment sequence so we can easily get 'highest batch'
	seqval, err := dbTx.IncrementSequence(HermezBatch, 1)
	if err != nil {
		return common.Hash{}, err
	}

	fmt.Println("ProcessAndStoreClosedBatch: seqval", seqval)
	fmt.Println("ProcessAndStoreClosedBatch: batch number", processingCtx.BatchNumber)

	return common.Hash{}, nil
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

	maxKey, err := dbTx.ReadSequence(HermezVerifiedBatch)
	if err != nil {
		return nil, err
	}

	vb, err := dbTx.GetOne(HermezVerifiedBatch, UintBytes(maxKey))
	if err != nil || len(vb) == 0 {
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

	verifiedBatch := &state.VerifiedBatch{}
	err = verifiedBatch.FromJSON(vb)
	if err != nil {
		return nil, err
	}

	return verifiedBatch, nil
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

func WriteHeaderToDb(dbTx kv.RwTx, vb *state.VerifiedBatch, ts time.Time) (*ethTypes.Header, error) {
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
		Coinbase:   common.HexToAddress("0x148Ee7dAF16574cD020aFa34CC658f8F3fbd2800"), // the sequencer gets the txfee
		Time:       uint64(ts.Unix()),
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

func UintBytes(no uint64) []byte {
	noBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(noBytes, no)
	return noBytes
}
