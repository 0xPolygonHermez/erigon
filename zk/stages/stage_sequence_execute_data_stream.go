package stages

import (
	"context"
	"fmt"

	"github.com/ledgerwatch/erigon/core/rawdb"
	"github.com/ledgerwatch/erigon/zk/datastream/server"
	verifier "github.com/ledgerwatch/erigon/zk/legacy_executor_verifier"
	"github.com/ledgerwatch/erigon/zk/utils"
	"github.com/ledgerwatch/log/v3"
)

type SequencerBatchStreamWriter struct {
	ctx            context.Context
	logPrefix      string
	legacyVerifier *verifier.LegacyExecutorVerifier
	sdb            *stageDb
	streamServer   *server.DataStreamServer
	hasExecutors   bool
	lastBatch      uint64
}

type BlockStatus struct {
	BlockNumber uint64
	Valid       bool
	Error       error
}

func (sbc *SequencerBatchStreamWriter) CommitNewUpdates() ([]BlockStatus, int, error) {
	var written []BlockStatus
	responses, remaining, err := sbc.legacyVerifier.ProcessResultsSequentially()
	if err != nil {
		return written, remaining, err
	}

	if len(responses) == 0 {
		return written, remaining, nil
	}

	written, err = sbc.writeBlockDetailsToDatastream(responses)
	if err != nil {
		return written, remaining, err
	}

	return written, remaining, nil
}

func (sbc *SequencerBatchStreamWriter) writeBlockDetailsToDatastream(verifiedBundles []*verifier.VerifierBundle) ([]BlockStatus, error) {
	var written []BlockStatus
	for _, bundle := range verifiedBundles {
		request := bundle.Request
		response := bundle.Response

		if response.Valid {
			parentBlock, err := rawdb.ReadBlockByNumber(sbc.sdb.tx, response.BlockNumber-1)
			if err != nil {
				return written, err
			}
			block, err := rawdb.ReadBlockByNumber(sbc.sdb.tx, response.BlockNumber)
			if err != nil {
				return written, err
			}

			if err := sbc.streamServer.WriteBlockWithBatchStartToStream(sbc.logPrefix, sbc.sdb.tx, sbc.sdb.hermezDb, request.ForkId, response.BatchNumber, sbc.lastBatch, *parentBlock, *block); err != nil {
				return written, err
			}

			// once we have handled the very first block we can update the last batch to be the current batch safely so that
			// we don't keep adding batch bookmarks in between blocks
			sbc.lastBatch = response.BatchNumber
		}

		status := BlockStatus{
			BlockNumber: response.BlockNumber,
			Valid:       response.Valid,
			Error:       response.Error,
		}

		written = append(written, status)

		// just break early if there is an invalid response as we don't want to process the remainder anyway
		if !response.Valid {
			break
		}
	}

	return written, nil
}

func finalizeLastBatchInDatastreamIfNotFinalized(batchContext *BatchContext, batchState *BatchState, thisBlock uint64) error {
	isLastEntryBatchEnd, err := batchContext.cfg.datastreamServer.IsLastEntryBatchEnd()
	if err != nil {
		return err
	}

	if isLastEntryBatchEnd {
		return nil
	}

	log.Warn(fmt.Sprintf("[%s] Last batch %d was not closed properly, closing it now...", batchContext.s.LogPrefix(), batchState.batchNumber))
	ler, err := utils.GetBatchLocalExitRootFromSCStorage(batchState.batchNumber, batchContext.sdb.hermezDb.HermezDbReader, batchContext.sdb.tx)
	if err != nil {
		return err
	}

	lastBlock, err := rawdb.ReadBlockByNumber(batchContext.sdb.tx, thisBlock)
	if err != nil {
		return err
	}
	root := lastBlock.Root()
	if err = batchContext.cfg.datastreamServer.WriteBatchEnd(batchContext.sdb.hermezDb, batchState.batchNumber, batchState.batchNumber-1, &root, &ler); err != nil {
		return err
	}
	return nil
}
