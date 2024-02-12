package stages

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/0xPolygonHermez/zkevm-data-streamer/datastreamer"
	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon/core/rawdb"
	eritypes "github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/eth/stagedsync"
	"github.com/ledgerwatch/erigon/eth/stagedsync/stages"
	"github.com/ledgerwatch/erigon/zk/datastream/server"
	"github.com/ledgerwatch/erigon/zk/hermez_db"
	"github.com/ledgerwatch/log/v3"
)

type DataStreamCatchupCfg struct {
	db      kv.RwDB
	stream  *datastreamer.StreamServer
	chainId uint64
}

func StageDataStreamCatchupCfg(stream *datastreamer.StreamServer, db kv.RwDB, chainId uint64) DataStreamCatchupCfg {
	return DataStreamCatchupCfg{
		stream:  stream,
		db:      db,
		chainId: chainId,
	}
}

func SpawnStageDataStreamCatchup(
	s *stagedsync.StageState,
	ctx context.Context,
	tx kv.RwTx,
	cfg DataStreamCatchupCfg,
) error {

	logPrefix := s.LogPrefix()
	log.Info(fmt.Sprintf("[%s]: Starting...", logPrefix))
	stream := cfg.stream

	if stream == nil {
		// skip the stage if there is no streamer provided
		log.Info(fmt.Sprintf("[%s]: no streamer provided, skipping stage", logPrefix))
		return nil
	}

	createdTx := false
	if tx == nil {
		log.Debug(fmt.Sprintf("[%s] data stream: no tx provided, creating a new one", logPrefix))
		var err error
		tx, err = cfg.db.BeginRw(ctx)
		if err != nil {
			return fmt.Errorf("failed to open tx, %w", err)
		}
		defer tx.Rollback()
		createdTx = true
	}

	srv := server.NewDataStreamServer(stream, cfg.chainId)
	reader := hermez_db.NewHermezDbReader(tx)

	// get the latest block so when to terminate the loop.  This is because not all batches contain blocks
	// so we cannot use this reliably to break the loop.  Block number is more reliable
	highestSeenBatchNumber, err := stages.GetStageProgress(tx, stages.HighestSeenBatchNumber)
	if err != nil {
		return err
	}

	currentBatch, err := stages.GetStageProgress(tx, stages.DataStream)
	if err != nil {
		return err
	}

	var lastBlock *eritypes.Block

	// skip genesis if we have no data in the stream yet
	if currentBatch == 0 {
		currentBatch++
	}

	batchToBlocks, err := preLoadBatchedToBlocks(tx)
	if err != nil {
		return err
	}

	logTicker := time.NewTicker(10 * time.Second)

	if err = stream.StartAtomicOp(); err != nil {
		return err
	}

	for ; currentBatch <= highestSeenBatchNumber; currentBatch++ {
		select {
		case <-logTicker.C:
			log.Info(fmt.Sprintf("[%s]: progress", logPrefix),
				"batch", currentBatch,
				"target", highestSeenBatchNumber, "%", math.Round(float64(currentBatch)/float64(highestSeenBatchNumber)*100))
		default:
		}

		// get the blocks for this batch
		blockNumbers, ok := batchToBlocks[currentBatch]

		// if there are no blocks to process just continue - previously this would check for a GER update in
		// the pre-etrog world but this isn't possible now because of the l1 info tree indexes, so we just
		// skip on
		if !ok || len(blockNumbers) == 0 {
			log.Info(fmt.Sprintf("[%s] found a batch with no blocks during data stream catchup", logPrefix), "batch", currentBatch)
			currentBatch++
			continue
		}

		for _, blockNumber := range blockNumbers {
			if lastBlock == nil {
				lastBlock, err = rawdb.ReadBlockByNumber(tx, blockNumber-1)
				if err != nil {
					return err
				}
			}
			block, err := rawdb.ReadBlockByNumber(tx, blockNumber)
			if err != nil {
				return err
			}
			err = writeBlockToStream(block, reader, srv, lastBlock, currentBatch)
			if err != nil {
				return err
			}
			lastBlock = block
		}
	}

	if err = stream.CommitAtomicOp(); err != nil {
		return err
	}

	if err = stages.SaveStageProgress(tx, stages.DataStream, currentBatch); err != nil {
		return err
	}

	if createdTx {
		err = tx.Commit()
		if err != nil {
			log.Error(fmt.Sprintf("[%s] error: %s", logPrefix, err))
		}
	}

	log.Info(fmt.Sprintf("[%s]: stage complete", logPrefix),
		"batch", currentBatch-1,
		"target", highestSeenBatchNumber, "%", math.Round(float64(currentBatch-1)/float64(highestSeenBatchNumber)*100))

	return err
}

func preLoadBatchedToBlocks(tx kv.RwTx) (map[uint64][]uint64, error) {
	// hold the mapping of block batches to block numbers - this is an expensive call so just
	// do it once
	// todo: can we not use memory here, could be a problem with a larger chain?
	batchToBlocks := make(map[uint64][]uint64)
	c, err := tx.Cursor(hermez_db.BLOCKBATCHES)
	if err != nil {
		return nil, err
	}
	for k, v, err := c.First(); k != nil; k, v, err = c.Next() {
		if err != nil {
			return nil, err
		}
		block := hermez_db.BytesToUint64(k)
		batch := hermez_db.BytesToUint64(v)
		_, ok := batchToBlocks[batch]
		if !ok {
			batchToBlocks[batch] = []uint64{block}
		} else {
			batchToBlocks[batch] = append(batchToBlocks[batch], block)
		}
	}
	return batchToBlocks, nil
}

func writeBlockToStream(
	block *eritypes.Block,
	reader *hermez_db.HermezDbReader,
	srv *server.DataStreamServer,
	lastBlock *eritypes.Block,
	currentBatch uint64,
) error {
	fork, err := reader.GetForkId(currentBatch)
	if err != nil {
		return err
	}

	err = srv.AddBookmark(server.BlockBookmarkType, block.NumberU64())
	if err != nil {
		return err
	}

	deltaTimestamp := block.Time() - lastBlock.Time()

	var ger common.Hash
	var l1BlockHash common.Hash

	l1Index, err := reader.GetBlockL1InfoTreeIndex(block.NumberU64())
	if err != nil {
		return err
	}

	if block.NumberU64() == 1 {
		// injected batch at the start of the network
		injected, err := reader.GetL1InjectedBatch(0)
		if err != nil {
			return err
		}
		ger = injected.LastGlobalExitRoot
		l1BlockHash = injected.L1ParentHash

		// block 1 in the stream has a delta timestamp of the block time itself
		deltaTimestamp = block.Time()
	} else {
		// standard behaviour for non-injected or forced batches
		if l1Index != 0 {
			// read the index info itself
			l1Info, err := reader.GetL1InfoTreeUpdate(l1Index)
			if err != nil {
				return err
			}
			if l1Info != nil {
				ger = l1Info.GER
				l1BlockHash = l1Info.ParentHash
			}
		}
	}

	err = srv.AddBlockStart(block, currentBatch, uint16(fork), ger, uint32(deltaTimestamp), uint32(l1Index), l1BlockHash)
	if err != nil {
		return err
	}

	for _, tx := range block.Transactions() {
		effectiveGasPricePercentage, err := reader.GetEffectiveGasPricePercentage(tx.Hash())
		if err != nil {
			return err
		}
		stateRoot, err := reader.GetStateRoot(block.NumberU64())
		if err != nil {
			return err
		}
		_, err = srv.AddTransaction(effectiveGasPricePercentage, stateRoot, uint16(fork), tx)
		if err != nil {
			return err
		}
	}

	err = srv.AddBlockEnd(block.NumberU64(), block.Root(), block.Root())
	if err != nil {
		return err
	}

	return nil
}
