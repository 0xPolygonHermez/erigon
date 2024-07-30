package stages

// import (
// 	"errors"
// 	"fmt"
// 	"sync"

// 	"github.com/gateway-fm/cdk-erigon-lib/common"
// 	"github.com/ledgerwatch/erigon/eth/ethconfig"
// 	verifier "github.com/ledgerwatch/erigon/zk/legacy_executor_verifier"
// 	"github.com/ledgerwatch/log/v3"
// )

// type BatchVerifier struct {
// 	cfg            *ethconfig.Zk
// 	legacyVerifier *verifier.LegacyExecutorVerifier
// 	hasExecutor    bool
// 	forkId         uint64
// 	promises       []*verifier.Promise[*verifier.VerifierBundle]
// 	mtxPromises    *sync.Mutex
// 	// stop           bool
// 	// errors         chan error
// 	// finishCond *sync.Cond
// }

// func NewBatchVerifier(
// 	cfg *ethconfig.Zk,
// 	hasExecutors bool,
// 	legacyVerifier *verifier.LegacyExecutorVerifier,
// 	forkId uint64,
// ) *BatchVerifier {
// 	return &BatchVerifier{
// 		cfg:            cfg,
// 		hasExecutor:    hasExecutors,
// 		legacyVerifier: legacyVerifier,
// 		forkId:         forkId,
// 		mtxPromises:    &sync.Mutex{},
// 		promises:       make([]*verifier.Promise[*verifier.VerifierBundle], 0),
// 		// errors:         make(chan error),
// 		// finishCond: sync.NewCond(&sync.Mutex{}),
// 	}
// }

// func (bv *BatchVerifier) StartAsyncVerification(
// 	batchNumber uint64,
// 	blockNumber uint64,
// 	stateRoot common.Hash,
// 	counters map[string]int,
// 	blockNumbers []uint64,
// ) {
// 	request := verifier.NewVerifierRequest(batchNumber, blockNumber, bv.forkId, stateRoot, counters)

// 	var promise *verifier.Promise[*verifier.VerifierBundle]
// 	if bv.hasExecutor {
// 		promise = bv.verifyWithExecutor(request, blockNumbers)
// 	} else {
// 		promise = bv.verifyWithoutExecutor(request, blockNumbers)
// 	}

// 	bv.appendPromise(promise)
// }

// func (bv *BatchVerifier) CheckProgress() ([]*verifier.VerifierBundle, int, error) {
// 	bv.mtxPromises.Lock()
// 	defer bv.mtxPromises.Unlock()

// 	var verifierResponse []*verifier.VerifierBundle

// 	// not a stop signal, so we can start to process our promises now
// 	for idx, promise := range bv.promises {
// 		verifierBundle, err := promise.TryGet()
// 		if verifierBundle == nil && err == nil {
// 			// If code enters here this means that this promise is not yet completed
// 			// We must processes responses sequentially so if this one is not ready we can just break
// 			break
// 		}

// 		if err != nil {
// 			// let leave it for debug purposes
// 			// a cancelled promise is removed from v.promises => it should never appear here, that's why let's panic if it happens, because it will indicate for massive error
// 			if errors.Is(err, verifier.ErrPromiseCancelled) {
// 				panic("this should never happen")
// 			}

// 			log.Error("error on our end while preparing the verification request, re-queueing the task", "err", err)

// 			if verifierBundle.Request.IsOverdue() {
// 				// signal an error, the caller can check on this and stop the process if needs be
// 				return nil, 0, fmt.Errorf("error: batch %d couldn't be processed in 30 minutes", verifierBundle.Request.BatchNumber)
// 			}

// 			// re-queue the task - it should be safe to replace the index of the slice here as we only add to it
// 			bv.promises[idx] = promise.CloneAndRerun()

// 			// break now as we know we can't proceed here until this promise is attempted again
// 			break
// 		}

// 		verifierResponse = append(verifierResponse, verifierBundle)
// 	}

// 	// remove processed promises from the list
// 	bv.promises = bv.promises[len(verifierResponse):]

// 	return verifierResponse, len(bv.promises), nil
// }

// // func (bv *BatchVerifier) CancelAllRequestsUnsafe() {
// // 	bv.mtxPromises.Lock()
// // 	defer bv.mtxPromises.Unlock()

// // 	// cancel all promises
// // 	// all queued promises will return ErrPromiseCancelled while getting its result
// // 	for _, p := range bv.promises {
// // 		p.Cancel()
// // 	}

// // 	// the goal of this car is to ensure that running promises are stopped as soon as possible
// // 	// we need it because the promise's function must finish and then the promise checks if it has been cancelled
// // 	bv.legacyVerifier.cancelAllVerifications.Store(true)

// // 	for _, e := range bv.legacyVerifier.executors {
// // 		// let's wait for all threads that are waiting to add to v.openRequests to finish
// // 		for e.QueueLength() > 0 {
// // 			time.Sleep(1 * time.Millisecond)
// // 		}
// // 	}

// // 	bv.legacyVerifier.cancelAllVerifications.Store(false)

// // 	bv.promises = make([]*verifier.Promise[*verifier.VerifierBundle], 0)
// // }

// // func (bv *BatchVerifier) WaitForFinish() {
// // 	count := 0
// // 	bv.mtxPromises.Lock()
// // 	count = len(bv.promises)
// // 	bv.mtxPromises.Unlock()

// // 	if count > 0 {
// // 		bv.finishCond.L.Lock()
// // 		bv.finishCond.Wait()
// // 		bv.finishCond.L.Unlock()
// // 	}
// // }

// func (bv *BatchVerifier) appendPromise(promise *verifier.Promise[*verifier.VerifierBundle]) {
// 	bv.mtxPromises.Lock()
// 	defer bv.mtxPromises.Unlock()
// 	bv.promises = append(bv.promises, promise)
// }

// func (bv *BatchVerifier) verifyWithoutExecutor(request *verifier.VerifierRequest, blockNumbers []uint64) *verifier.Promise[*verifier.VerifierBundle] {
// 	valid := true
// 	// simulate a die roll to determine if this is a good batch or not
// 	// 1 in 6 chance of being a bad batch
// 	// if rand.Intn(6) == 0 {
// 	// 	valid = false
// 	// }

// 	promise := verifier.NewPromise[*verifier.VerifierBundle](func() (*verifier.VerifierBundle, error) {
// 		response := &verifier.VerifierResponse{
// 			BatchNumber:      request.BatchNumber,
// 			BlockNumber:      request.BlockNumber,
// 			Valid:            valid,
// 			OriginalCounters: request.Counters,
// 			Witness:          nil,
// 			ExecutorResponse: nil,
// 			Error:            nil,
// 		}
// 		return verifier.NewVerifierBundle(request, response), nil
// 	})
// 	promise.Wait()

// 	return promise
// }

// func (bv *BatchVerifier) verifyWithExecutor(request *verifier.VerifierRequest, blockNumbers []uint64) *verifier.Promise[*verifier.VerifierBundle] {
// 	return bv.legacyVerifier.VerifyAsync(request, blockNumbers)
// }
