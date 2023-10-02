package erigon_db

import (
	"fmt"
	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon/core/rawdb"
	ethTypes "github.com/ledgerwatch/erigon/core/types"
	"math/big"
	"time"
)

type ErigonDb struct {
	tx kv.RwTx
}

func NewErigonDb(tx kv.RwTx) *ErigonDb {
	return &ErigonDb{
		tx: tx,
	}
}

func (db ErigonDb) WriteHeader(batchNo *big.Int, stateRoot, txHash common.Hash, coinbase common.Address, ts time.Time) (*ethTypes.Header, error) {
	h := &ethTypes.Header{
		Root:       stateRoot,
		TxHash:     txHash,
		Difficulty: big.NewInt(0),
		Number:     batchNo,
		GasLimit:   30_000_000,
		Coinbase:   coinbase,
		Time:       uint64(ts.Unix()),
	}
	rawdb.WriteHeader(db.tx, h)
	err := rawdb.WriteCanonicalHash(db.tx, h.Hash(), batchNo.Uint64())
	if err != nil {
		return nil, fmt.Errorf("failed to write canonical hash: %w", err)
	}
	return h, nil
}

func (db ErigonDb) WriteBody(batchNo *big.Int, headerHash common.Hash, txs []ethTypes.Transaction) error {
	b := &ethTypes.Body{
		Transactions: txs,
	}

	// writes txs to EthTx (canonical table)
	return rawdb.WriteBody(db.tx, headerHash, batchNo.Uint64(), b)
}
