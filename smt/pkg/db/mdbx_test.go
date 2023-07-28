package db

import (
	"context"
	"github.com/ledgerwatch/erigon-lib/kv/mdbx"
	"github.com/ledgerwatch/erigon/smt/pkg/utils"
	"github.com/stretchr/testify/assert"
	"math/big"
	"testing"
)

func TestEriDb(t *testing.T) {
	dbi, err := mdbx.NewTemporaryMdbx()
	tx, err := dbi.BeginRw(context.Background())
	assert.NoError(t, err)
	defer tx.Rollback()
	_ = tx.CreateBucket("HermezSmt")

	db := NewEriDb(tx)

	// The key and value we're going to test
	key := utils.NodeKey{1, 2, 3, 4}
	value := utils.NodeValue12{big.NewInt(1), big.NewInt(2), big.NewInt(3), big.NewInt(4), big.NewInt(5), big.NewInt(6),
		big.NewInt(7), big.NewInt(8), big.NewInt(9), big.NewInt(10), big.NewInt(11), big.NewInt(12)}

	// Testing Insert method
	err = db.Insert(key, value)
	assert.NoError(t, err)

	// Testing Get method
	retrievedValue, err := db.Get(key)
	assert.NoError(t, err)
	assert.Equal(t, value, retrievedValue)
}
