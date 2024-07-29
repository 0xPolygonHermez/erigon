package stages

import (
	"fmt"
	"math"

	"github.com/gateway-fm/cdk-erigon-lib/common"
	"github.com/ledgerwatch/erigon/core"
	"github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/zk/l1_data"
	zktx "github.com/ledgerwatch/erigon/zk/tx"
)

// TYPE BLOCK STATE
type BlockState struct {
	blockTransactions []types.Transaction
	l1RecoveryData    *L1RecoveryData
	usedBlockElements UsedBlockElements
}

func newBlockState(l1Recovery bool) *BlockState {
	blockState := &BlockState{}

	if l1Recovery {
		blockState.l1RecoveryData = newL1RecoveryData()
	}

	return blockState
}

func (bs *BlockState) isL1Recovery() bool {
	return bs.l1RecoveryData != nil
}

func (bs *BlockState) isThereAnyTransactionsToRecover() bool {
	if !bs.isL1Recovery() {
		return false
	}

	return len(bs.blockTransactions) != 0 || bs.l1RecoveryData.nextBatchData.IsWorkRemaining
}

func (bs *BlockState) loadDataByDecodedBlockIndex(decodedBlocksIndex uint64) bool {
	if !bs.l1RecoveryData.loadDataByDecodedBlockIndex(decodedBlocksIndex) {
		return false
	}

	bs.blockTransactions = bs.l1RecoveryData.decodedBlock.Transactions
	return true
}

func (bs *BlockState) getDeltaTimestamp() uint64 {
	if bs.l1RecoveryData != nil {
		return uint64(bs.l1RecoveryData.decodedBlock.DeltaTimestamp)
	}

	return math.MaxUint64
}

func (bs *BlockState) getCoinbase(cfg *SequenceBlockCfg) common.Address {
	if bs.l1RecoveryData != nil {
		return bs.l1RecoveryData.nextBatchData.Coinbase
	}

	return cfg.zk.AddressSequencer
}

func (bs *BlockState) getL1EffectiveGases(cfg SequenceBlockCfg, i int) uint8 {
	if bs.isL1Recovery() {
		return bs.l1RecoveryData.decodedBlock.EffectiveGasPricePercentages[i]
	}

	return DeriveEffectiveGasPrice(cfg, bs.blockTransactions[i])
}

// TYPE BLOCK ELEMENTS
type UsedBlockElements struct {
	transactions     []types.Transaction
	receipts         types.Receipts
	effectiveGases   []uint8
	executionResults []*core.ExecutionResult
}

func (ube *UsedBlockElements) resetBlockBuildingArrays() {
	ube.transactions = []types.Transaction{}
	ube.receipts = types.Receipts{}
	ube.effectiveGases = []uint8{}
	ube.executionResults = []*core.ExecutionResult{}
}

func (ube *UsedBlockElements) onFinishAddingTransaction(transaction types.Transaction, receipt *types.Receipt, execResult *core.ExecutionResult, effectiveGas uint8) {
	ube.transactions = append(ube.transactions, transaction)
	ube.receipts = append(ube.receipts, receipt)
	ube.executionResults = append(ube.executionResults, execResult)
	ube.effectiveGases = append(ube.effectiveGases, effectiveGas)
}

// TYPE L1 RECOVERY DATA
type L1RecoveryData struct {
	decodedBlocksSize uint64
	decodedBlock      *zktx.DecodedBatchL2Data
	nextBatchData     *l1_data.DecodedL1Data
}

func newL1RecoveryData() *L1RecoveryData {
	return &L1RecoveryData{}
}

func (l1rd *L1RecoveryData) loadNextBatchData(sdb *stageDb, thisBatch, forkId uint64) (err error) {
	l1rd.nextBatchData, err = l1_data.BreakDownL1DataByBatch(thisBatch, forkId, sdb.hermezDb.HermezDbReader)
	if err != nil {
		return err
	}

	l1rd.decodedBlocksSize = uint64(len(l1rd.nextBatchData.DecodedData))
	return nil
}

func (l1rd *L1RecoveryData) hasAnyDecodedBlocks() bool {
	return l1rd.decodedBlocksSize == 0
}

func (l1rd *L1RecoveryData) getInfoTreeIndex(sdb *stageDb) (uint64, error) {
	var infoTreeIndex uint64

	if l1rd.nextBatchData.L1InfoRoot == SpecialZeroIndexHash {
		return uint64(0), nil
	}

	infoTreeIndex, found, err := sdb.hermezDb.GetL1InfoTreeIndexByRoot(l1rd.nextBatchData.L1InfoRoot)
	if err != nil {
		return uint64(0), err
	}
	if !found {
		return uint64(0), fmt.Errorf("could not find L1 info tree index for root %s", l1rd.nextBatchData.L1InfoRoot.String())
	}

	return infoTreeIndex, nil
}

func (l1rd *L1RecoveryData) loadDataByDecodedBlockIndex(decodedBlocksIndex uint64) bool {
	if decodedBlocksIndex == l1rd.decodedBlocksSize {
		return false
	}

	l1rd.decodedBlock = &l1rd.nextBatchData.DecodedData[decodedBlocksIndex]
	return true
}
