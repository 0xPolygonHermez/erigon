package commands

import (
	"math/big"

	"github.com/gateway-fm/cdk-erigon-lib/common"
	"github.com/gateway-fm/cdk-erigon-lib/common/hexutility"
	types2 "github.com/gateway-fm/cdk-erigon-lib/types"
	"github.com/holiman/uint256"
	"github.com/ledgerwatch/erigon/common/hexutil"
	"github.com/ledgerwatch/erigon/common/math"
	"github.com/ledgerwatch/erigon/core/types"
	zktx "github.com/ledgerwatch/erigon/zk/tx"
)

func (api *BaseAPI) SetL2RpcUrl(url string) {
	api.l2RpcUrl = url
}

func (api *BaseAPI) GetL2RpcUrl() string {
	if len(api.l2RpcUrl) == 0 {
		panic("L2RpcUrl is not set")
	}
	return api.l2RpcUrl
}

// RPCTransaction represents a transaction that will serialize to the RPC representation of a transaction
type RPCTransaction struct {
	BlockHash        *common.Hash       `json:"blockHash"`
	BlockNumber      *hexutil.Big       `json:"blockNumber"`
	From             common.Address     `json:"from"`
	Gas              hexutil.Uint64     `json:"gas"`
	GasPrice         *hexutil.Big       `json:"gasPrice,omitempty"`
	Tip              *hexutil.Big       `json:"maxPriorityFeePerGas,omitempty"`
	FeeCap           *hexutil.Big       `json:"maxFeePerGas,omitempty"`
	Hash             common.Hash        `json:"hash"`
	Input            hexutility.Bytes   `json:"input"`
	Nonce            hexutil.Uint64     `json:"nonce"`
	To               *common.Address    `json:"to"`
	TransactionIndex *hexutil.Uint64    `json:"transactionIndex"`
	Value            *hexutil.Big       `json:"value"`
	Type             hexutil.Uint64     `json:"type"`
	Accesses         *types2.AccessList `json:"accessList,omitempty"`
	ChainID          *hexutil.Big       `json:"chainId,omitempty"`
	V                *hexutil.Big       `json:"v"`
	R                *hexutil.Big       `json:"r"`
	S                *hexutil.Big       `json:"s"`
	L2Hash           *common.Hash       `json:"l2Hash,omitempty"`
}

// newRPCTransaction returns a transaction that will serialize to the RPC
// representation, with the given location metadata set (if available).
func newRPCTransaction(tx types.Transaction, blockHash common.Hash, blockNumber uint64, index uint64, baseFee *big.Int, includeExtraInfo bool) *RPCTransaction {
	// Determine the signer. For replay-protected transactions, use the most permissive
	// signer, because we assume that signers are backwards-compatible with old
	// transactions. For non-protected transactions, the homestead signer signer is used
	// because the return value of ChainId is zero for those transactions.
	chainId := uint256.NewInt(0)
	result := &RPCTransaction{
		Type:  hexutil.Uint64(tx.Type()),
		Gas:   hexutil.Uint64(tx.GetGas()),
		Hash:  tx.Hash(),
		Input: hexutility.Bytes(tx.GetData()),
		Nonce: hexutil.Uint64(tx.GetNonce()),
		To:    tx.GetTo(),
		Value: (*hexutil.Big)(tx.GetValue().ToBig()),
	}
	switch t := tx.(type) {
	case *types.LegacyTx:
		chainId = types.DeriveChainId(&t.V)
		// if a legacy transaction has an EIP-155 chain id, include it explicitly, otherwise chain id is not included
		result.ChainID = (*hexutil.Big)(chainId.ToBig())
		result.GasPrice = (*hexutil.Big)(t.GasPrice.ToBig())
		result.V = (*hexutil.Big)(t.V.ToBig())
		result.R = (*hexutil.Big)(t.R.ToBig())
		result.S = (*hexutil.Big)(t.S.ToBig())
	case *types.AccessListTx:
		chainId.Set(t.ChainID)
		result.ChainID = (*hexutil.Big)(chainId.ToBig())
		result.GasPrice = (*hexutil.Big)(t.GasPrice.ToBig())
		result.V = (*hexutil.Big)(t.V.ToBig())
		result.R = (*hexutil.Big)(t.R.ToBig())
		result.S = (*hexutil.Big)(t.S.ToBig())
		result.Accesses = &t.AccessList
	case *types.DynamicFeeTransaction:
		chainId.Set(t.ChainID)
		result.ChainID = (*hexutil.Big)(chainId.ToBig())
		result.Tip = (*hexutil.Big)(t.Tip.ToBig())
		result.FeeCap = (*hexutil.Big)(t.FeeCap.ToBig())
		result.V = (*hexutil.Big)(t.V.ToBig())
		result.R = (*hexutil.Big)(t.R.ToBig())
		result.S = (*hexutil.Big)(t.S.ToBig())
		result.Accesses = &t.AccessList
		baseFee, overflow := uint256.FromBig(baseFee)
		if baseFee != nil && !overflow && blockHash != (common.Hash{}) {
			// price = min(tip + baseFee, gasFeeCap)
			price := math.Min256(new(uint256.Int).Add(tx.GetTip(), baseFee), tx.GetFeeCap())
			result.GasPrice = (*hexutil.Big)(price.ToBig())
		} else {
			result.GasPrice = nil
		}
	}
	signer := types.LatestSignerForChainID(chainId.ToBig())
	result.From, _ = tx.Sender(*signer)
	if blockHash != (common.Hash{}) {
		result.BlockHash = &blockHash
		result.BlockNumber = (*hexutil.Big)(new(big.Int).SetUint64(blockNumber))
		result.TransactionIndex = (*hexutil.Uint64)(&index)
	}

	if includeExtraInfo {
		l2TxHash, err := zktx.ComputeL2TxHash(
			tx.GetChainID().ToBig(),
			tx.GetValue(),
			tx.GetPrice(),
			tx.GetNonce(),
			tx.GetGas(),
			tx.GetTo(),
			&result.From,
			tx.GetData(),
		)
		if err == nil {
			result.L2Hash = &l2TxHash
		}
	}

	return result
}
