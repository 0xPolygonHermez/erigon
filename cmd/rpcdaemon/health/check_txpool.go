package health

import (
	"context"
	"fmt"
	"github.com/ledgerwatch/erigon/turbo/jsonrpc"
)

func checkTxPool(txApi TxPoolAPI, ethApi EthAPI) error {
	if txApi == nil {
		return fmt.Errorf("no connection to the Erigon server or `tx_pool` namespace isn't enabled")
	}

	if ethApi == nil {
		return fmt.Errorf("no connection to the Erigon server or `eth` namespace isn't enabled")
	}

	var err error

	data, err := txApi.Content(context.TODO())
	if err != nil {
		return err
	}

	contentMap, ok := data.(map[string]map[string]map[string]*jsonrpc.RPCTransaction)
	if !ok {
		return fmt.Errorf("unexpected response type for tx_pool: %T", data)
	}

	pendingData, ok := contentMap["pending"]
	if !ok {
		return nil
	}

	if pendingData != nil && len(pendingData) > 0 {
		// pending pool has transactions lets check last block if it was 0 tx
		var blockData map[string]interface{}
		fullTx := false
		blockData, err = ethApi.GetBlockByNumber(context.TODO(), -1, &fullTx)
		if err != nil {
			return err
		}

		var transactions []interface{}
		transactions, ok = blockData["transactions"].([]interface{})
		if !ok {
			return nil
		}

		if len(transactions) == 0 {
			return fmt.Errorf("found transactions in pending pool but last block has no transactions")
		}
	}

	return nil
}
