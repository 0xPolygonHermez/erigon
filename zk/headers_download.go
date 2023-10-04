package zk

import (
	"fmt"
	"math/big"

	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/kv"
	ethTypes "github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/zk/datastream"
	"github.com/ledgerwatch/erigon/zk/datastream/types"
	"github.com/ledgerwatch/erigon/zk/erigon_db"
	"github.com/ledgerwatch/erigon/zk/hermez_db"
	txtype "github.com/ledgerwatch/erigon/zk/tx"
)

type erigonDb interface {
	WriteHeader(batchNo *big.Int, stateRoot, txHash common.Hash, coinbase common.Address, ts uint64) (*ethTypes.Header, error)
	WriteBody(batchNo *big.Int, headerHash common.Hash, txs []ethTypes.Transaction) error
}

func DownloadHeaders(tx kv.RwTx) (uint64, error) {
	eriDb := erigon_db.NewErigonDb(tx)
	hermezDb, err := hermez_db.NewHermezDb(tx)
	if err != nil {
		return 0, err
	}

	l2BlockChan := make(chan types.FullL2Block, 100000)
	entriesReadChan := make(chan uint64, 2)
	errChan := make(chan error, 2)

	// start client
	go func() {
		entriesRead, err := datastream.DownloadHeaders(datastream.TestDatastreamUrl, l2BlockChan)
		entriesReadChan <- entriesRead
		errChan <- err
	}()

	writeFinished := false
	stopLoop := false
	entriesRead := uint64(0)
	var routineErr error
	for {
		// get block
		var l2Block types.FullL2Block
		select {
		case a := <-l2BlockChan:
			l2Block = a
		case entriesReadFromChan := <-entriesReadChan:
			routineErr = <-errChan
			if routineErr != nil {
				return 0, routineErr
			}
			entriesRead = entriesReadFromChan
			writeFinished = true
		default:
			if writeFinished {
				stopLoop = true
			}
		}

		bn := new(big.Int).SetUint64(l2Block.BatchNumber)
		h, err := eriDb.WriteHeader(bn, l2Block.GlobalExitRoot, l2Block.L2Blockhash, l2Block.Coinbase, uint64(l2Block.Timestamp))
		if err != nil {
			return 0, fmt.Errorf("write header error: %v", err)
		}

		batchTxData := []byte{}
		for _, transaction := range l2Block.L2Txs {
			batchTxData = append(batchTxData, transaction.Encoded...)
		}

		txs, _, _, err := txtype.DecodeTxs(batchTxData, uint64(l2Block.ForkId))

		if err := eriDb.WriteBody(bn, h.Hash(), txs); err != nil {
			return 0, fmt.Errorf("write body error: %v", err)
		}

		if err := hermezDb.WriteForkId(l2Block.BatchNumber, uint64(l2Block.ForkId)); err != nil {
			return 0, fmt.Errorf("write block batch error: %v", err)
		}

		if stopLoop {
			break
		}

		if err := hermezDb.WriteBlockBatch(l2Block.L2BlockNumber, l2Block.BatchNumber); err != nil {
			return 0, fmt.Errorf("write block batch error: %v", err)
		}
	}

	return entriesRead, nil
}
