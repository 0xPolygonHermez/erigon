package commands

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/holiman/uint256"

	"github.com/gateway-fm/cdk-erigon-lib/common"
	"github.com/gateway-fm/cdk-erigon-lib/common/hexutility"
	"github.com/gateway-fm/cdk-erigon-lib/gointerfaces/txpool"
	"github.com/gateway-fm/cdk-erigon-lib/kv"
	types2 "github.com/gateway-fm/cdk-erigon-lib/types"
	"github.com/ledgerwatch/erigon/accounts/abi"
	"github.com/ledgerwatch/erigon/common/hexutil"
	"github.com/ledgerwatch/erigon/core"
	"github.com/ledgerwatch/erigon/core/state"
	"github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/core/vm"
	"github.com/ledgerwatch/erigon/rpc"
	"github.com/ledgerwatch/erigon/turbo/adapter/ethapi"
	"github.com/ledgerwatch/erigon/turbo/rpchelper"
	"github.com/ledgerwatch/erigon/turbo/transactions"
	"github.com/ledgerwatch/log/v3"
)

type PreExecAPI interface {
}

// PreExecAPIImpl is implementation of the EthAPI interface based on remote Db access
type PreExecAPIImpl struct {
	*BaseAPI
	ethBackend      rpchelper.ApiBackend
	txPool          txpool.TxpoolClient
	mining          txpool.MiningClient
	gasCache        *GasPriceCache
	db              kv.RoDB
	GasCap          uint64
	ReturnDataLimit int
	logger          log.Logger
}

func NewPreExecAPI(base *BaseAPI, db kv.RoDB, eth rpchelper.ApiBackend, txPool txpool.TxpoolClient, mining txpool.MiningClient, gascap uint64, returnDataLimit int, logger log.Logger) *PreExecAPIImpl {
	return &PreExecAPIImpl{
		BaseAPI:         base,
		db:              db,
		ethBackend:      eth,
		txPool:          txPool,
		mining:          mining,
		gasCache:        NewGasPriceCache(),
		GasCap:          gascap,
		ReturnDataLimit: returnDataLimit,
		logger:          logger,
	}
}

const (
	UnKnown            = 1000
	InsufficientBalane = 1001
	Reverted           = 1002
)

type PreArgs struct {
	ChainId              *big.Int           `json:"chainId,omitempty"`
	From                 *common.Address    `json:"from"`
	To                   *common.Address    `json:"to"`
	Gas                  *hexutil.Uint64    `json:"gas"`
	GasPrice             *hexutil.Big       `json:"gasPrice"`
	MaxFeePerGas         *hexutil.Big       `json:"maxFeePerGas"`
	MaxPriorityFeePerGas *hexutil.Big       `json:"maxPriorityFeePerGas"`
	Value                *hexutil.Big       `json:"value"`
	Nonce                *hexutil.Uint64    `json:"nonce"`
	Data                 *hexutility.Bytes  `json:"data"`
	Input                *hexutility.Bytes  `json:"input"`
	AccessList           *types2.AccessList `json:"accessList"`
}

type PreError struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

func toPreError(err error, result *core.ExecutionResult) PreError {
	preErr := PreError{
		Code: UnKnown,
	}
	if err != nil {
		preErr.Msg = err.Error()
	}
	if result != nil && result.Err != nil {
		preErr.Msg = result.Err.Error()
	}
	if strings.HasPrefix(preErr.Msg, "execution reverted") {
		preErr.Code = Reverted
		if result != nil {
			preErr.Msg, _ = abi.UnpackRevert(result.Revert())
		}
	}
	if strings.HasPrefix(preErr.Msg, "out of gas") {
		preErr.Code = Reverted
	}
	if strings.HasPrefix(preErr.Msg, "insufficient funds for transfer") {
		preErr.Code = InsufficientBalane
	}
	if strings.HasPrefix(preErr.Msg, "insufficient balance for transfer") {
		preErr.Code = InsufficientBalane
	}
	if strings.HasPrefix(preErr.Msg, "insufficient funds for gas * price") {
		preErr.Code = InsufficientBalane
	}
	return preErr
}

type PreResult struct {
	Trace     interface{}  `json:"trace"`
	Logs      []*types.Log `json:"logs"`
	StateDiff interface{}  `json:"stateDiff,omitempty"`
	Error     PreError     `json:"error,omitempty"`
	GasUsed   uint64       `json:"gasUsed"`
}

func (api *PreExecAPIImpl) TraceMany(ctx context.Context, origins []PreArgs) ([]PreResult, error) {
	preResList := make([]PreResult, 0)
	tx, err := api.db.BeginRo(ctx)
	if err != nil {
		return nil, err
	}
	chainConfig, err := api.chainConfig(tx)
	if err != nil {
		return nil, err
	}
	engine := api.engine()
	blockNrOrHash := rpc.BlockNumberOrHashWithNumber(rpc.LatestBlockNumber)
	blockNumber, hash, _, err := rpchelper.GetCanonicalBlockNumber(blockNrOrHash, tx, api.filters) // DoCall cannot be executed on non-canonical blocks
	if err != nil {
		return nil, err
	}
	block, err := api.BaseAPI.blockWithSenders(tx, hash, blockNumber)
	if err != nil {
		return nil, err
	}
	if block == nil {
		return nil, nil
	}

	stateReader, err := rpchelper.CreateStateReader(ctx, tx, blockNrOrHash, 0, api.filters, api.stateCache, api.historyV3(tx), chainConfig.ChainName)
	if err != nil {
		return nil, err
	}
	state := state.New(stateReader)

	header := block.Header()
	for i := 0; i < len(origins); i++ {
		origin := origins[i]
		if origin.Nonce == nil {
			preResList = append(preResList, PreResult{
				Error: PreError{
					Code: UnKnown,
					Msg:  "nonce is nil",
				},
			})
			continue
		}
		if i > 0 && (uint64)(*origin.Nonce) <= (uint64)(*origins[i-1].Nonce) {
			preResList = append(preResList, PreResult{
				Error: PreError{
					Code: UnKnown,
					Msg:  fmt.Sprintf("nonce decreases, tx index %d has nonce %d, tx index %d has nonce %d", i-1, (uint64)(*origins[i-1].Nonce), i, (uint64)(*origin.Nonce)),
				},
			})
			continue
		}
		args := ethapi.CallArgs{
			From:                 origin.From,
			To:                   origin.To,
			Gas:                  origin.Gas,
			GasPrice:             origin.GasPrice,
			MaxFeePerGas:         origin.MaxFeePerGas,
			MaxPriorityFeePerGas: origin.MaxPriorityFeePerGas,
			Value:                origin.Value,
			Data:                 origin.Data,
			AccessList:           origin.AccessList,
			ChainID:              (*hexutil.Big)(big.NewInt(1)),
		}
		// Get a new instance of the EVM.
		var baseFee *uint256.Int
		if header != nil && header.BaseFee != nil {
			var overflow bool
			baseFee, overflow = uint256.FromBig(header.BaseFee)
			if overflow {
				return nil, fmt.Errorf("header.BaseFee uint256 overflow")
			}
		}
		msg, err := args.ToMessage(api.GasCap, baseFee)
		if err != nil {
			preResList = append(preResList, PreResult{
				Error: PreError{
					Code: UnKnown,
					Msg:  err.Error(),
				},
			})
			continue
		}
		txHash := common.BigToHash(big.NewInt(int64(i)))
		blockCtx := transactions.NewEVMBlockContext(engine, header, blockNrOrHash.RequireCanonical, tx, api._blockReader)
		txCtx := core.NewEVMTxContext(msg)

		var ot OeTracer
		traceResult := &TraceCallResult{Trace: []*ParityTrace{}}
		ot.r = traceResult
		ot.traceAddr = []int{}

		evm := vm.NewEVM(blockCtx, txCtx, state, chainConfig, vm.Config{Tracer: &ot, Debug: true, NoBaseFee: true})
		if err != nil {
			preResList = append(preResList, PreResult{
				Error: PreError{
					Code: UnKnown,
					Msg:  err.Error(),
				},
			})
			continue
		}

		// Execute the message.
		gp := new(core.GasPool).AddGas(msg.Gas())
		result, err := core.ApplyMessage(evm, msg, gp, true /* refunds */, false /* gasBailout */)
		if err != nil {
			preRes := PreResult{
				Error: toPreError(err, result),
			}
			if result != nil {
				preRes.GasUsed = result.UsedGas
			}
			preResList = append(preResList, preRes)
			continue
		}

		res := ot.r.Trace

		blockHash := header.Hash()
		blockNum := header.Number.Uint64()
		txIndex := uint64(i)
		for _, pt := range ot.r.Trace {
			pt.BlockHash = &blockHash
			pt.BlockNumber = &blockNum
			pt.TransactionHash = &txHash
			pt.TransactionPosition = &txIndex
		}

		preRes := PreResult{
			Trace: res,
			Logs:  state.GetLogs(txHash),
		}
		if result != nil {
			preRes.GasUsed = result.UsedGas
			if result.Failed() {
				preRes.Error = toPreError(err, result)
			}
		}

		if preRes.Error.Msg == "" && len(res) > 0 && res[0].Error != "" {
			preRes.Error = PreError{
				Code: Reverted,
				Msg:  res[0].Error,
			}
		}
		preResList = append(preResList, preRes)
	}
	return preResList, nil
}
