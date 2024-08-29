package commands

import (
	"context"
	"math/big"
	"testing"

	"github.com/gateway-fm/cdk-erigon-lib/common/datadir"
	"github.com/gateway-fm/cdk-erigon-lib/kv/kvcache"
	"github.com/ledgerwatch/erigon/accounts/abi/bind/backends"
	"github.com/ledgerwatch/erigon/common/hexutil"
	"github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/crypto"
	"github.com/ledgerwatch/erigon/eth/ethconfig"
	"github.com/ledgerwatch/erigon/eth/stagedsync/stages"
	"github.com/ledgerwatch/erigon/params"
	"github.com/ledgerwatch/erigon/rpc"
	"github.com/ledgerwatch/erigon/rpc/rpccfg"
	"github.com/ledgerwatch/erigon/zk/hermez_db"
	"github.com/ledgerwatch/erigon/zk/syncer"
	"github.com/stretchr/testify/assert"
)

var (
	key, _   = crypto.HexToECDSA("b71c71a67e1177ad4e901695e1b4b9ee17ae16c6668d313eac2f96dbcda3f291")
	key1, _  = crypto.HexToECDSA("49a7b37aa6f6645917e7b807e9d1c00d4fa71f18343b0d4122a4d2df64dd6fee")
	key2, _  = crypto.HexToECDSA("8a1f9a8f95be41cd7ccb6168179afb4504aefe388d1e14474d32c45c72ce7b7a")
	address  = crypto.PubkeyToAddress(key.PublicKey)
	address1 = crypto.PubkeyToAddress(key1.PublicKey)
	address2 = crypto.PubkeyToAddress(key2.PublicKey)
	gspec    = &types.Genesis{
		Config: params.TestChainConfig,
		Alloc: types.GenesisAlloc{
			address:  {Balance: big.NewInt(9000000000000000000)},
			address1: {Balance: big.NewInt(200000000000000000)},
			address2: {Balance: big.NewInt(300000000000000000)},
		},
		GasLimit: 10000000,
	}
	chainID = big.NewInt(1337)
	ctx     = context.Background()

	addr1BalanceCheck = "70a08231" + "000000000000000000000000" + address1.Hex()[2:]
	addr2BalanceCheck = "70a08231" + "000000000000000000000000" + address2.Hex()[2:]
	transferAddr2     = "70a08231" + "000000000000000000000000" + address1.Hex()[2:] + "0000000000000000000000000000000000000000000000000000000000000064"
)

func TestLatestConsolidatedBlockNumber(t *testing.T) {
	assert := assert.New(t)
	////////////////
	contractBackend := backends.NewTestSimulatedBackendWithConfig(t, gspec.Alloc, gspec.Config, gspec.GasLimit)
	defer contractBackend.Close()
	stateCache := kvcache.New(kvcache.DefaultCoherentConfig)
	contractBackend.Commit()
	///////////

	db := contractBackend.DB()
	agg := contractBackend.Agg()

	baseApi := NewBaseApi(nil, stateCache, contractBackend.BlockReader(), agg, false, rpccfg.DefaultEvmCallTimeout, contractBackend.Engine(), datadir.New(t.TempDir()))
	ethImpl := NewEthAPI(baseApi, db, nil, nil, nil, 5000000, 100_000, &ethconfig.Defaults)
	var l1Syncer *syncer.L1Syncer
	zkEvmImpl := NewZkEvmAPI(ethImpl, db, 100_000, &ethconfig.Defaults, l1Syncer, "")
	tx, err := db.BeginRw(ctx)
	assert.NoError(err)
	hDB := hermez_db.NewHermezDb(tx)
	for i := 1; i <= 10; i++ {
		hDB.WriteBlockBatch(uint64(i), 1)
	}
	if err := stages.SaveStageProgress(tx, stages.L1VerificationsBatchNo, 1); err != nil {
		t.Errorf("failed to save stage progress, %v", err)
	}
	tx.Commit()
	blockNumber, err := zkEvmImpl.ConsolidatedBlockNumber(ctx)
	if err != nil {
		t.Errorf("calling ConsolidatedBlockNumber resulted in an error: %v", err)
	}
	t.Log("blockNumber: ", blockNumber)

	var expectedL2BlockNumber hexutil.Uint64 = 10
	assert.Equal(expectedL2BlockNumber, blockNumber)
}

func TestIsBlockConsolidated(t *testing.T) {
	assert := assert.New(t)
	////////////////
	contractBackend := backends.NewTestSimulatedBackendWithConfig(t, gspec.Alloc, gspec.Config, gspec.GasLimit)
	defer contractBackend.Close()
	stateCache := kvcache.New(kvcache.DefaultCoherentConfig)
	contractBackend.Commit()
	///////////

	db := contractBackend.DB()
	agg := contractBackend.Agg()

	baseApi := NewBaseApi(nil, stateCache, contractBackend.BlockReader(), agg, false, rpccfg.DefaultEvmCallTimeout, contractBackend.Engine(), datadir.New(t.TempDir()))
	ethImpl := NewEthAPI(baseApi, db, nil, nil, nil, 5000000, 100_000, &ethconfig.Defaults)
	var l1Syncer *syncer.L1Syncer
	zkEvmImpl := NewZkEvmAPI(ethImpl, db, 100_000, &ethconfig.Defaults, l1Syncer, "")
	isConsolidated, err := zkEvmImpl.IsBlockConsolidated(ctx, 11000000000)
	if err != nil {
		t.Errorf("calling IsBlockConsolidated resulted in an error: %v", err)
	}
	t.Logf("blockNumber: 11 -> %v", isConsolidated)
	assert.False(isConsolidated)
	tx, err := db.BeginRw(ctx)
	assert.NoError(err)
	hDB := hermez_db.NewHermezDb(tx)
	for i := 1; i <= 10; i++ {
		hDB.WriteBlockBatch(uint64(i), 1)
	}
	if err := stages.SaveStageProgress(tx, stages.L1VerificationsBatchNo, 1); err != nil {
		t.Errorf("failed to save stage progress, %v", err)
	}
	tx.Commit()
	for i := 1; i <= 10; i++ {
		isConsolidated, err := zkEvmImpl.IsBlockConsolidated(ctx, rpc.BlockNumber(i))
		if err != nil {
			t.Errorf("calling IsBlockConsolidated resulted in an error: %v", err)
		}
		t.Logf("blockNumber: %d -> %v", i, isConsolidated)
		assert.True(isConsolidated)
	}
}
