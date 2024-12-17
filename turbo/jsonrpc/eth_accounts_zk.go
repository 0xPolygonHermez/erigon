package jsonrpc

import (
	"context"
	"fmt"
	"strings"

	libcommon "github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/common/hexutil"
	"github.com/ledgerwatch/erigon-lib/gointerfaces"
	txpool_proto "github.com/ledgerwatch/erigon-lib/gointerfaces/txpool"
	"github.com/ledgerwatch/erigon/rpc"
	"github.com/ledgerwatch/erigon/turbo/rpchelper"
	"github.com/ledgerwatch/erigon/zk/sequencer"
	"github.com/ledgerwatch/erigon/zkevm/hex"
	"github.com/ledgerwatch/erigon/zkevm/jsonrpc/client"
	"google.golang.org/grpc"
)

func (api *APIImpl) sendGetTransactionCountToSequencer(rpcUrl string, address libcommon.Address, blockNrOrHash *rpc.BlockNumberOrHash) (*hexutil.Uint64, error) {
	addressHex := "0x" + hex.EncodeToString(address.Bytes())
	var blockNrOrHashValue interface{}
	if blockNrOrHash != nil {
		if blockNrOrHash.BlockNumber != nil {
			bn := *blockNrOrHash.BlockNumber
			blockNrOrHashValue = bn.MarshallJson()
		} else if blockNrOrHash.BlockHash != nil {
			blockNrOrHashValue = "0x" + hex.EncodeToString(blockNrOrHash.BlockHash.Bytes())
		}
	}

	res, err := client.JSONRPCCall(rpcUrl, "eth_getTransactionCount", addressHex, blockNrOrHashValue)
	if err != nil {
		return nil, err
	}

	if res.Error != nil {
		return nil, fmt.Errorf("RPC error response: %s", res.Error.Message)
	}

	// hash comes in escaped quotes, so we trim them here
	// \"0x1234\" -> 0x1234
	hashHex := strings.Trim(string(res.Result), "\"")

	// now convert to a uint
	decoded, err := hexutil.DecodeUint64(hashHex)
	if err != nil {
		return nil, err
	}

	result := hexutil.Uint64(decoded)

	return &result, nil
}

var nonceGoneBackwards = false
var nonce = uint64(0)
var lastNonce = uint64(0)

// GetTransactionCount implements eth_getTransactionCount. Returns the number of transactions sent from an address (the nonce).
func (api *APIImpl) GetTransactionCount(ctx context.Context, address libcommon.Address, blockNrOrHash *rpc.BlockNumberOrHash) (*hexutil.Uint64, error) {
	// zkevm: forward requests to the sequencer
	if !sequencer.IsSequencer() {
		res, err := api.sendGetTransactionCountToSequencer(api.l2RpcUrl, address, blockNrOrHash)
		if err != nil {
			return nil, err
		}
		return res, nil
	}

	// if not set, use latest
	if blockNrOrHash == nil {
		tmp := rpc.BlockNumberOrHashWithNumber(rpc.LatestBlockNumber)
		blockNrOrHash = &tmp
	}

	if blockNrOrHash.BlockNumber != nil && *blockNrOrHash.BlockNumber == rpc.PendingBlockNumber {
		reply, err := api.txPool.Nonce(ctx, &txpool_proto.NonceRequest{
			Address: gointerfaces.ConvertAddressToH160(address),
		}, &grpc.EmptyCallOption{})
		if err != nil {
			return nil, err
		}

		nonce = reply.Nonce
		nonceGoneBackwards = nonce < lastNonce
		if nonceGoneBackwards {
			// trigger breakpoint
			fmt.Println("nonceGoneBackwards", nonceGoneBackwards, nonce, lastNonce, reply.Found, reply.Nonce)
		}
		lastNonce = nonce

		if reply.Found {
			reply.Nonce++
			return (*hexutil.Uint64)(&reply.Nonce), nil
		}
	}
	tx, err1 := api.db.BeginRo(ctx)
	if err1 != nil {
		return nil, fmt.Errorf("getTransactionCount cannot open tx: %w", err1)
	}
	defer tx.Rollback()

	latestExecutedBlockNumber, err := rpchelper.GetLatestExecutedBlockNumber(tx)
	if err != nil {
		return nil, fmt.Errorf("getTransactionCount cannot get latest executed block number: %w", err)
	}

	if blockNrOrHash.BlockNumber != nil && *blockNrOrHash.BlockNumber == rpc.BlockNumber(latestExecutedBlockNumber) {
		blockNumber := rpc.BlockNumber(rpc.LatestExecutedBlockNumber)
		blockNrOrHash.BlockNumber = &blockNumber
	}

	reader, err := rpchelper.CreateStateReader(ctx, tx, *blockNrOrHash, 0, api.filters, api.stateCache, api.historyV3(tx), "")
	if err != nil {
		return nil, err
	}
	nonce := hexutil.Uint64(0)
	acc, err := reader.ReadAccountData(address)
	if acc == nil || err != nil {
		return &nonce, err
	}

	return (*hexutil.Uint64)(&acc.Nonce), err
}
