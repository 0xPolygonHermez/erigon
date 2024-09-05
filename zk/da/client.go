package da

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gateway-fm/cdk-erigon-lib/common"
	"github.com/ledgerwatch/erigon/common/hexutil"
	"github.com/ledgerwatch/log/v3"

	"github.com/ledgerwatch/erigon/zkevm/jsonrpc/client"
)

const retryDelay = 500 * time.Millisecond

func GetOffChainData(ctx context.Context, url string, hash common.Hash) ([]byte, error) {
	for {
		response, err := client.JSONRPCCall(url, "sync_getOffChainData", hash)

		if err != nil || response.Error != nil {
			log.Error(fmt.Sprintf("GetOffChainData hash:%v, error:%v, response err:%v", hash.String(), err, response.Error))
			time.Sleep(retryDelay)
			continue
		}

		select {
		case <-ctx.Done():
			errMsg := fmt.Sprintf("GetOffChainData hash:%v, context done", hash.String())
			log.Error(errMsg)
			return nil, fmt.Errorf(errMsg)
		default:
		}

		return hexutil.Decode(strings.Trim(string(response.Result), "\""))
	}
}
