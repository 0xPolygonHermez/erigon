package commands

import (
	"encoding/json"
	"fmt"

	"github.com/gateway-fm/cdk-erigon-lib/common"
	"github.com/ledgerwatch/erigon/zkevm/jsonrpc/client"
)

func (api *APIImpl) forwardGetTransactionByHash(rpcUrl string, txnHash common.Hash, includeExtraInfo bool) (json.RawMessage, error) {
	asString := txnHash.String()
	res, err := client.JSONRPCCall(rpcUrl, "eth_getTransactionByHash", asString, includeExtraInfo)
	if err != nil {
		return nil, err
	}

	if res.Error != nil {
		return nil, fmt.Errorf("RPC error response is: %s", res.Error.Message)
	}

	return res.Result, nil
}
