package stages

import (
	"context"
	"fmt"
	"github.com/0xPolygonHermez/zkevm-data-streamer/datastreamer"
	"github.com/gateway-fm/cdk-erigon-lib/kv"
	"github.com/ledgerwatch/erigon/core/rawdb"
	"github.com/ledgerwatch/erigon/eth/stagedsync"
	"github.com/ledgerwatch/erigon/eth/stagedsync/stages"
	"github.com/ledgerwatch/erigon/zk/datastream/server"
	"github.com/ledgerwatch/erigon/zk/hermez_db"
	"github.com/ledgerwatch/log/v3"
	"time"
	eritypes "github.com/ledgerwatch/erigon/core/types"
	"github.com/gateway-fm/cdk-erigon-lib/common"
)

type DataStreamCatchupCfg struct {
	db            kv.RwDB
	stream        *datastreamer.StreamServer
	chainId       uint64
	streamVersion int
}

func StageDataStreamCatchupCfg(stream *datastreamer.StreamServer, db kv.RwDB, chainId uint64, streamVersion int) DataStreamCatchupCfg {
	return DataStreamCatchupCfg{
		stream:        stream,
		db:            db,
		chainId:       chainId,
		streamVersion: streamVersion,
	}
}

func SpawnStageDataStreamCatchup(
	s *stagedsync.StageState,
	ctx context.Context,
	tx kv.RwTx,
	cfg DataStreamCatchupCfg,
) error {
	logPrefix := s.LogPrefix()
	log.Info(fmt.Sprintf("[%s] Starting...", logPrefix))
	stream := cfg.stream

	if stream == nil {
		// skip the stage if there is no streamer provided
		log.Info(fmt.Sprintf("[%s] no streamer provided, skipping stage", logPrefix))
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

	finalBlockNumber, err := CatchupDatastream(logPrefix, tx, stream, cfg.chainId, cfg.streamVersion)
	if err != nil {
		return err
	}

	if createdTx {
		if err := tx.Commit(); err != nil {
			log.Error(fmt.Sprintf("[%s] error: %s", logPrefix, err))
		}
	}

	log.Info(fmt.Sprintf("[%s] stage complete", logPrefix), "block", finalBlockNumber)

	return err
}

func CatchupDatastream(logPrefix string, tx kv.RwTx, stream *datastreamer.StreamServer, chainId uint64, streamVersion int) (uint64, error) {
	srv := server.NewDataStreamServer(stream, chainId, server.StandardOperationMode)
	reader := hermez_db.NewHermezDbReader(tx)

	// get the latest verified batch number
	latestBatch, err := stages.GetStageProgress(tx, stages.SequenceExecutorVerify)
	if err != nil {
		return 0, err
	}

	finalBlockNumber, err := reader.GetHighestBlockInBatch(latestBatch)
	if err != nil {
		return 0, err
	}

	previousProgress, err := stages.GetStageProgress(tx, stages.DataStream)
	if err != nil {
		return 0, err
	}

	log.Info(fmt.Sprintf("[%s] Getting progress", logPrefix),
		"adding up to blockNum", finalBlockNumber,
		"previousProgress", previousProgress,
	)

	var lastBlock *eritypes.Block

	// write genesis if we have no data in the stream yet
	if previousProgress == 0 {
		genesis, err := rawdb.ReadBlockByNumber(tx, 0)
		if err != nil {
			return 0, err
		}
		lastBlock = genesis
		if err = writeGenesisToStream(genesis, reader, stream, srv, streamVersion, chainId); err != nil {
			return 0, err
		}
	}

	logTicker := time.NewTicker(10 * time.Second)

	if err = stream.StartAtomicOp(); err != nil {
		return 0, err
	}
	totalToWrite := finalBlockNumber - previousProgress

	insertEntryCount := 1000000
	entriesProto := make([]server.DataStreamEntryProto, 1000000)
	index := 0
	for currentBlockNumber := previousProgress + 1; currentBlockNumber <= finalBlockNumber; currentBlockNumber++ {
		select {
		case <-logTicker.C:
			log.Info(fmt.Sprintf("[%s]: progress", logPrefix),
				"block", currentBlockNumber,
				"target", finalBlockNumber, "%", float64(currentBlockNumber-previousProgress)/float64(totalToWrite)*100)
		default:
		}

		if lastBlock == nil {
			lastBlock, err = rawdb.ReadBlockByNumber(tx, currentBlockNumber-1)
			if err != nil {
				return 0, err
			}
		}

		block, err := rawdb.ReadBlockByNumber(tx, currentBlockNumber)
		if err != nil {
			return 0, err
		}

		batchNum, err := reader.GetBatchNoByL2Block(currentBlockNumber)
		if err != nil {
			return 0, err
		}

		prevBatchNum, err := reader.GetBatchNoByL2Block(currentBlockNumber - 1)
		if err != nil {
			return 0, err
		}

		// TODO: oversight in stream write?
		//gersInBetween, err := reader.GetBatchGlobalExitRoots(prevBatchNum, batchNum)
		//if err != nil {
		//	return 0, err
		//}

		highestBlockInBatch, err := reader.GetHighestBlockInBatch(batchNum)
		if err != nil {
			return 0, err
		}

		batchEnd := highestBlockInBatch == block.NumberU64()

		l1InfoMinTimestamps := make(map[uint64]uint64)

		blockEntries, err := srv.CreateStreamEntriesProto(block, reader, lastBlock, batchNum, prevBatchNum, l1InfoMinTimestamps, batchEnd)
		if err != nil {
			return 0, err
		}

		for _, entry := range *blockEntries {
			entriesProto[index] = entry
			index++
		}

		// basically commit once 80% of the entries array is filled
		if index+1 >= insertEntryCount*4/5 {
			log.Info(fmt.Sprintf("[%s] Commit count reached, committing entries", logPrefix), "block", currentBlockNumber)
			if err = srv.CommitEntriesToStreamProto(entriesProto[:index]); err != nil {
				return 0, err
			}
			if err = stages.SaveStageProgress(tx, stages.DataStream, currentBlockNumber); err != nil {
				return 0, err
			}
			entriesProto = make([]server.DataStreamEntryProto, insertEntryCount)
			index = 0
		}

		lastBlock = block
	}

	if err = srv.CommitEntriesToStreamProto(entriesProto[:index]); err != nil {
		return 0, err
	}

	if err = stream.CommitAtomicOp(); err != nil {
		return 0, err
	}

	if err = stages.SaveStageProgress(tx, stages.DataStream, finalBlockNumber); err != nil {
		return 0, err
	}

	return finalBlockNumber, nil
}

func writeGenesisToStream(
	genesis *eritypes.Block,
	reader *hermez_db.HermezDbReader,
	stream *datastreamer.StreamServer,
	srv *server.DataStreamServer,
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

	if err = srv.CommitEntriesToStreamProto([]server.DataStreamEntryProto{batchBookmark, batchStart, l2BlockBookmark, l2Block, batchEnd}); err != nil {
		return err
	}

	err = stream.CommitAtomicOp()
	if err != nil {
		return err
	}

	return nil
}
