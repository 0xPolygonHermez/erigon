// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Package core implements the Ethereum consensus protocol.
package core

import (
	"fmt"
	"math/big"
	"time"

	libcommon "github.com/ledgerwatch/erigon-lib/common"

	"github.com/ledgerwatch/erigon/chain"

	"github.com/ledgerwatch/erigon-lib/kv"

	"github.com/ledgerwatch/erigon/common/math"
	"github.com/ledgerwatch/erigon/consensus"
	"github.com/ledgerwatch/erigon/consensus/misc"
	"github.com/ledgerwatch/erigon/core/state"
	"github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/core/vm"
)

// ExecuteBlockEphemerally runs a block from provided stateReader and
// writes the result to the provided stateWriter
func ExecuteBlockEphemerallyZk(
	chainConfig *chain.Config, vmConfig *vm.Config,
	blockHashFunc func(n uint64) libcommon.Hash,
	engine consensus.Engine, block *types.Block,
	stateReader state.StateReader,
	stateWriter state.WriterWithChangeSets,
	chainReader consensus.ChainHeaderReader,
	getTracer func(txIndex int, txHash libcommon.Hash) (vm.EVMLogger, error),
	dbTx kv.RwTx,
	roHermezDb state.ReadOnlyHermezDb,
) (*EphemeralExecResult, error) {

	defer BlockExecutionTimer.UpdateDuration(time.Now())
	block.Uncles()
	ibs := state.New(stateReader)
	header := block.Header()

	usedGas := new(uint64)
	gp := new(GasPool)
	gp.AddGas(block.GasLimit())

	var (
		rejectedTxs []*RejectedTx
		includedTxs types.Transactions
		receipts    types.Receipts
	)

	var excessDataGas *big.Int
	if chainReader != nil {
		// TODO(eip-4844): understand why chainReader is sometimes nil (e.g. certain test cases)
		ph := chainReader.GetHeaderByHash(block.ParentHash())
		if ph != nil {
			excessDataGas = ph.ExcessDataGas
		}
	}
	if !vmConfig.ReadOnly {
		if err := InitializeBlockExecution(engine, chainReader, block.Header(), block.Transactions(), block.Uncles(), chainConfig, ibs, excessDataGas); err != nil {
			return nil, err
		}
	}

	if chainConfig.DAOForkBlock != nil && chainConfig.DAOForkBlock.Cmp(block.Number()) == 0 {
		misc.ApplyDAOHardFork(ibs)
	}

	// the state root of the previous block is written into state
	// this should be fine since we get block 0 from the datastream
	stateRoot, err := roHermezDb.GetStateRoot(block.NumberU64() - 1)
	if err != nil {
		return nil, err
	}
	ibs.PreExecuteStateSet(chainConfig, block, &stateRoot)

	noop := state.NewNoopWriter()
	for i, tx := range block.Transactions() {
		ibs.Prepare(tx.Hash(), block.Hash(), i)
		writeTrace := false
		if vmConfig.Debug && vmConfig.Tracer == nil {
			tracer, err := getTracer(i, tx.Hash())
			if err != nil {
				return nil, fmt.Errorf("could not obtain tracer: %w", err)
			}
			vmConfig.Tracer = tracer
			writeTrace = true
		}

		gp.Reset(block.GasLimit())

		effectiveGasPricePercentage, err := roHermezDb.GetEffectiveGasPricePercentage(tx.Hash())
		if err != nil {
			return nil, err
		}

		receipt, _, err := ApplyTransaction(chainConfig, blockHashFunc, engine, nil, gp, ibs, noop, header, tx, usedGas, *vmConfig, excessDataGas, effectiveGasPricePercentage)
		if writeTrace {
			if ftracer, ok := vmConfig.Tracer.(vm.FlushableTracer); ok {
				ftracer.Flush(tx)
			}

			vmConfig.Tracer = nil
		}

		if err != nil {
			if !vmConfig.StatelessExec {
				return nil, fmt.Errorf("could not apply tx %d from block %d [%v]: %w", i, block.NumberU64(), tx.Hash().Hex(), err)
			}
			rejectedTxs = append(rejectedTxs, &RejectedTx{i, err.Error()})
		} else {
			includedTxs = append(includedTxs, tx)
			if !vmConfig.NoReceipts {
				receipts = append(receipts, receipt)
			}
		}

	}

	// [zkevm] todo: calculate it
	var l1InfoRoot *libcommon.Hash

	ibs.PostExecuteStateSet(chainConfig, block.NumberU64(), l1InfoRoot, &stateRoot)

	receiptSha := types.DeriveSha(receipts)
	// [zkevm] todo
	//if !vmConfig.StatelessExec && chainConfig.IsByzantium(header.Number.Uint64()) && !vmConfig.NoReceipts && receiptSha != block.ReceiptHash() {
	//	return nil, fmt.Errorf("mismatched receipt headers for block %d (%s != %s)", block.NumberU64(), receiptSha.Hex(), block.ReceiptHash().Hex())
	//}

	// in zkEVM we don't have headers to check GasUsed against
	//if !vmConfig.StatelessExec && *usedGas != header.GasUsed && header.GasUsed > 0 {
	//	return nil, fmt.Errorf("gas used by execution: %d, in header: %d", *usedGas, header.GasUsed)
	//}

	var bloom types.Bloom
	if !vmConfig.NoReceipts {
		bloom = types.CreateBloom(receipts)
		// [zkevm] todo
		//if !vmConfig.StatelessExec && bloom != header.Bloom {
		//	return nil, fmt.Errorf("bloom computed by execution: %x, in header: %x", bloom, header.Bloom)
		//}
	}
	if !vmConfig.ReadOnly {
		txs := block.Transactions()
		if _, _, _, err := FinalizeBlockExecution(engine, stateReader, block.Header(), txs, block.Uncles(), stateWriter, chainConfig, ibs, receipts, block.Withdrawals(), chainReader, false, excessDataGas); err != nil {
			return nil, err
		}
	}
	blockLogs := ibs.Logs()
	execRs := &EphemeralExecResult{
		TxRoot:      types.DeriveSha(includedTxs),
		ReceiptRoot: receiptSha,
		Bloom:       bloom,
		LogsHash:    rlpHash(blockLogs),
		Receipts:    receipts,
		Difficulty:  (*math.HexOrDecimal256)(header.Difficulty),
		GasUsed:     math.HexOrDecimal64(*usedGas),
		Rejected:    rejectedTxs,
	}

	return execRs, nil
}
