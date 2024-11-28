package acc_input_hash

import (
	"fmt"
	"github.com/ledgerwatch/erigon-lib/common"
	"context"
	eritypes "github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/zk/utils"
	"github.com/ledgerwatch/erigon/zk/types"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/chain"
)

// BlockReader is a rawdb block reader abstraction for easier testing
type BlockReader interface {
	ReadBlockByNumber(blockNo uint64) (*eritypes.Block, error)
}

// BatchDataReader is an abstraction for reading batch data
type BatchDataReader interface {
	GetBlockL1InfoTreeIndex(blockNo uint64) (uint64, error)
	GetEffectiveGasPricePercentage(txHash common.Hash) (uint8, error)
	GetL2BlockNosByBatch(batchNo uint64) ([]uint64, error)
	GetForkId(batchNo uint64) (uint64, error)
}

// L1DataReader is an abstraction for reading L1 data
type L1DataReader interface {
	GetBlockGlobalExitRoot(l2BlockNo uint64) (common.Hash, error)
	GetL1InfoTreeUpdateByGer(ger common.Hash) (*types.L1InfoTreeUpdate, error)
	GetL1InfoTreeIndexToRoots() (map[uint64]common.Hash, error)
}

// AccInputHashReader is an abstraction for reading acc input hashes
type AccInputHashReader interface {
	GetAccInputHashForBatchOrPrevious(batchNo uint64) (common.Hash, uint64, error)
	BatchDataReader
	L1DataReader
}

// AccInputHashCalculator is an abstraction for calculating acc input hashes
type AccInputHashCalculator interface {
	Calculate(batchNum uint64) (common.Hash, error)
}

type BaseCalc struct {
	Ctx         context.Context
	Reader      AccInputHashReader
	Tx          kv.Tx
	BlockReader BlockReader
}

var Calculators = map[uint64]func(*BaseCalc) AccInputHashCalculator{
	// pre 7 not supported
	7:  NewFork7Calculator, // etrog
	8:  NewFork7Calculator,
	9:  NewFork7Calculator,
	10: NewFork7Calculator,
	11: NewFork7Calculator,
	12: NewFork7Calculator,
	13: NewFork7Calculator,
}

func NewCalculator(ctx context.Context, tx kv.Tx, reader AccInputHashReader, forkID uint64) (AccInputHashCalculator, error) {
	calcConstructor, ok := Calculators[forkID]
	if !ok {
		return nil, fmt.Errorf("unsupported fork ID: %d", forkID)
	}

	baseCalc := &BaseCalc{
		Ctx:         ctx,
		Reader:      reader,
		Tx:          tx,
		BlockReader: &RawDbBlockReader{tx: tx},
	}

	return calcConstructor(baseCalc), nil
}

func NewCalculatorWithBlockReader(ctx context.Context, tx kv.Tx, reader AccInputHashReader, blockReader BlockReader, forkID uint64) (AccInputHashCalculator, error) {
	calcConstructor, ok := Calculators[forkID]
	if !ok {
		return nil, fmt.Errorf("unsupported fork ID: %d", forkID)
	}

	baseCalc := &BaseCalc{
		Ctx:         ctx,
		Reader:      reader,
		Tx:          tx,
		BlockReader: blockReader,
	}

	return calcConstructor(baseCalc), nil
}

type PreFork7Calculator struct {
	*BaseCalc
}

func NewPreFork7Calculator(bc *BaseCalc) AccInputHashCalculator {
	return PreFork7Calculator{BaseCalc: bc}
}

func (p PreFork7Calculator) Calculate(batchNum uint64) (common.Hash, error) {
	// TODO: warn log - and return error
	// this isn't supported
	return common.Hash{}, nil
}

type Fork7Calculator struct {
	*BaseCalc
}

func NewFork7Calculator(bc *BaseCalc) AccInputHashCalculator {
	return Fork7Calculator{BaseCalc: bc}
}

func (f Fork7Calculator) Calculate(batchNum uint64) (common.Hash, error) {
	accInputHash, returnedBatchNo, err := f.Reader.GetAccInputHashForBatchOrPrevious(batchNum)
	if err != nil {
		return common.Hash{}, err
	}

	// check the forkid of the returnedBatchNo
	forkId, err := f.Reader.GetForkId(returnedBatchNo)
	if err != nil {
		return common.Hash{}, err
	}

	if forkId < uint64(chain.ForkID7Etrog) {
		return common.Hash{}, fmt.Errorf("unsupported fork ID: %d", forkId)
	}

	// if we have it, return it
	if returnedBatchNo == batchNum {
		return accInputHash, nil
	}

	// otherwise calculate it
	accInputHash, err = LocalAccInputHashCalc(f.Ctx, f.Reader, f.BlockReader, f.Tx, batchNum, returnedBatchNo, accInputHash)
	if err != nil {
		return common.Hash{}, err
	}

	return accInputHash, nil
}

func LocalAccInputHashCalc(ctx context.Context, reader AccInputHashReader, blockReader BlockReader, tx kv.Tx, batchNum, startBatchNo uint64, prevAccInputHash common.Hash) (common.Hash, error) {
	var accInputHash common.Hash

	infoTreeIndexes, err := reader.GetL1InfoTreeIndexToRoots()
	if err != nil {
		return common.Hash{}, fmt.Errorf("failed to get l1 info tree indexes: %w", err)
	}
	if len(infoTreeIndexes) == 0 {
		return common.Hash{}, fmt.Errorf("no l1 info tree indexes found")
	}

	// go from injected batch with known batch 0 accinputhash of 0x0...0
	if startBatchNo == 0 {
		startBatchNo = 1
	}

	for i := startBatchNo; i <= batchNum; i++ {
		select {
		case <-ctx.Done():
			return common.Hash{}, ctx.Err()
		default:
			currentForkId, err := reader.GetForkId(i)
			if err != nil {
				return common.Hash{}, fmt.Errorf("failed to get fork id for batch %d: %w", i, err)
			}

			batchBlockNos, err := reader.GetL2BlockNosByBatch(i)
			if err != nil {
				return common.Hash{}, fmt.Errorf("failed to get batch blocks for batch %d: %w", i, err)
			}
			batchBlocks := []*eritypes.Block{}
			var batchTxs []eritypes.Transaction
			var coinbase common.Address
			for in, blockNo := range batchBlockNos {
				block, err := blockReader.ReadBlockByNumber(blockNo)
				if err != nil {
					return common.Hash{}, fmt.Errorf("failed to get block %d: %w", blockNo, err)
				}
				if in == 0 {
					coinbase = block.Coinbase()
				}
				batchBlocks = append(batchBlocks, block)
				batchTxs = append(batchTxs, block.Transactions()...)
			}

			lastBlockNoInPreviousBatch := uint64(0)
			firstBlockInBatch := batchBlocks[0]
			if firstBlockInBatch.NumberU64() != 0 {
				lastBlockNoInPreviousBatch = firstBlockInBatch.NumberU64() - 1
			}

			lastBlockInPreviousBatch, err := blockReader.ReadBlockByNumber(lastBlockNoInPreviousBatch)
			if err != nil {
				return common.Hash{}, err
			}

			batchL2Data, err := utils.GenerateBatchDataFromDb(tx, reader, batchBlocks, lastBlockInPreviousBatch, currentForkId)
			if err != nil {
				return common.Hash{}, fmt.Errorf("failed to generate batch data for batch %d: %w", i, err)
			}

			ger, err := reader.GetBlockGlobalExitRoot(batchBlockNos[len(batchBlockNos)-1])
			if err != nil {
				return common.Hash{}, fmt.Errorf("failed to get global exit root for batch %d: %w", i, err)
			}

			l1InfoTreeUpdate, err := reader.GetL1InfoTreeUpdateByGer(ger)
			if err != nil {
				return common.Hash{}, fmt.Errorf("failed to get l1 info root for batch %d: %w", i, err)
			}
			l1InfoRoot := infoTreeIndexes[0]
			if l1InfoTreeUpdate != nil {
				l1InfoRoot = infoTreeIndexes[l1InfoTreeUpdate.Index]
			}
			limitTs := batchBlocks[len(batchBlocks)-1].Time()
			inputs := utils.AccHashInputs{
				OldAccInputHash: &prevAccInputHash,
				Sequencer:       coinbase,
				BatchData:       batchL2Data,
				L1InfoRoot:      &l1InfoRoot,
				LimitTimestamp:  limitTs,
				ForcedBlockHash: &common.Hash{},
			}
			accInputHash, err = utils.CalculateAccInputHashByForkId(inputs)
			if err != nil {
				return common.Hash{}, fmt.Errorf("failed to calculate accInputHash for batch %d: %w", i, err)
			}
			prevAccInputHash = accInputHash
		}
	}
	return accInputHash, nil
}
