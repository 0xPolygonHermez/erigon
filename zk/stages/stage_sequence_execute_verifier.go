package stages

import (
	"errors"
	"fmt"
	"math/rand"
	"sync"

	"github.com/gateway-fm/cdk-erigon-lib/common"
	"github.com/ledgerwatch/erigon/eth/ethconfig"
	verifier "github.com/ledgerwatch/erigon/zk/legacy_executor_verifier"
	"github.com/ledgerwatch/log/v3"
)

type PromiseWithBlocks struct {
	Promise *verifier.Promise[*verifier.VerifierBundleWithBlocks]
	Blocks  []uint64
}

type BatchVerifier struct {
	cfg            *ethconfig.Zk
	legacyVerifier *verifier.LegacyExecutorVerifier
	hasExecutor    bool
	forkId         uint64
	mtxPromises    *sync.Mutex
	promises       []*PromiseWithBlocks
	stop           bool
	errors         chan error
	finishCond     *sync.Cond
}

func NewBatchVerifier(
	cfg *ethconfig.Zk,
	hasExecutors bool,
	legacyVerifier *verifier.LegacyExecutorVerifier,
	forkId uint64,
) *BatchVerifier {
	return &BatchVerifier{
		cfg:            cfg,
		hasExecutor:    hasExecutors,
		legacyVerifier: legacyVerifier,
		forkId:         forkId,
		mtxPromises:    &sync.Mutex{},
		promises:       make([]*PromiseWithBlocks, 0),
		errors:         make(chan error),
		finishCond:     sync.NewCond(&sync.Mutex{}),
	}
}

func (bv *BatchVerifier) AddNewCheck(
	batchNumber uint64,
	blockNumber uint64,
	stateRoot common.Hash,
	counters map[string]int,
	blockNumbers []uint64,
) {
	request := verifier.NewVerifierRequest(batchNumber, blockNumber, bv.forkId, stateRoot, counters)

	var promise *PromiseWithBlocks
	if bv.hasExecutor {
		promise = bv.asyncPromise(request, blockNumbers)
	} else {
		promise = bv.syncPromise(request, blockNumbers)
	}

	bv.appendPromise(promise)
}

func (bv *BatchVerifier) WaitForFinish() {
	count := 0
	bv.mtxPromises.Lock()
	count = len(bv.promises)
	bv.mtxPromises.Unlock()

	if count > 0 {
		bv.finishCond.L.Lock()
		bv.finishCond.Wait()
		bv.finishCond.L.Unlock()
	}
}

func (bv *BatchVerifier) appendPromise(promise *PromiseWithBlocks) {
	bv.mtxPromises.Lock()
	defer bv.mtxPromises.Unlock()
	bv.promises = append(bv.promises, promise)
}

func (bv *BatchVerifier) CheckProgress() ([]*verifier.VerifierBundle, int, error) {
	bv.mtxPromises.Lock()
	defer bv.mtxPromises.Unlock()

	var responses []*verifier.VerifierBundle

	// not a stop signal, so we can start to process our promises now
	processed := 0
	for idx, promise := range bv.promises {
		bundleWithBlocks, err := promise.Promise.TryGet()
		if bundleWithBlocks == nil && err == nil {
			// nothing to process in this promise so we skip it
			break
		}

		if err != nil {
			// let leave it for debug purposes
			// a cancelled promise is removed from v.promises => it should never appear here, that's why let's panic if it happens, because it will indicate for massive error
			if errors.Is(err, verifier.ErrPromiseCancelled) {
				panic("this should never happen")
			}

			log.Error("error on our end while preparing the verification request, re-queueing the task", "err", err)

			if bundleWithBlocks == nil {
				// we can't proceed here until this promise is attempted again
				break
			}

			if bundleWithBlocks.Bundle.Request.IsOverdue() {
				// signal an error, the caller can check on this and stop the process if needs be
				return nil, 0, fmt.Errorf("error: batch %d couldn't be processed in 30 minutes", bundleWithBlocks.Bundle.Request.BatchNumber)
			}

			// re-queue the task - it should be safe to replace the index of the slice here as we only add to it
			if bv.hasExecutor {
				prom := bv.asyncPromise(bundleWithBlocks.Bundle.Request, bundleWithBlocks.Blocks)
				bv.promises[idx] = prom
			} else {
				prom := bv.syncPromise(bundleWithBlocks.Bundle.Request, bundleWithBlocks.Blocks)
				bv.promises[idx] = prom
			}

			// break now as we know we can't proceed here until this promise is attempted again
			break
		}

		processed++
		responses = append(responses, bundleWithBlocks.Bundle)
	}

	// remove processed promises from the list
	remaining := bv.removeProcessedPromises(processed)

	return responses, remaining, nil
}

func (bv *BatchVerifier) removeProcessedPromises(processed int) int {
	count := len(bv.promises)

	if processed == 0 {
		return count
	}

	if processed == len(bv.promises) {
		bv.promises = make([]*PromiseWithBlocks, 0)
		return 0
	}

	bv.promises = bv.promises[processed:]

	return len(bv.promises)
}

func (bv *BatchVerifier) syncPromise(request *verifier.VerifierRequest, blockNumbers []uint64) *PromiseWithBlocks {
	// simulate a die roll to determine if this is a good batch or not
	// 1 in 6 chance of being a bad batch
	valid := true
	if rand.Intn(6) == 0 {
		valid = false
	}

	promise := verifier.NewPromiseSync[*verifier.VerifierBundleWithBlocks](func() (*verifier.VerifierBundleWithBlocks, error) {
		response := &verifier.VerifierResponse{
			BatchNumber:      request.BatchNumber,
			BlockNumber:      request.BlockNumber,
			Valid:            valid,
			OriginalCounters: request.Counters,
			Witness:          nil,
			ExecutorResponse: nil,
			Error:            nil,
		}
		bundle := verifier.NewVerifierBundle(request, response)
		return &verifier.VerifierBundleWithBlocks{Blocks: blockNumbers, Bundle: bundle}, nil
	})

	return &PromiseWithBlocks{Blocks: blockNumbers, Promise: promise}
}

func (bv *BatchVerifier) asyncPromise(request *verifier.VerifierRequest, blockNumbers []uint64) *PromiseWithBlocks {
	promise := bv.legacyVerifier.CreateAsyncPromise(request, blockNumbers)

	return &PromiseWithBlocks{Blocks: blockNumbers, Promise: promise}
}
