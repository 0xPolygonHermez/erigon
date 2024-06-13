package commands

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"time"

	libcommon "github.com/gateway-fm/cdk-erigon-lib/common"
	"github.com/gateway-fm/cdk-erigon-lib/common/hexutility"
	"github.com/ledgerwatch/erigon/accounts/abi"
	"github.com/ledgerwatch/erigon/chain"
	"github.com/ledgerwatch/erigon/core/state"
	"github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/rpc"
	"github.com/ledgerwatch/erigon/turbo/adapter/ethapi"
	"github.com/ledgerwatch/erigon/turbo/rpchelper"
	"github.com/ledgerwatch/erigon/turbo/services"
	"github.com/ledgerwatch/erigon/turbo/transactions"
)

type multiCallResp struct {
	Results []*callResult   `json:"results"`
	Stats   *multiCallStats `json:"stats"`
}

type callResult struct {
	Code      int              `json:"code"`
	Err       string           `json:"err"`
	FromCache bool             `json:"fromCache"`
	Result    hexutility.Bytes `json:"result"`
	GasUsed   int64            `json:"gasUsed"`
	TimeCost  float64          `json:"timeCost"`
}

type multiCallStats struct {
	BlockNum     int64          `json:"blockNum"`
	BlockHash    libcommon.Hash `json:"blockHash"`
	BlockTime    int64          `json:"blockTime"`
	Success      bool           `json:"success"`
	CacheEnabled bool           `json:"cacheEnabled"`
}

const (
	singleCallTimeout = 1 * time.Second
	multiCallLimit    = 50

	// client param error
	errCodeTxArgs               = -40000
	errNativeMethodNotFound     = -40001
	errNativeMethodInput        = -40002
	errNativeMethodInputAddress = -40003

	// evm processing error
	errNativeMethodOutput     = -40010
	errNativeMethodStateError = -40011
	errMessageExecuting       = -40012
	errEVMCancelled           = -40013
	errEVMReverted            = -40014
	errEVMFastFailed          = -40015

	// internal error
	errUnderlyingDB = -40020
	errLoadingState = -40021
)

const (
	nativeAddr = "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
)

var (
	// copied from: accounts/abi/abi_test.go
	Uint8, _   = abi.NewType("uint8", "", nil)
	Uint256, _ = abi.NewType("uint256", "", nil)
	String, _  = abi.NewType("string", "", nil)
	Address, _ = abi.NewType("address", "", nil)

	erc20ABI = abi.ABI{
		Methods: map[string]abi.Method{
			"name":        funcName,
			"symbol":      funcSymbol,
			"decimals":    funcDecimals,
			"totalSupply": funcTotalSupply,
			"balanceOf":   funcBalanceOf,
		},
	}

	funcName = abi.NewMethod("name", "name", abi.Function, "", false, false,
		[]abi.Argument{},
		[]abi.Argument{
			{Name: "", Type: String, Indexed: false},
		},
	)
	funcSymbol = abi.NewMethod("symbol", "symbol", abi.Function, "", false, false,
		[]abi.Argument{},
		[]abi.Argument{
			{Name: "", Type: String, Indexed: false},
		},
	)
	funcDecimals = abi.NewMethod("decimals", "decimals", abi.Function, "", false, false,
		[]abi.Argument{},
		[]abi.Argument{
			{Name: "", Type: Uint8, Indexed: false},
		},
	)
	funcTotalSupply = abi.NewMethod("totalSupply", "totalSupply", abi.Function, "", false, false,
		[]abi.Argument{},
		[]abi.Argument{
			{Name: "", Type: Uint256, Indexed: false},
		},
	)
	funcBalanceOf = abi.NewMethod("balanceOf", "balanceOf", abi.Function, "", false, false,
		[]abi.Argument{
			{Name: "", Type: Address, Indexed: false},
		},
		[]abi.Argument{
			{Name: "", Type: Uint256, Indexed: false},
		},
	)
)

func handleNative(ctx context.Context, state *state.IntraBlockState, msg types.Message) ([]byte, int, error) {
	data := msg.Data()
	method, err := erc20ABI.MethodById(data)
	if err != nil {
		return nil, errNativeMethodNotFound, err
	}

	switch method.Name {
	case "name", "symbol":
		res, err := method.Outputs.Pack("ETH")
		if err != nil {
			return nil, errNativeMethodOutput, err
		}
		return res, 0, nil
	case "decimals":
		res, err := method.Outputs.Pack(uint8(18))
		if err != nil {
			return nil, errNativeMethodOutput, err
		}
		return res, 0, nil
	case "totalSupply":
		res, err := method.Outputs.Pack(big.NewInt(1_000_000_000_000_000_000)) // 1 ETH
		if err != nil {
			return nil, errNativeMethodOutput, err
		}
		return res, 0, nil
	case "balanceOf":
		inputs, err := method.Inputs.Unpack(data[4:])
		if err != nil || len(inputs) == 0 {
			return nil, errNativeMethodInput, err
		}
		address, ok := inputs[0].(libcommon.Address)
		if !ok {
			return nil, errNativeMethodInputAddress, fmt.Errorf("input address error")
		}
		b := big.NewInt(0).SetBytes(state.GetBalance(address).Bytes())
		balance, err := method.Outputs.Pack(b)
		if err != nil {
			return nil, errNativeMethodOutput, err
		}
		if state.Error() != nil {
			return nil, errNativeMethodStateError, state.Error()
		}
		return balance, 0, nil
	default:
		return nil, errNativeMethodNotFound, fmt.Errorf("method not found")
	}
}

func (api *APIImpl) doOneCall(ctx context.Context, args ethapi.CallArgs, blockNrOrHash rpc.BlockNumberOrHash, block *types.Block, overrides *ethapi.StateOverrides, chainConfig *chain.Config, headerReader services.HeaderReader, disableCache bool) (*callResult, error) {
	var err error
	var result = &callResult{}

	start := time.Now()

	// make sure this will be called prior to the SetCallCache defer func on returning
	defer func() {
		result.TimeCost = time.Since(start).Seconds()
	}()

	// handle native call
	msg, err := args.ToMessage(api.GasCap, nil)
	if err != nil {
		result.Code = errCodeTxArgs
		result.Err = err.Error()
		return result, err
	}

	tx, err := api.db.BeginRo(ctx)
	if err != nil {
		result.Code = errUnderlyingDB
		result.Err = err.Error()
		return result, err
	}
	defer tx.Rollback()

	stateReader, err := rpchelper.CreateStateReader(ctx, tx, blockNrOrHash, 0, api.filters, api.stateCache, api.historyV3(tx), chainConfig.ChainName)
	if err != nil {
		result.Code = errLoadingState
		result.Err = err.Error()
		return result, err
	}

	// skip EVM if requests for native token
	if strings.ToLower(msg.To().Hex()) == nativeAddr {
		state := state.New(stateReader)
		res, code, err := handleNative(ctx, state, msg)
		if err != nil {
			result.Code = code
			result.Err = err.Error()
		}
		result.Result = res
		return result, err
	}

	engine := api.engine()
	header := block.HeaderNoCopy()
	evmRet, err := transactions.DoCall(ctx, engine, args, tx, blockNrOrHash, header, overrides, api.GasCap, chainConfig, stateReader, api._blockReader, api.evmCallTimeout)
	if err != nil {
		result.Code = errMessageExecuting
		result.Err = err.Error()
		return result, err
	}

	if evmRet.Err != nil {
		e := evmRet.Err
		if len(evmRet.Revert()) > 0 {
			e = ethapi.NewRevertError(evmRet)
		}
		result.Code = errEVMReverted
		result.Err = e.Error()
		return result, e
	}

	result.Result = evmRet.Return()
	result.GasUsed = int64(evmRet.UsedGas)

	return result, nil
}

func (api *APIImpl) MultiCall(ctx context.Context, args []ethapi.CallArgs, blockNrOrHash rpc.BlockNumberOrHash, pfastFail, puseParallel, pdisableCache *bool, overrides *ethapi.StateOverrides) (resp *multiCallResp, err error) {

	// maximum calls check
	if len(args) > multiCallLimit {
		return nil, fmt.Errorf("calls exceed limit, expected: <%v, actual: %v", multiCallLimit, len(args))
	}

	setb := func(p *bool, d bool) bool {
		if p == nil {
			return d
		}
		return *p
	}

	fastFail := setb(pfastFail, true)
	useParallel := setb(puseParallel, true)
	disableCache := setb(pdisableCache, false)

	tx, err := api.db.BeginRo(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	chainConfig, err := api.chainConfig(tx)
	if err != nil {
		return nil, err
	}

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

	ret := make([]*callResult, len(args))
	blockTime := block.Time()
	stats := &multiCallStats{
		BlockNum:     block.Number().Int64(),
		BlockHash:    block.Hash(),
		BlockTime:    int64(blockTime),
		Success:      true,
		CacheEnabled: !disableCache,
	}

	ctx, cancel := context.WithTimeout(ctx, singleCallTimeout)
	defer cancel()

	if useParallel {
		// run in parallel
		var wg sync.WaitGroup
		for i, arg := range args {
			wg.Add(1)
			go func(i int, arg ethapi.CallArgs) {
				defer wg.Done()

				// state is not reentrancy in concurrent scenarios, so use a copy
				r, _ := api.doOneCall(ctx, arg, blockNrOrHash, block, overrides, chainConfig, api._blockReader, disableCache)
				ret[i] = r
				if r.Err != "" {
					stats.Success = false
					if fastFail {
						cancel()
					}
					return
				}
			}(i, arg)
		}
		wg.Wait()

		return &multiCallResp{Results: ret, Stats: stats}, nil
	}

	// run in sequence
	failedOnce := false
	for i, arg := range args {
		if failedOnce {
			ret[i] = &callResult{
				Code: errEVMFastFailed,
				Err:  "fast failed",
			}
			continue
		}

		r, _ := api.doOneCall(ctx, arg, blockNrOrHash, block, overrides, chainConfig, api._blockReader, disableCache)
		ret[i] = r
		if r.Err != "" {
			stats.Success = false
			if fastFail {
				failedOnce = true
			}
			continue
		}
	}

	return &multiCallResp{Results: ret, Stats: stats}, nil
}
