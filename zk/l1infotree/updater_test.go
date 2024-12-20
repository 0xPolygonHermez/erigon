package l1infotree_test

import (
	"context"
	"log"
	"math/big"
	"testing"
	"time"

	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/kv/mdbx"
	"github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/eth/ethconfig"
	"github.com/ledgerwatch/erigon/zk/hermez_db"
	"github.com/ledgerwatch/erigon/zk/l1infotree"
	"github.com/ledgerwatch/erigon/zk/syncer"
	"github.com/ledgerwatch/erigon/zk/syncer/mocks"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// to be removed
func GetDbTx() (tx kv.RwTx, cleanup func()) {
	dbi, err := mdbx.NewTemporaryMdbx(context.Background(), "")
	if err != nil {
		panic(err)
	}
	tx, err = dbi.BeginRw(context.Background())
	if err != nil {
		panic(err)
	}

	err = hermez_db.CreateHermezBuckets(tx)
	if err != nil {
		panic(err)
	}

	return tx, func() {
		tx.Rollback()
		dbi.Close()
	}
}

func TestNewHermezDb(t *testing.T) {
	tx, cleanup := GetDbTx()
	defer cleanup()
	db := hermez_db.NewHermezDb(tx)
	assert.NotNil(t, db)
}

func TestNewUpdater(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := &ethconfig.Zk{}

	l1InfoTreeSyncer := syncer.NewL1Syncer(
		ctx,
		nil, nil, nil,
		cfg.L1BlockRange,
		cfg.L1QueryDelay,
		cfg.L1HighestBlockType,
	)

	updater := l1infotree.NewUpdater(cfg, l1InfoTreeSyncer)
	assert.NotNil(t, updater)
}

func TestUpdater_WarmUp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	EthermanMock := mocks.NewMockIEtherman(mockCtrl)

	cfg := &ethconfig.Zk{
		L1FirstBlock: 1,
	}

	finalizedHeader := &types.Header{Number: big.NewInt(10), Difficulty: big.NewInt(100)}
	finalizedBlock := types.NewBlockWithHeader(finalizedHeader)

	// this will be last block answer
	EthermanMock.EXPECT().
		BlockByNumber(gomock.Any(), (*big.Int)(nil)).
		Return(finalizedBlock, nil).AnyTimes()

	// // filterLogs
	EthermanMock.EXPECT().
		FilterLogs(gomock.Any(), gomock.Any()).Return(
		[]types.Log{
			{},
			{},
		}, nil).AnyTimes()

	l1InfoTreeSyncer := syncer.NewL1Syncer(
		ctx,
		[]syncer.IEtherman{EthermanMock},
		[]common.Address{},
		[][]common.Hash{},
		cfg.L1BlockRange,
		cfg.L1QueryDelay,
		cfg.L1HighestBlockType,
	)

	updater := l1infotree.NewUpdater(cfg, l1InfoTreeSyncer)

	tx, cleanup := GetDbTx()
	defer cleanup()

	err := updater.WarmUp(tx)

	assert.Equal(t, uint64(0), updater.GetProgress())
	// log error
	log.Println(err)
	assert.NoError(t, err)
}

func TestUpdater_GetProgress(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := &ethconfig.Zk{}

	l1InfoTreeSyncer := syncer.NewL1Syncer(
		ctx,
		nil, nil, nil,
		cfg.L1BlockRange,
		cfg.L1QueryDelay,
		cfg.L1HighestBlockType,
	)

	updater := l1infotree.NewUpdater(cfg, l1InfoTreeSyncer)
	assert.NotNil(t, updater)

	progress := updater.GetProgress()
	assert.Equal(t, uint64(0), progress)
}

func TestUpdater_GetLatestUpdate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := &ethconfig.Zk{}

	l1InfoTreeSyncer := syncer.NewL1Syncer(
		ctx,
		nil, nil, nil,
		cfg.L1BlockRange,
		cfg.L1QueryDelay,
		cfg.L1HighestBlockType,
	)

	updater := l1infotree.NewUpdater(cfg, l1InfoTreeSyncer)
	assert.NotNil(t, updater)

	latestUpdate := updater.GetLatestUpdate()
	assert.Nil(t, latestUpdate)
}

func TestUpdater_CheckForInfoTreeUpdates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mockCtrl := gomock.NewController(t)
	defer mockCtrl.Finish()
	EthermanMock := mocks.NewMockIEtherman(mockCtrl)

	mockHeader := &types.Header{
		Number:     big.NewInt(1),
		Time:       uint64(time.Now().Unix()),
		ParentHash: common.HexToHash("0x0"),
	}

	expectedBlock := types.NewBlockWithHeader(mockHeader)

	EthermanMock.EXPECT().
		BlockByNumber(gomock.Any(), (*big.Int)(nil)).
		Return(expectedBlock, nil).
		AnyTimes()

	EthermanMock.EXPECT().
		HeaderByNumber(gomock.Any(), gomock.Any()).
		Return(mockHeader, nil).
		AnyTimes()

	cfg := &ethconfig.Zk{
		L1BlockRange: 2,
	}

	l1InfoTreeSyncer := syncer.NewL1Syncer(
		ctx,
		[]syncer.IEtherman{EthermanMock},
		[]common.Address{},
		[][]common.Hash{},
		cfg.L1BlockRange,
		cfg.L1QueryDelay,
		cfg.L1HighestBlockType,
	)

	updater := l1infotree.NewUpdater(cfg, l1InfoTreeSyncer)

	tx, cleanup := GetDbTx()
	defer cleanup()

	err := updater.WarmUp(tx)
	assert.NoError(t, err)

	_, err = updater.CheckForInfoTreeUpdates(
		"TestUpdater_CheckForInfoTreeUpdates",
		tx,
	)
	assert.NoError(t, err)
	// TODO: add more setup on mock/syncer to get a certain number of logs here.
	// assert.NotNil(t, allLogs)
}
