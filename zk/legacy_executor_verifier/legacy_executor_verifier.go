package legacy_executor_verifier

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"encoding/hex"
	"errors"
	"fmt"
	"strconv"

	"github.com/0xPolygonHermez/zkevm-data-streamer/datastreamer"
	"github.com/gateway-fm/cdk-erigon-lib/common"
	"github.com/gateway-fm/cdk-erigon-lib/kv"
	"github.com/ledgerwatch/erigon/chain"
	"github.com/ledgerwatch/erigon/core/rawdb"
	"github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/eth/ethconfig"
	"github.com/ledgerwatch/erigon/zk/datastream/server"
	"github.com/ledgerwatch/erigon/zk/hermez_db"
	"github.com/ledgerwatch/erigon/zk/legacy_executor_verifier/proto/github.com/0xPolygonHermez/zkevm-node/state/runtime/executor"
	"github.com/ledgerwatch/erigon/zk/utils"
	"github.com/ledgerwatch/log/v3"
)

var ErrNoExecutorAvailable = fmt.Errorf("no executor available")

type VerifierRequest struct {
	BatchNumber  uint64
	BlockNumber  uint64
	ForkId       uint64
	StateRoot    common.Hash
	Counters     map[string]int
	creationTime time.Time
}

func NewVerifierRequest(batchNumber, blockNumber, forkId uint64, stateRoot common.Hash, counters map[string]int) *VerifierRequest {
	return &VerifierRequest{
		BatchNumber:  batchNumber,
		BlockNumber:  blockNumber,
		ForkId:       forkId,
		StateRoot:    stateRoot,
		Counters:     counters,
		creationTime: time.Now(),
	}
}

func (vr *VerifierRequest) IsOverdue() bool {
	return time.Since(vr.creationTime) > time.Duration(30*time.Minute)
}

type VerifierResponse struct {
	BatchNumber      uint64
	BlockNumber      uint64
	Valid            bool
	Witness          []byte
	ExecutorResponse *executor.ProcessBatchResponseV2
	OriginalCounters map[string]int
	Error            error
}

type VerifierBundle struct {
	Request  *VerifierRequest
	Response *VerifierResponse
}

func NewVerifierBundle(request *VerifierRequest, response *VerifierResponse) *VerifierBundle {
	return &VerifierBundle{
		Request:  request,
		Response: response,
	}
}

type WitnessGenerator interface {
	GetWitnessByBlockRange(tx kv.Tx, ctx context.Context, startBlock, endBlock uint64, debug, witnessFull bool) ([]byte, error)
}

type LegacyExecutorVerifier struct {
	db                     kv.RwDB
	cfg                    ethconfig.Zk
	executors              []*Executor
	executorNumber         int
	cancelAllVerifications atomic.Bool

	// quit chan struct{}

	streamServer     *server.DataStreamServer
	witnessGenerator WitnessGenerator

	promises    []*Promise[*VerifierBundle]
	mtxPromises *sync.Mutex
	// addedBatches map[uint64]struct{}

	// these three items are used to keep track of where the datastream is at
	// compared with the executor checks.  It allows for data to arrive in strange
	// orders and will backfill the stream as needed.
	// lowestWrittenBatch uint64
	// responsesToWrite   map[uint64]struct{}
	// responsesMtx       *sync.Mutex
}

func NewLegacyExecutorVerifier(
	cfg ethconfig.Zk,
	executors []*Executor,
	chainCfg *chain.Config,
	db kv.RwDB,
	witnessGenerator WitnessGenerator,
	stream *datastreamer.StreamServer,
) *LegacyExecutorVerifier {
	streamServer := server.NewDataStreamServer(stream, chainCfg.ChainID.Uint64())
	return &LegacyExecutorVerifier{
		db:                     db,
		cfg:                    cfg,
		executors:              executors,
		executorNumber:         0,
		cancelAllVerifications: atomic.Bool{},
		// quit:                   make(chan struct{}),
		streamServer:     streamServer,
		witnessGenerator: witnessGenerator,
		promises:         make([]*Promise[*VerifierBundle], 0),
		mtxPromises:      &sync.Mutex{},
		// addedBatches:           make(map[uint64]struct{}),
		// responsesToWrite:       map[uint64]struct{}{},
		// responsesMtx:           &sync.Mutex{},
	}
}

func (v *LegacyExecutorVerifier) StartAsyncVerification(
	forkId uint64,
	batchNumber uint64,
	stateRoot common.Hash,
	counters map[string]int,
	blockNumbers []uint64,
	useRemoteExecutor bool,
) {
	var promise *Promise[*VerifierBundle]

	request := NewVerifierRequest(batchNumber, blockNumbers[len(blockNumbers)-1], forkId, stateRoot, counters)
	if useRemoteExecutor {
		promise = v.VerifyAsync(request, blockNumbers)
	} else {
		promise = v.VerifyWithoutExecutor(request, blockNumbers)
	}

	v.appendPromise(promise)
}

func (v *LegacyExecutorVerifier) appendPromise(promise *Promise[*VerifierBundle]) {
	v.mtxPromises.Lock()
	defer v.mtxPromises.Unlock()
	v.promises = append(v.promises, promise)
}

func (v *LegacyExecutorVerifier) VerifySync(tx kv.Tx, request *VerifierRequest, witness, streamBytes []byte, timestampLimit, firstBlockNumber uint64, l1InfoTreeMinTimestamps map[uint64]uint64) error {
	oldAccInputHash := common.HexToHash("0x0")
	payload := &Payload{
		Witness:                 witness,
		DataStream:              streamBytes,
		Coinbase:                v.cfg.AddressSequencer.String(),
		OldAccInputHash:         oldAccInputHash.Bytes(),
		L1InfoRoot:              nil,
		TimestampLimit:          timestampLimit,
		ForcedBlockhashL1:       []byte{0},
		ContextId:               strconv.FormatUint(request.BatchNumber, 10),
		L1InfoTreeMinTimestamps: l1InfoTreeMinTimestamps,
	}

	e := v.GetNextOnlineAvailableExecutor()
	if e == nil {
		return ErrNoExecutorAvailable
	}

	t := utils.StartTimer("legacy-executor-verifier", "verify-sync")
	defer t.LogTimer()

	e.AquireAccess()
	defer e.ReleaseAccess()

	previousBlock, err := rawdb.ReadBlockByNumber(tx, firstBlockNumber-1)
	if err != nil {
		return err
	}

	_, _, executorErr := e.Verify(payload, request, previousBlock.Root())
	return executorErr
}

// Unsafe is not thread-safe so it MUST be invoked only from a single thread
// func (v *LegacyExecutorVerifier) AddRequestUnsafe(request *VerifierRequest, sequencerBatchSealTime time.Duration) *Promise[*VerifierBundle] {
// 	// eager promise will do the work as soon as called in a goroutine, then we can retrieve the result later
// 	// ProcessResultsSequentiallyUnsafe relies on the fact that this function returns ALWAYS non-verifierBundle and error. The only exception is the case when verifications has been canceled. Only then the verifierBundle can be nil
// 	promise := NewPromise[*VerifierBundle](func() (*VerifierBundle, error) {
// 		verifierBundle := NewVerifierBundle(request, nil)

// 		e := v.GetNextOnlineAvailableExecutor()
// 		if e == nil {
// 			return verifierBundle, ErrNoExecutorAvailable
// 		}

// 		t := utils.StartTimer("legacy-executor-verifier", "add-request-unsafe")

// 		e.AquireAccess()
// 		defer e.ReleaseAccess()
// 		if v.cancelAllVerifications.Load() {
// 			return nil, ErrPromiseCancelled
// 		}

// 		var err error
// 		var blocks []uint64
// 		startTime := time.Now()
// 		ctx := context.Background()
// 		// mapmutation has some issue with us not having a quit channel on the context call to `Done` so
// 		// here we're creating a cancelable context and just deferring the cancel
// 		innerCtx, cancel := context.WithCancel(ctx)
// 		defer cancel()

// 		// get the data stream bytes
// 		for time.Since(startTime) < 3*sequencerBatchSealTime {
// 			// we might not have blocks yet as the underlying stage loop might still be running and the tx hasn't been
// 			// committed yet so just requeue the request
// 			blocks, err = v.availableBlocksToProcess(innerCtx, request.BatchNumber)
// 			if err != nil {
// 				return verifierBundle, err
// 			}

// 			if len(blocks) > 0 {
// 				break
// 			}

// 			time.Sleep(time.Second)
// 		}

// 		if len(blocks) == 0 {
// 			return verifierBundle, fmt.Errorf("still not blocks in this batch")
// 		}

// 		tx, err := v.db.BeginRo(innerCtx)
// 		if err != nil {
// 			return verifierBundle, err
// 		}
// 		defer tx.Rollback()

// 		hermezDb := hermez_db.NewHermezDbReader(tx)

// 		l1InfoTreeMinTimestamps := make(map[uint64]uint64)
// 		streamBytes, err := v.GetWholeBatchStreamBytes(request.BatchNumber, tx, blocks, hermezDb, l1InfoTreeMinTimestamps, nil)
// 		if err != nil {
// 			return verifierBundle, err
// 		}

// 		witness, err := v.witnessGenerator.GetWitnessByBlockRange(tx, ctx, blocks[0], blocks[len(blocks)-1], false, v.cfg.WitnessFull)
// 		if err != nil {
// 			return nil, err
// 		}

// 		log.Debug("witness generated", "data", hex.EncodeToString(witness))

// 		// now we need to figure out the timestamp limit for this payload.  It must be:
// 		// timestampLimit >= currentTimestamp (from batch pre-state) + deltaTimestamp
// 		// so to ensure we have a good value we can take the timestamp of the last block in the batch
// 		// and just add 5 minutes
// 		lastBlock, err := rawdb.ReadBlockByNumber(tx, blocks[len(blocks)-1])
// 		if err != nil {
// 			return verifierBundle, err
// 		}

// 		// executor is perfectly happy with just an empty hash here
// 		oldAccInputHash := common.HexToHash("0x0")
// 		timestampLimit := lastBlock.Time()
// 		payload := &Payload{
// 			Witness:                 witness,
// 			DataStream:              streamBytes,
// 			Coinbase:                v.cfg.AddressSequencer.String(),
// 			OldAccInputHash:         oldAccInputHash.Bytes(),
// 			L1InfoRoot:              nil,
// 			TimestampLimit:          timestampLimit,
// 			ForcedBlockhashL1:       []byte{0},
// 			ContextId:               strconv.FormatUint(request.BatchNumber, 10),
// 			L1InfoTreeMinTimestamps: l1InfoTreeMinTimestamps,
// 		}

// 		previousBlock, err := rawdb.ReadBlockByNumber(tx, blocks[0]-1)
// 		if err != nil {
// 			return verifierBundle, err
// 		}

// 		ok, executorResponse, executorErr := e.Verify(payload, request, previousBlock.Root())
// 		if executorErr != nil {
// 			if errors.Is(executorErr, ErrExecutorStateRootMismatch) {
// 				log.Error("[Verifier] State root mismatch detected", "err", executorErr)
// 			} else if errors.Is(executorErr, ErrExecutorUnknownError) {
// 				log.Error("[Verifier] Unexpected error found from executor", "err", executorErr)
// 			} else {
// 				log.Error("[Verifier] Error", "err", executorErr)
// 			}
// 		}

// 		// log timing w/o stream write
// 		t.LogTimer()

// 		if ok {
// 			if err = v.checkAndWriteToStream(tx, hermezDb, request.BatchNumber); err != nil {
// 				log.Error("error writing data to stream", "err", err)
// 			}
// 		}

// 		verifierBundle.Response = &VerifierResponse{
// 			BatchNumber:      request.BatchNumber,
// 			Valid:            ok,
// 			Witness:          witness,
// 			ExecutorResponse: executorResponse,
// 			Error:            executorErr,
// 		}
// 		return verifierBundle, nil
// 	})

// 	// add batch to the list of batches we've added
// 	v.addedBatches[request.BatchNumber] = struct{}{}

// 	// add the promise to the list of promises
// 	v.promises = append(v.promises, promise)
// 	return promise
// }

var counter = 0

func (v *LegacyExecutorVerifier) VerifyAsync(request *VerifierRequest, blockNumbers []uint64) *Promise[*VerifierBundle] {
	// eager promise will do the work as soon as called in a goroutine, then we can retrieve the result later
	// ProcessResultsSequentiallyUnsafe relies on the fact that this function returns ALWAYS non-verifierBundle and error. The only exception is the case when verifications has been canceled. Only then the verifierBundle can be nil
	return NewPromise[*VerifierBundle](func() (*VerifierBundle, error) {
		verifierBundle := NewVerifierBundle(request, nil)
		// bundleWithBlocks := &VerifierBundle{
		// 	Blocks: blockNumbers,
		// 	Bundle: verifierBundle,
		// }

		e := v.GetNextOnlineAvailableExecutor()
		if e == nil {
			return verifierBundle, ErrNoExecutorAvailable
		}

		e.AquireAccess()
		defer e.ReleaseAccess()
		if v.cancelAllVerifications.Load() {
			return nil, ErrPromiseCancelled
		}

		var err error
		ctx := context.Background()
		// mapmutation has some issue with us not having a quit channel on the context call to `Done` so
		// here we're creating a cancelable context and just deferring the cancel
		innerCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		tx, err := v.db.BeginRo(innerCtx)
		if err != nil {
			return verifierBundle, err
		}
		defer tx.Rollback()

		hermezDb := hermez_db.NewHermezDbReader(tx)

		l1InfoTreeMinTimestamps := make(map[uint64]uint64)
		streamBytes, err := v.GetWholeBatchStreamBytes(request.BatchNumber, tx, blockNumbers, hermezDb, l1InfoTreeMinTimestamps, nil)
		if err != nil {
			return verifierBundle, err
		}

		witness, err := v.witnessGenerator.GetWitnessByBlockRange(tx, ctx, blockNumbers[0], blockNumbers[len(blockNumbers)-1], false, v.cfg.WitnessFull)
		if err != nil {
			return verifierBundle, err
		}

		log.Debug("witness generated", "data", hex.EncodeToString(witness))

		// now we need to figure out the timestamp limit for this payload.  It must be:
		// timestampLimit >= currentTimestamp (from batch pre-state) + deltaTimestamp
		// so to ensure we have a good value we can take the timestamp of the last block in the batch
		// and just add 5 minutes
		lastBlock, err := rawdb.ReadBlockByNumber(tx, blockNumbers[len(blockNumbers)-1])
		if err != nil {
			return verifierBundle, err
		}

		// executor is perfectly happy with just an empty hash here
		oldAccInputHash := common.HexToHash("0x0")
		timestampLimit := lastBlock.Time()
		payload := &Payload{
			Witness:                 witness,
			DataStream:              streamBytes,
			Coinbase:                v.cfg.AddressSequencer.String(),
			OldAccInputHash:         oldAccInputHash.Bytes(),
			L1InfoRoot:              nil,
			TimestampLimit:          timestampLimit,
			ForcedBlockhashL1:       []byte{0},
			ContextId:               strconv.FormatUint(request.BatchNumber, 10),
			L1InfoTreeMinTimestamps: l1InfoTreeMinTimestamps,
		}

		previousBlock, err := rawdb.ReadBlockByNumber(tx, blockNumbers[0]-1)
		if err != nil {
			return verifierBundle, err
		}

		ok, executorResponse, executorErr := e.Verify(payload, request, previousBlock.Root())

		if request.BlockNumber == 4 && counter == 0 {
			ok = false
			counter = 1
		}

		if executorErr != nil {
			if errors.Is(executorErr, ErrExecutorStateRootMismatch) {
				log.Error("[Verifier] State root mismatch detected", "err", executorErr)
			} else if errors.Is(executorErr, ErrExecutorUnknownError) {
				log.Error("[Verifier] Unexpected error found from executor", "err", executorErr)
			} else {
				log.Error("[Verifier] Error", "err", executorErr)
			}
		}

		verifierBundle.Response = &VerifierResponse{
			BatchNumber:      request.BatchNumber,
			BlockNumber:      request.BlockNumber,
			Valid:            ok,
			Witness:          witness,
			ExecutorResponse: executorResponse,
			Error:            executorErr,
		}
		return verifierBundle, nil
	})
}

func (v *LegacyExecutorVerifier) VerifyWithoutExecutor(request *VerifierRequest, blockNumbers []uint64) *Promise[*VerifierBundle] {
	valid := true
	// simulate a die roll to determine if this is a good batch or not
	// 1 in 6 chance of being a bad batch
	// if rand.Intn(6) == 0 {
	// 	valid = false
	// }

	promise := NewPromise[*VerifierBundle](func() (*VerifierBundle, error) {
		response := &VerifierResponse{
			BatchNumber:      request.BatchNumber,
			BlockNumber:      request.BlockNumber,
			Valid:            valid,
			OriginalCounters: request.Counters,
			Witness:          nil,
			ExecutorResponse: nil,
			Error:            nil,
		}
		return NewVerifierBundle(request, response), nil
	})
	promise.Wait()

	return promise
}

func (v *LegacyExecutorVerifier) ProcessResultsSequentially() ([]*VerifierBundle, int, error) {
	v.mtxPromises.Lock()
	defer v.mtxPromises.Unlock()

	var verifierResponse []*VerifierBundle

	// not a stop signal, so we can start to process our promises now
	for idx, promise := range v.promises {
		verifierBundle, err := promise.TryGet()
		if verifierBundle == nil && err == nil {
			// If code enters here this means that this promise is not yet completed
			// We must processes responses sequentially so if this one is not ready we can just break
			break
		}

		if err != nil {
			// let leave it for debug purposes
			// a cancelled promise is removed from v.promises => it should never appear here, that's why let's panic if it happens, because it will indicate for massive error
			if errors.Is(err, ErrPromiseCancelled) {
				panic("this should never happen")
			}

			log.Error("error on our end while preparing the verification request, re-queueing the task", "err", err)

			if verifierBundle.Request.IsOverdue() {
				// signal an error, the caller can check on this and stop the process if needs be
				return nil, 0, fmt.Errorf("error: batch %d couldn't be processed in 30 minutes", verifierBundle.Request.BatchNumber)
			}

			// re-queue the task - it should be safe to replace the index of the slice here as we only add to it
			v.promises[idx] = promise.CloneAndRerun()

			// break now as we know we can't proceed here until this promise is attempted again
			break
		}

		verifierResponse = append(verifierResponse, verifierBundle)
	}

	// remove processed promises from the list
	v.promises = v.promises[len(verifierResponse):]

	return verifierResponse, len(v.promises), nil
}

// func (v *LegacyExecutorVerifier) checkAndWriteToStream(tx kv.Tx, hdb *hermez_db.HermezDbReader, newBatch uint64) error {
// 	t := utils.StartTimer("legacy-executor-verifier", "check-and-write-to-stream")
// 	defer t.LogTimer()

// 	v.responsesMtx.Lock()
// 	defer v.responsesMtx.Unlock()

// 	v.responsesToWrite[newBatch] = struct{}{}

// 	// if we haven't written anything yet - cold start of the node
// 	if v.lowestWrittenBatch == 0 {
// 		// we haven't written anything yet so lets make sure there is no gap
// 		// in the stream for this batch
// 		latestBatch, err := v.streamServer.GetHighestBatchNumber()
// 		if err != nil {
// 			return err
// 		}
// 		log.Info("[Verifier] Initialising on cold start", "latestBatch", latestBatch, "newBatch", newBatch)

// 		v.lowestWrittenBatch = latestBatch

// 		// check if we have the next batch we're waiting for
// 		if latestBatch == newBatch-1 {
// 			if err := v.WriteBatchToStream(newBatch, hdb, tx); err != nil {
// 				return err
// 			}
// 			v.lowestWrittenBatch = newBatch
// 			delete(v.responsesToWrite, newBatch)
// 		}
// 	}

// 	// now check if the batch we want next is good
// 	for {
// 		// check if we have the next batch to write
// 		nextBatch := v.lowestWrittenBatch + 1
// 		if _, ok := v.responsesToWrite[nextBatch]; !ok {
// 			break
// 		}

// 		if err := v.WriteBatchToStream(nextBatch, hdb, tx); err != nil {
// 			return err
// 		}
// 		delete(v.responsesToWrite, nextBatch)
// 		v.lowestWrittenBatch = nextBatch
// 	}

// 	return nil
// }

// Unsafe is not thread-safe so it MUST be invoked only from a single thread
// func (v *LegacyExecutorVerifier) ProcessResultsSequentiallyUnsafe(tx kv.RwTx) ([]*VerifierResponse, error) {
// 	results := make([]*VerifierResponse, 0, len(v.promises))
// 	for i := 0; i < len(v.promises); i++ {
// 		verifierBundle, err := v.promises[i].TryGet()
// 		if verifierBundle == nil && err == nil {
// 			break
// 		}

// 		if err != nil {
// 			// let leave it for debug purposes
// 			// a cancelled promise is removed from v.promises => it should never appear here, that's why let's panic if it happens, because it will indicate for massive error
// 			if errors.Is(err, ErrPromiseCancelled) {
// 				panic("this should never happen")
// 			}

// 			log.Error("error on our end while preparing the verification request, re-queueing the task", "err", err)
// 			// this is an error on our end, so just re-create the promise at exact position where it was
// 			if verifierBundle.Request.IsOverdue() {
// 				return nil, fmt.Errorf("error: batch %d couldn't be processed in 30 minutes", verifierBundle.Request.BatchNumber)
// 			}

// 			v.promises[i] = NewPromise[*VerifierBundle](v.promises[i].task)
// 			break
// 		}

// 		verifierResponse := verifierBundle.Response
// 		results = append(results, verifierResponse)
// 		delete(v.addedBatches, verifierResponse.BatchNumber)

// 		// no point to process any further responses if we've found an invalid one
// 		if !verifierResponse.Valid {
// 			break
// 		}
// 	}

// 	return results, nil
// }

// func (v *LegacyExecutorVerifier) MarkTopResponseAsProcessed(batchNumber uint64) {
// 	v.promises = v.promises[1:]
// 	delete(v.addedBatches, batchNumber)
// }

func (v *LegacyExecutorVerifier) CancelAllRequests() {
	// cancel all promises
	// all queued promises will return ErrPromiseCancelled while getting its result
	for _, p := range v.promises {
		p.Cancel()
	}

	// the goal of this car is to ensure that running promises are stopped as soon as possible
	// we need it because the promise's function must finish and then the promise checks if it has been cancelled
	v.cancelAllVerifications.Store(true)

	for _, e := range v.executors {
		// let's wait for all threads that are waiting to add to v.openRequests to finish
		for e.QueueLength() > 0 {
			time.Sleep(1 * time.Millisecond)
		}
	}

	v.cancelAllVerifications.Store(false)

	v.promises = make([]*Promise[*VerifierBundle], 0)
}

// // Unsafe is not thread-safe so it MUST be invoked only from a single thread
// func (v *LegacyExecutorVerifier) HasExecutorsUnsafe() bool {
// 	return len(v.executors) > 0
// }

// Unsafe is not thread-safe so it MUST be invoked only from a single thread
// func (v *LegacyExecutorVerifier) IsRequestAddedUnsafe(batch uint64) bool {
// 	_, ok := v.addedBatches[batch]
// 	return ok
// }

// func (v *LegacyExecutorVerifier) WriteBatchToStream(batchNumber uint64, hdb *hermez_db.HermezDbReader, roTx kv.Tx) error {
// 	log.Info("[Verifier] Writing batch to stream", "batch", batchNumber)

// 	if err := v.streamServer.WriteWholeBatchToStream("verifier", roTx, hdb, v.lowestWrittenBatch, batchNumber); err != nil {
// 		return err
// 	}
// 	return nil
// }

func (v *LegacyExecutorVerifier) GetNextOnlineAvailableExecutor() *Executor {
	var exec *Executor

	// TODO: find executors with spare capacity

	// attempt to find an executor that is online amongst them all
	for i := 0; i < len(v.executors); i++ {
		v.executorNumber++
		if v.executorNumber >= len(v.executors) {
			v.executorNumber = 0
		}
		temp := v.executors[v.executorNumber]
		if temp.CheckOnline() {
			exec = temp
			break
		}
	}

	return exec
}

// func (v *LegacyExecutorVerifier) availableBlocksToProcess(innerCtx context.Context, batchNumber uint64) ([]uint64, error) {
// 	tx, err := v.db.BeginRo(innerCtx)
// 	if err != nil {
// 		return []uint64{}, err
// 	}
// 	defer tx.Rollback()

// 	hermezDb := hermez_db.NewHermezDbReader(tx)
// 	blocks, err := hermezDb.GetL2BlockNosByBatch(batchNumber)
// 	if err != nil {
// 		return []uint64{}, err
// 	}

// 	for _, blockNum := range blocks {
// 		block, err := rawdb.ReadBlockByNumber(tx, blockNum)
// 		if err != nil {
// 			return []uint64{}, err
// 		}
// 		if block == nil {
// 			return []uint64{}, nil
// 		}
// 	}

// 	return blocks, nil
// }

func (v *LegacyExecutorVerifier) GetWholeBatchStreamBytes(
	batchNumber uint64,
	tx kv.Tx,
	blockNumbers []uint64,
	hermezDb *hermez_db.HermezDbReader,
	l1InfoTreeMinTimestamps map[uint64]uint64,
	transactionsToIncludeByIndex [][]int, // passing nil here will include all transactions in the blocks
) (streamBytes []byte, err error) {
	blocks := make([]types.Block, 0, len(blockNumbers))
	txsPerBlock := make(map[uint64][]types.Transaction)

	// as we only ever use the executor verifier for whole batches we can safely assume that the previous batch
	// will always be the request batch - 1 and that the first block in the batch will be at the batch
	// boundary so we will always add in the batch bookmark to the stream
	previousBatch := batchNumber - 1

	for idx, blockNumber := range blockNumbers {
		block, err := rawdb.ReadBlockByNumber(tx, blockNumber)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, *block)

		filteredTransactions := block.Transactions()
		// filter transactions by indexes that should be included
		if transactionsToIncludeByIndex != nil {
			filteredTransactions = filterTransactionByIndexes(block.Transactions(), transactionsToIncludeByIndex[idx])
		}

		txsPerBlock[blockNumber] = filteredTransactions
	}

	entries, err := server.BuildWholeBatchStreamEntriesProto(tx, hermezDb, v.streamServer.GetChainId(), batchNumber, previousBatch, blocks, txsPerBlock, l1InfoTreeMinTimestamps)
	if err != nil {
		return nil, err
	}

	return entries.Marshal()
}

func filterTransactionByIndexes(
	filteredTransactions types.Transactions,
	transactionsToIncludeByIndex []int,
) types.Transactions {
	if transactionsToIncludeByIndex != nil {
		filteredTransactionsBuilder := make(types.Transactions, len(transactionsToIncludeByIndex))
		for i, txIndexInBlock := range transactionsToIncludeByIndex {
			filteredTransactionsBuilder[i] = filteredTransactions[txIndexInBlock]
		}

		filteredTransactions = filteredTransactionsBuilder
	}

	return filteredTransactions
}
