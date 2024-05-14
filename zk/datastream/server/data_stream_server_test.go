package server

// import (
// 	"math/big"
// 	"testing"

// 	"github.com/0xPolygonHermez/zkevm-data-streamer/datastreamer"
// 	"github.com/0xPolygonHermez/zkevm-data-streamer/log"
// 	"github.com/gateway-fm/cdk-erigon-lib/common"
// 	types "github.com/ledgerwatch/erigon/core/types"
// )

// func Test_unwindToBlock(t *testing.T) {
// 	testCases := []struct {
// 		name               string
// 		blocks             int64
// 		unwindTo           uint64
// 		expectedBlockCount uint64
// 	}{
// 		{
// 			name:               "unwind to block 0",
// 			blocks:             10,
// 			unwindTo:           0,
// 			expectedBlockCount: 0,
// 		},
// 		// {
// 		// 	name:               "unwind to block 5",
// 		// 	blocks:             10,
// 		// 	unwindTo:           5,
// 		// 	expectedBlockCount: 5,
// 		// },
// 		// {
// 		// 	name:               "unwind to block 10",
// 		// 	blocks:             10,
// 		// 	unwindTo:           10,
// 		// 	expectedBlockCount: 10,
// 		// },
// 	}

// 	for _, tc := range testCases {
// 		t.Run(tc.name, func(t *testing.T) {
// 			blocks := make([]types.Block, tc.blocks)
// 			for i := int64(0); i < tc.blocks; i++ {
// 				blocks[i] = *createBlockByNumber(i)
// 			}

// 			logConfig := log.Config{}
// 			dataStream, err := datastreamer.NewServer(uint16(1234), uint8(2), 1, datastreamer.StreamType(1), "file", &logConfig)
// 			if err != nil {
// 				t.Errorf("failed to create data stream server: %v", err)
// 			}
// 			srv := NewDataStreamServer(dataStream, 0, StandardOperationMode)
// 			if err := srv.stream.Start(); err != nil {
// 				t.Errorf("failed to start data stream server: %v", err)
// 			}

// 			if err = srv.stream.StartAtomicOp(); err != nil {
// 				t.Errorf("failed to start atomic op: %v", err)
// 			}

// 			entries := createBlockEntries(srv, &blocks)
// 			if err := srv.CommitEntriesToStream(entries, true); err != nil {
// 				t.Errorf("failed to commit entries to stream: %v", err)
// 			}

// 			if err := srv.UnwindToBlock(tc.unwindTo); err != nil {
// 				t.Errorf("failed to unwind to block %d: %v", tc.unwindTo, err)
// 			}

// 			header := srv.stream.GetHeader()
// 			defer srv.stream.RollbackAtomicOp()
// 			if header.TotalEntries/3 != tc.expectedBlockCount {
// 				t.Errorf("expected %d blocks, got %d", tc.expectedBlockCount, header.TotalEntries/3)
// 			}
// 		})
// 	}
// }

// func createBlockByNumber(blockNum int64) *types.Block {
// 	header := types.Header{Number: big.NewInt(blockNum)}
// 	block := types.NewBlockWithHeader(&header)

// 	return block
// }

// func createBlockEntries(srv *DataStreamServer, blocks *[]types.Block) []DataStreamEntry {
// 	entries := []DataStreamEntry{}
// 	effectiveGasPricePercentage := uint8(255)
// 	intermediateRoot := common.Hash{}
// 	l1BlockHash := common.Hash{}
// 	l1InfoIndex := 1
// 	deltaTimestamp := 100
// 	fork := 7
// 	ger := common.Hash{}

// 	for i, block := range *blocks {
// 		batchNumber := uint64(i)
// 		blockStart := srv.CreateBlockStartEntry(&block, batchNumber, uint16(fork), ger, uint32(deltaTimestamp), uint32(l1InfoIndex), l1BlockHash)
// 		entries = append(entries, blockStart)

// 		for _, tx := range block.Transactions() {
// 			transaction, err := srv.CreateTransactionEntry(effectiveGasPricePercentage, intermediateRoot, uint16(fork), tx)
// 			if err != nil {
// 				return nil
// 			}
// 			entries = append(entries, transaction)

// 		}

// 		blockEnd := srv.CreateBlockEndEntry(block.NumberU64(), block.Root(), block.Root())
// 		entries = append(entries, blockEnd)
// 	}

// 	return entries
// }
