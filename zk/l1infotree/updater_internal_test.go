package l1infotree

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/kv/mdbx"
	"github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/zk/hermez_db"
	"github.com/stretchr/testify/assert"
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

func TestUpdater_chunkLogs(t *testing.T) {
	logs := []types.Log{
		{
			Address: common.Address{},
			Topics:  []common.Hash{common.HexToHash("0x01")},
			Data:    []byte{0x01},
		},
		{
			Address: common.Address{},
			Topics:  []common.Hash{common.HexToHash("0x02")},
			Data:    []byte{0x02},
		},
		{
			Address: common.Address{},
			Topics:  []common.Hash{common.HexToHash("0x03")},
			Data:    []byte{0x03},
		},
		{
			Address: common.Address{},
			Topics:  []common.Hash{common.HexToHash("0x04")},
			Data:    []byte{0x04},
		},
	}

	chunkedLogs := chunkLogs(logs, 2)
	assert.Len(t, chunkedLogs, 2)
	assert.Len(t, chunkedLogs[0], 2)
	assert.Len(t, chunkedLogs[1], 2)
}

func TestUpdater_initialiseL1InfoTree(t *testing.T) {
	tx, cleanup := GetDbTx()
	defer cleanup()
	db := hermez_db.NewHermezDb(tx)
	assert.NotNil(t, db)

	// updater := NewUpdater(db)
	// func initialiseL1InfoTree(hermezDb *hermez_db.HermezDb) (*L1InfoTree, error) {
	l1infotree, err := InitialiseL1InfoTree(db)
	assert.NoError(t, err)
	assert.NotNil(t, l1infotree)

}
func TestUpdater_createL1InfoTreeUpdate(t *testing.T) {
	tx, cleanup := GetDbTx()
	defer cleanup()
	db := hermez_db.NewHermezDb(tx)
	assert.NotNil(t, db)

	// func createL1InfoTreeUpdate(hermezDb *hermez_db.HermezDb, l1InfoTree *L1InfoTree) (*zkTypes.L1InfoTreeUpdate, error) {
	l1infotree, err := InitialiseL1InfoTree(db)
	assert.NoError(t, err)
	assert.NotNil(t, l1infotree)

	// Prepare a valid log with 3 topics
	log := types.Log{
		BlockNumber: 1,
		Topics:      []common.Hash{common.HexToHash("0x01"), common.HexToHash("0x02"), common.HexToHash("0x03")},
	}
	header := &types.Header{Number: big.NewInt(1), Time: uint64(time.Now().Unix()), ParentHash: common.HexToHash("0x0")}

	// func createL1InfoTreeUpdate(l types.Log, header *types.Header) (*zkTypes.L1InfoTreeUpdate, error) {
	l1infotreeupdate, err := createL1InfoTreeUpdate(log, header)
	assert.NoError(t, err)
	assert.NotNil(t, l1infotreeupdate)
	assert.Equal(t, log.Topics[1], l1infotreeupdate.MainnetExitRoot)
	assert.Equal(t, log.Topics[2], l1infotreeupdate.RollupExitRoot)
	assert.Equal(t, log.BlockNumber, l1infotreeupdate.BlockNumber)
	assert.Equal(t, header.Time, l1infotreeupdate.Timestamp)
	assert.Equal(t, header.ParentHash, l1infotreeupdate.ParentHash)

	// Prepare a valid log with less than 3 topics
	lessThen3Topics := types.Log{
		BlockNumber: 1,
		Topics:      []common.Hash{},
	}

	// func createL1InfoTreeUpdate(l types.Log, header *types.Header) (*zkTypes.L1InfoTreeUpdate, error) {
	l1infotreeupdate2, err2 := createL1InfoTreeUpdate(lessThen3Topics, header)
	assert.Error(t, err2)
	assert.Nil(t, l1infotreeupdate2)

	unmatchingHeader := &types.Header{Number: big.NewInt(2), Time: uint64(time.Now().Unix()), ParentHash: common.HexToHash("0x0")}

	// func createL1InfoTreeUpdate(l types.Log, header *types.Header) (*zkTypes.L1InfoTreeUpdate, error) {
	l1infotreeupdate3, err3 := createL1InfoTreeUpdate(lessThen3Topics, unmatchingHeader)
	assert.Error(t, err3)
	assert.Nil(t, l1infotreeupdate3)
}

func TestUpdater_handleL1InfoTreeUpdate(t *testing.T) {
	tx, cleanup := GetDbTx()
	defer cleanup()
	db := hermez_db.NewHermezDb(tx)
	assert.NotNil(t, db)

	// Prepare a valid log with 3 topics
	log := types.Log{
		BlockNumber: 1,
		Topics:      []common.Hash{common.HexToHash("0x01"), common.HexToHash("0x02"), common.HexToHash("0x03")},
	}
	header := &types.Header{Number: big.NewInt(1), Time: uint64(time.Now().Unix()), ParentHash: common.HexToHash("0x0")}

	// func createL1InfoTreeUpdate(l types.Log, header *types.Header) (*zkTypes.L1InfoTreeUpdate, error) {
	l1infotreeupdate, err := createL1InfoTreeUpdate(log, header)
	assert.NoError(t, err)

	// func handleL1InfoTreeUpdate(hermezDb *hermez_db.HermezDb, update *zkTypes.L1InfoTreeUpdate) error {
	err = handleL1InfoTreeUpdate(db, l1infotreeupdate)
	assert.Nil(t, err)
	assert.NoError(t, err)

}
