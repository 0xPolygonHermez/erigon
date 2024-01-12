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

package vm

import (
	"sync/atomic"

	"github.com/ledgerwatch/erigon/chain"

	"github.com/ledgerwatch/erigon/core/vm/evmtypes"
)

// NewZKEVM returns a new EVM with zkEVM rules. The returned EVM is not thread safe and should
// only ever be used *once*.
func NewZKEVM(blockCtx evmtypes.BlockContext, txCtx evmtypes.TxContext, state evmtypes.IntraBlockState, chainConfig *chain.Config, vmConfig Config) *EVM {
	evm := &EVM{
		context:         blockCtx,
		txContext:       txCtx,
		intraBlockState: state,
		config:          vmConfig,
		chainConfig:     chainConfig,
		chainRules:      chainConfig.Rules(blockCtx.BlockNumber, blockCtx.Time),
	}

	evm.interpreter = NewZKEVMInterpreter(evm, vmConfig)

	return evm
}

func (evm *EVM) ResetBetweenBlocks_zkevm(blockCtx evmtypes.BlockContext, txCtx evmtypes.TxContext, ibs evmtypes.IntraBlockState, vmConfig Config, chainRules *chain.Rules) {
	evm.context = blockCtx
	evm.txContext = txCtx
	evm.intraBlockState = ibs
	evm.config = vmConfig
	evm.chainRules = chainRules

	evm.interpreter = NewZKEVMInterpreter(evm, vmConfig)

	// ensure the evm is reset to be used again
	atomic.StoreInt32(&evm.abort, 0)
}
