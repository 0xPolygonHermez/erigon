package server

import (
	"github.com/gateway-fm/cdk-erigon-lib/kv"
	"github.com/ledgerwatch/erigon/zk/hermez_db"
	"github.com/0xPolygonHermez/zkevm-data-streamer/datastreamer"
	"time"
	eritypes "github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/core/rawdb"
	"fmt"
	"github.com/ledgerwatch/log/v3"
	"github.com/gateway-fm/cdk-erigon-lib/common"
)

func WriteBlocksToStream(
	tx kv.Tx,
	reader *hermez_db.HermezDbReader,
	srv *DataStreamServer,
	stream *datastreamer.StreamServer,
	from, to uint64,
	logPrefix string,
) error {
	logTicker := time.NewTicker(10 * time.Second)
	var lastBlock *eritypes.Block
	var err error
	if err = stream.StartAtomicOp(); err != nil {
		return err
	}
	totalToWrite := to - (from - 1)
	insertEntryCount := 1000000
	entries := make([]DataStreamEntryProto, insertEntryCount)
	index := 0
	copyFrom := from
	for currentBlockNumber := from; currentBlockNumber <= to; currentBlockNumber++ {
		select {
		case <-logTicker.C:
			log.Info(fmt.Sprintf("[%s]: progress", logPrefix),
				"block", currentBlockNumber,
				"target", to, "%", float64(currentBlockNumber-copyFrom)/float64(totalToWrite)*100)
		default:
		}

		if lastBlock == nil {
			lastBlock, err = rawdb.ReadBlockByNumber(tx, currentBlockNumber-1)
			if err != nil {
				return err
			}
		}

		block, err := rawdb.ReadBlockByNumber(tx, currentBlockNumber)
		if err != nil {
			return err
		}

		batchNum, err := reader.GetBatchNoByL2Block(currentBlockNumber)
		if err != nil {
			return err
		}

		prevBatchNum, err := reader.GetBatchNoByL2Block(currentBlockNumber - 1)
		if err != nil {
			return err
		}

		// todo - this needs handling for older blocks in the stream
		//gersInBetween, err := reader.GetBatchGlobalExitRoots(prevBatchNum, batchNum)
		//if err != nil {
		//	return err
		//}

		highestBlockInBatch, err := reader.GetHighestBlockInBatch(batchNum)
		if err != nil {
			return err
		}

		batchEnd := highestBlockInBatch == block.NumberU64()

		l1InfoMinTimestamps := make(map[uint64]uint64)

		blockEntries, err := srv.CreateStreamEntriesProto(block, reader, lastBlock, batchNum, prevBatchNum, l1InfoMinTimestamps, batchEnd)
		if err != nil {
			return err
		}

		for _, entry := range *blockEntries {
			entries[index] = entry
			index++
		}

		// basically commit onece 80% of the entries array is filled
		if index+1 >= insertEntryCount*4/5 {
			log.Info(fmt.Sprintf("[%s] Commit count reached, committing entries", logPrefix), "block", currentBlockNumber)
			if err = srv.CommitEntriesToStreamProto(entries[:index]); err != nil {
				return err
			}
			entries = make([]DataStreamEntryProto, insertEntryCount)
			index = 0
		}

		lastBlock = block
	}

	if err = srv.CommitEntriesToStreamProto(entries[:index]); err != nil {
		return err
	}

	if err = stream.CommitAtomicOp(); err != nil {
		return err
	}

	return nil
}

func WriteGenesisToStream(
	genesis *eritypes.Block,
	reader *hermez_db.HermezDbReader,
	stream *datastreamer.StreamServer,
	srv *DataStreamServer,
	streamVersion int,
	chainId uint64,
) error {

	batchNo, err := reader.GetBatchNoByL2Block(0)
	if err != nil {
		return err
	}

	ger, err := reader.GetBlockGlobalExitRoot(genesis.NumberU64())
	if err != nil {
		return err
	}

	forkId, err := reader.GetForkId(batchNo)
	if err != nil {
		return err
	}

	err = stream.StartAtomicOp()
	if err != nil {
		return err
	}

	batchBookmark := srv.CreateBatchBookmarkEntryProto(genesis.NumberU64())
	l2BlockBookmark := srv.CreateL2BlockBookmarkEntryProto(genesis.NumberU64())
	l2Block := srv.CreateL2BlockProto(genesis, batchNo, ger, 0, 0, common.Hash{}, 0)
	batchStart, err := srv.CreateBatchStartProto(batchNo, chainId, forkId)
	if err != nil {
		return err
	}

	batchEnd, err := srv.CreateBatchEndProto(common.Hash{}, genesis.Root())

	if err = srv.CommitEntriesToStreamProto([]DataStreamEntryProto{batchBookmark, batchStart, l2BlockBookmark, l2Block, batchEnd}); err != nil {
		return err
	}

	err = stream.CommitAtomicOp()
	if err != nil {
		return err
	}

	return nil
}
