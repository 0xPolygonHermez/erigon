package mdbxdatabrowser

import (
	"fmt"
	"math/big"
	"testing"

	libcommon "github.com/gateway-fm/cdk-erigon-lib/common"
	"github.com/gateway-fm/cdk-erigon-lib/kv/memdb"
	"github.com/stretchr/testify/require"

	"github.com/ledgerwatch/erigon/common/u256"
	"github.com/ledgerwatch/erigon/core/rawdb"
	"github.com/ledgerwatch/erigon/core/types"
)
func TestDbDataRetriever_GetBlockByNumber(t *testing.T) {
	t.Run("querying an existing block", func(t *testing.T) {
		// arrange
		_, tx := memdb.NewTestTx(t)

		tx1 := types.NewTransaction(1, libcommon.HexToAddress("0x1050"), u256.Num1, 1, u256.Num1, nil)
		tx2 := types.NewTransaction(2, libcommon.HexToAddress("0x100"), u256.Num27, 2, u256.Num2, nil)

		block := types.NewBlockWithHeader(
			&types.Header{
				Number:      big.NewInt(5),
				Extra:       []byte("some random data"),
				UncleHash:   types.EmptyUncleHash,
				TxHash:      types.EmptyRootHash,
				ReceiptHash: types.EmptyRootHash,
			})
		block = block.WithBody(types.Transactions{tx1, tx2}, nil)

		require.NoError(t, rawdb.WriteCanonicalHash(tx, block.Hash(), block.NumberU64()))
		require.NoError(t, rawdb.WriteBlock(tx, block))

		// act and assert
		dbReader := NewDbDataRetriever(tx)
		result, err := dbReader.GetBlockByNumber(block.NumberU64(), true, true)
		require.NoError(t, err)
		require.Equal(t, block.Hash(), result.Hash)
		require.Equal(t, block.Number().Uint64(), uint64(result.Number))
	})

	t.Run("querying non-existent block", func(t *testing.T) {
		blockNum := uint64(10)

		_, tx := memdb.NewTestTx(t)
		dbReader := NewDbDataRetriever(tx)
		result, err := dbReader.GetBlockByNumber(blockNum, true, true)
		require.ErrorContains(t, err, fmt.Sprintf("block %d not found", blockNum))
		require.Nil(t, result)
	})
}
