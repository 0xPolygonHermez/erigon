package jsonrpc

import (
	"context"
	"github.com/ledgerwatch/erigon-lib/common/datadir"
	"github.com/ledgerwatch/erigon-lib/kv/kvcache"
	"github.com/ledgerwatch/erigon/accounts/abi/bind/backends"
	"github.com/ledgerwatch/erigon/eth/ethconfig"
	"github.com/ledgerwatch/erigon/eth/filters"
	"github.com/ledgerwatch/erigon/rpc/rpccfg"
	"github.com/ledgerwatch/log/v3"
	"math/big"
	"testing"
)

func TestGetLogsWithRange(t *testing.T) {
	// set range
	logsMaxRange := uint64(1000)

	contractBackend := backends.NewTestSimulatedBackendWithConfig(t, gspec.Alloc, gspec.Config, gspec.GasLimit)
	defer contractBackend.Close()
	stateCache := kvcache.New(kvcache.DefaultCoherentConfig)
	contractBackend.Commit()

	db := contractBackend.DB()
	agg := contractBackend.Agg()
	baseApi := NewBaseApi(nil, stateCache, contractBackend.BlockReader(), agg, false, rpccfg.DefaultEvmCallTimeout, contractBackend.Engine(), datadir.New(t.TempDir()))
	ethImpl := NewEthAPI(baseApi, db, nil, nil, nil, 5000000, 100_000, 100_000, &ethconfig.Defaults, false, 100, 100, log.New(), defaultL1GasPriceTracker, logsMaxRange)

	scenarios := []struct {
		name          string
		toBlock       uint64
		expectedError bool
	}{
		{
			name:          "GetLogs from 0 to 100",
			toBlock:       100,
			expectedError: false,
		},
		{
			name:          "GetLogs from 0 to 1100",
			toBlock:       1100,
			expectedError: true,
		},
		{
			name:          "GetLogs from 0 to 1000",
			toBlock:       1000,
			expectedError: false,
		},
		{
			name:          "GetLogs from 0 to 1001",
			toBlock:       1001,
			expectedError: true,
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			crit := filters.FilterCriteria{
				FromBlock: new(big.Int).SetUint64(0),
				ToBlock:   new(big.Int).SetUint64(scenario.toBlock),
			}
			_, err := ethImpl.GetLogs(context.Background(), crit)
			if err != nil {
				if !scenario.expectedError {
					t.Errorf("calling GetLogs: %v", err)
				}
				if err.Error() != "block range too large" {
					t.Errorf("expected error: block range too large, got: %v", err)
				}
			} else {
				if scenario.expectedError {
					t.Errorf("expected error but got none")
				}
			}
		})
	}
}
