package acc_input_hash

import (
	"fmt"
	"github.com/ledgerwatch/erigon-lib/common"
	"context"
)

type AccInputHashReader interface {
	GetAccInputHashForBatchOrPrevious(batchNo uint64) (common.Hash, uint64, error)
}

type AccInputHashCalculator interface {
	Calculate(ctx context.Context, batchNum uint64, reader AccInputHashReader) (common.Hash, error)
}

var Calculators = map[uint64]AccInputHashCalculator{
	// pre 7 not supported
	7:  Fork7Calculator{}, // etrog
	8:  Fork7Calculator{},
	9:  Fork7Calculator{},
	10: Fork7Calculator{},
	11: Fork7Calculator{},
	12: Fork7Calculator{},
	13: Fork7Calculator{},
}

func CalculateAccInputHash(ctx context.Context, forkID uint64, batchNum uint64, reader AccInputHashReader) (common.Hash, error) {
	calc, ok := Calculators[forkID]
	if !ok {
		return common.Hash{}, fmt.Errorf("unsupported fork ID: %d", forkID)
	}
	return calc.Calculate(ctx, batchNum, reader)
}

type PreFork7Calculator struct{}

func (p PreFork7Calculator) Calculate(ctx context.Context, batchNum uint64, reader AccInputHashReader) (common.Hash, error) {
	// TODO: warn log - and return error
	// this isn't supported
	return common.Hash{}, nil
}

type Fork7Calculator struct{}

func (f Fork7Calculator) Calculate(ctx context.Context, batchNum uint64, reader AccInputHashReader) (common.Hash, error) {

	accInputHash, returnedBatchNo, err := reader.GetAccInputHashForBatchOrPrevious(batchNum)
	if err != nil {
		return common.Hash{}, err
	}

	// if we have it, return it!
	if returnedBatchNo == batchNum {
		return accInputHash, nil
	}

	// otherwise calculate it!

	return accInputHash, nil
}
