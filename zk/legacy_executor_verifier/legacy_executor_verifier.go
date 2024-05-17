package legacy_executor_verifier

import (
	"context"
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
	"github.com/ledgerwatch/erigon/eth/ethconfig"
	"github.com/ledgerwatch/erigon/zk/datastream/server"
	dstypes "github.com/ledgerwatch/erigon/zk/datastream/types"
	"github.com/ledgerwatch/erigon/zk/hermez_db"
	"github.com/ledgerwatch/erigon/zk/legacy_executor_verifier/proto/github.com/0xPolygonHermez/zkevm-node/state/runtime/executor"
	"github.com/ledgerwatch/erigon/zk/syncer"
	"github.com/ledgerwatch/log/v3"
)

var ErrNoExecutorAvailable = fmt.Errorf("no executor available")

type VerifierRequest struct {
	BatchNumber  uint64
	ForkId       uint64
	StateRoot    common.Hash
	Counters     map[string]int
	creationTime time.Time
}

func NewVerifierRequest(batchNumber, forkId uint64, stateRoot common.Hash, counters map[string]int) *VerifierRequest {
	return &VerifierRequest{
		BatchNumber:  batchNumber,
		ForkId:       forkId,
		StateRoot:    stateRoot,
		Counters:     counters,
		creationTime: time.Now(),
	}
}

func (vr *VerifierRequest) isOverdue() bool {
	return time.Since(vr.creationTime) > time.Duration(30*time.Minute)
}

type VerifierResponse struct {
	BatchNumber      uint64
	Valid            bool
	Witness          []byte
	ExecutorResponse *executor.ProcessBatchResponseV2
	Error            error
}

type VerifierBundle struct {
	request  *VerifierRequest
	response *VerifierResponse
}

func NewVerifierBundle(request *VerifierRequest, response *VerifierResponse) *VerifierBundle {
	return &VerifierBundle{
		request:  request,
		response: response,
	}
}

type ILegacyExecutor interface {
	Verify(*Payload, *VerifierRequest, common.Hash) (bool, *executor.ProcessBatchResponseV2, error)
	CheckOnline() bool
	QueueLength() int
	CancelAllVerifications()
	AllowAllVerifications()
}

type WitnessGenerator interface {
	GenerateWitness(tx kv.Tx, ctx context.Context, startBlock, endBlock uint64, debug, witnessFull bool) ([]byte, error)
}

type LegacyExecutorVerifier struct {
	db             kv.RwDB
	cfg            ethconfig.Zk
	executors      []ILegacyExecutor
	executorNumber int

	quit chan struct{}

	streamServer     *server.DataStreamServer
	stream           *datastreamer.StreamServer
	witnessGenerator WitnessGenerator
	l1Syncer         *syncer.L1Syncer

	promises     []*Promise[*VerifierBundle]
	addedBatches map[uint64]struct{}
}

func NewLegacyExecutorVerifier(
	cfg ethconfig.Zk,
	executors []ILegacyExecutor,
	chainCfg *chain.Config,
	db kv.RwDB,
	witnessGenerator WitnessGenerator,
	l1Syncer *syncer.L1Syncer,
	stream *datastreamer.StreamServer,
) *LegacyExecutorVerifier {
	streamServer := server.NewDataStreamServer(stream, chainCfg.ChainID.Uint64(), server.ExecutorOperationMode)
	return &LegacyExecutorVerifier{
		db:               db,
		cfg:              cfg,
		executors:        executors,
		executorNumber:   0,
		quit:             make(chan struct{}),
		streamServer:     streamServer,
		stream:           stream,
		witnessGenerator: witnessGenerator,
		l1Syncer:         l1Syncer,
		promises:         make([]*Promise[*VerifierBundle], 0),
		addedBatches:     make(map[uint64]struct{}),
	}
}

// var counter = int32(0)

// Unsafe is not thread-safe so it MUST be invoked only from a single thread
func (v *LegacyExecutorVerifier) AddRequestUnsafe(request *VerifierRequest, sequencerBatchSealTime time.Duration) *Promise[*VerifierBundle] {
	// eager promise will do the work as soon as called in a goroutine, then we can retrieve the result later
	promise := NewPromise[*VerifierBundle](func() (*VerifierBundle, error) {
		var err error
		var tx kv.Tx
		var hermezDb *hermez_db.HermezDbReader
		var blocks []uint64
		// hermezDb := hermez_db.NewHermezDbReader(tx)
		startTime := time.Now()
		ctx := context.Background()

		// mapmutation has some issue with us not having a quit channel on the context call to `Done` so
		// here we're creating a cancelable context and just deferring the cancel
		innerCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		// get the data stream bytes
		for time.Since(startTime) < 2*sequencerBatchSealTime {
			tx, err = v.db.BeginRo(innerCtx)
			if err != nil {
				return nil, err
			}

			hermezDb = hermez_db.NewHermezDbReader(tx)
			blocks, err = hermezDb.GetL2BlockNosByBatch(request.BatchNumber)
			if err != nil {
				tx.Rollback()
				return nil, err
			}

			// we might not have blocks yet as the underlying stage loop might still be running and the tx hasn't been
			// committed yet so just requeue the request
			if len(blocks) > 0 {
				break
			}

			tx.Rollback()
			time.Sleep(time.Second)
		}

		defer tx.Rollback()

		if len(blocks) == 0 {
			return nil, fmt.Errorf("error: no blocks in batch %d", request.BatchNumber)
		}

		l1InfoTreeMinTimestamps := make(map[uint64]uint64)
		streamBytes, err := v.getStreamBytes(request, tx, blocks, hermezDb, l1InfoTreeMinTimestamps)
		if err != nil {
			return nil, err
		}

		witness, err := v.witnessGenerator.GenerateWitness(tx, innerCtx, blocks[0], blocks[len(blocks)-1], false, v.cfg.WitnessFull)
		if err != nil {
			return nil, err
		}

		log.Debug("witness generated", "data", hex.EncodeToString(witness))

		// executor is perfectly happy with just an empty hash here
		oldAccInputHash := common.HexToHash("0x0")

		// now we need to figure out the timestamp limit for this payload.  It must be:
		// timestampLimit >= currentTimestamp (from batch pre-state) + deltaTimestamp
		// so to ensure we have a good value we can take the timestamp of the last block in the batch
		// and just add 5 minutes
		lastBlock, err := rawdb.ReadBlockByNumber(tx, blocks[len(blocks)-1])
		if err != nil {
			return nil, err
		}

		timestampLimit := lastBlock.Time()
		payload := &Payload{
			Witness:                 witness,
			DataStream:              streamBytes,
			Coinbase:                v.cfg.AddressSequencer.String(),
			OldAccInputHash:         oldAccInputHash.Bytes(),
			L1InfoRoot:              nil,
			TimestampLimit:          timestampLimit,
			ForcedBlockhashL1:       []byte{0},
			ContextId:               strconv.Itoa(int(request.BatchNumber)),
			L1InfoTreeMinTimestamps: l1InfoTreeMinTimestamps,
		}

		previousBlock, err := rawdb.ReadBlockByNumber(tx, blocks[0]-1)
		if err != nil {
			return nil, err
		}

		e := v.getNextOnlineAvailableExecutor()
		if e == nil {
			return nil, ErrNoExecutorAvailable
		}

		ok, executorResponse, executorErr := e.Verify(payload, request, previousBlock.Root())
		if executorErr != nil {
			if errors.Is(err, ErrExecutorStateRootMismatch) {
				log.Error("[Verifier] State root mismatch detected", "err", err)
			} else if errors.Is(err, ErrExecutorUnknownError) {
				log.Error("[Verifier] Unexpected error found from executor", "err", err)
			} else {
				log.Error("[Verifier] Error", "err", err)
			}
		}

		// if request.BatchNumber == 2 && atomic.LoadInt32(&counter) == 0 {
		// 	ok = false
		// 	atomic.StoreInt32(&counter, 1)
		// }

		return NewVerifierBundle(request, &VerifierResponse{
			BatchNumber:      request.BatchNumber,
			Valid:            ok,
			Witness:          witness,
			ExecutorResponse: executorResponse,
			Error:            executorErr,
		}), nil
	})

	// add batch to the list of batches we've added
	v.addedBatches[request.BatchNumber] = struct{}{}

	// add the promise to the list of promises
	v.promises = append(v.promises, promise)
	return promise
}

// Unsafe is not thread-safe so it MUST be invoked only from a single thread
func (v *LegacyExecutorVerifier) ProcessResultsUnsafe(tx kv.RwTx) ([]*VerifierResponse, error) {
	hdb := hermez_db.NewHermezDbReader(tx)

	results := make([]*VerifierResponse, 0, len(v.promises))
	for i := 0; i < len(v.promises); i++ {
		verifierBundle, err := v.promises[i].TryGet()
		if verifierBundle == nil && err == nil {
			break
		}

		verifierResponse := verifierBundle.response
		if err != nil {
			log.Error("error on our end while preparing the verification request, re-queueing the task", "err", err)
			// this is an error on our end, so just re-create the promise at exact position where it was
			if verifierBundle.request.isOverdue() {
				return nil, fmt.Errorf("error: batch %d couldn't be processed in 30 minutes", verifierBundle.request.BatchNumber)
			}

			v.promises[i] = NewPromise[*VerifierBundle](v.promises[i].task)
			break
		}

		if verifierResponse.Valid {
			if err = v.WriteBatchToStream(verifierResponse.BatchNumber, hdb, tx); err != nil {
				log.Error("error getting verifier result", "err", err)
			}
		}

		results = append(results, verifierResponse)
		delete(v.addedBatches, verifierResponse.BatchNumber)

		// no point to process any further responses if we've found an invalid one
		if !verifierResponse.Valid {
			break
		}
	}

	// leave only non-processed promises
	v.promises = v.promises[len(results):]

	return results, nil
}

// Unsafe is not thread-safe so it MUST be invoked only from a single thread
func (v *LegacyExecutorVerifier) CancelAllRequestsUnsafe() {
	for _, e := range v.executors {
		e.CancelAllVerifications()
	}

	for _, e := range v.executors {
		// lets wait for all threads that are waiting to add to v.openRequests to finish
		for e.QueueLength() > 0 {
			time.Sleep(1 * time.Millisecond)
		}
	}

	for _, e := range v.executors {
		e.AllowAllVerifications()
	}

	v.promises = make([]*Promise[*VerifierBundle], 0)
}

// Unsafe is not thread-safe so it MUST be invoked only from a single thread
func (v *LegacyExecutorVerifier) HasExecutorsUnsafe() bool {
	return len(v.executors) > 0
}

// Unsafe is not thread-safe so it MUST be invoked only from a single thread
func (v *LegacyExecutorVerifier) IsRequestAddedUnsafe(batch uint64) bool {
	_, ok := v.addedBatches[batch]
	return ok
}

func (v *LegacyExecutorVerifier) WriteBatchToStream(batchNumber uint64, hdb *hermez_db.HermezDbReader, roTx kv.Tx) error {
	blks, err := hdb.GetL2BlockNosByBatch(batchNumber)
	if err != nil {
		return err
	}

	if err := server.WriteBlocksToStream(roTx, hdb, v.streamServer, v.stream, blks[0], blks[len(blks)-1], "verifier"); err != nil {
		return err
	}
	return nil
}

func (v *LegacyExecutorVerifier) getNextOnlineAvailableExecutor() ILegacyExecutor {
	var exec ILegacyExecutor

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

func (v *LegacyExecutorVerifier) getStreamBytes(request *VerifierRequest, tx kv.Tx, blocks []uint64, hermezDb *hermez_db.HermezDbReader, l1InfoTreeMinTimestamps map[uint64]uint64) ([]byte, error) {
	lastBlock, err := rawdb.ReadBlockByNumber(tx, blocks[0]-1)
	if err != nil {
		return nil, err
	}
	var streamBytes []byte

	// as we only ever use the executor verifier for whole batches we can safely assume that the previous batch
	// will always be the request batch - 1 and that the first block in the batch will be at the batch
	// boundary so we will always add in the batch bookmark to the stream
	previousBatch := request.BatchNumber - 1

	for _, blockNumber := range blocks {
		block, err := rawdb.ReadBlockByNumber(tx, blockNumber)
		if err != nil {
			return nil, err
		}

		//TODO: get ger updates between blocks
		gerUpdates := []dstypes.GerUpdate{}

		sBytes, err := v.streamServer.CreateAndBuildStreamEntryBytes(block, hermezDb, lastBlock, request.BatchNumber, previousBatch, true, &gerUpdates, l1InfoTreeMinTimestamps)
		if err != nil {
			return nil, err
		}
		streamBytes = append(streamBytes, sBytes...)
		lastBlock = block
		// we only put in the batch bookmark at the start of the stream data once
		previousBatch = request.BatchNumber
	}

	return streamBytes, nil
}
