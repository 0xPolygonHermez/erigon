package legacy_executor_verifier

import (
	"github.com/ledgerwatch/erigon-lib/common"
	"sync"
)

const (
	maximumInflightRequests = 1024 // todo [zkevm] this should probably be from config
)

type VerifierRequest struct {
	BatchNumber uint64
}

type VerifierResponse struct {
	BatchNumber uint64
	Valid       bool
}

type ILegacyExecutor interface {
	Verify(*Payload, *common.Hash) (bool, error)
}

type LegacyExecutorVerifier struct {
	executors     []ILegacyExecutor
	executorLocks []*sync.Mutex
	available     *sync.Cond

	requestChan   chan VerifierRequest
	responseChan  chan VerifierResponse
	responses     []VerifierResponse
	responseMutex *sync.Mutex
	quit          chan struct{}
}

func NewLegacyExecutorVerifier(executors []ILegacyExecutor) *LegacyExecutorVerifier {
	executorLocks := make([]*sync.Mutex, len(executors))
	for i := range executorLocks {
		executorLocks[i] = &sync.Mutex{}
	}

	availableLock := sync.Mutex{}
	verifier := &LegacyExecutorVerifier{
		executors:     executors,
		executorLocks: executorLocks,
		available:     sync.NewCond(&availableLock),
		requestChan:   make(chan VerifierRequest, maximumInflightRequests),
		responseChan:  make(chan VerifierResponse, maximumInflightRequests),
		responses:     make([]VerifierResponse, 0),
		responseMutex: &sync.Mutex{},
		quit:          make(chan struct{}),
	}

	return verifier
}

func (v *LegacyExecutorVerifier) StopWork() {
	close(v.quit)
}

func (v *LegacyExecutorVerifier) StartWork() {
	go func() {
	LOOP:
		for {
			select {
			case <-v.quit:
				break LOOP
			case request := <-v.requestChan:
				go v.handleRequest(request)
			case response := <-v.responseChan:
				v.handleResponse(response)
			}
		}
	}()
}

func (v *LegacyExecutorVerifier) handleRequest(request VerifierRequest) {
	// todo [zkevm] actually do some work
	response := VerifierResponse{
		BatchNumber: request.BatchNumber,
		Valid:       true,
	}
	v.responseChan <- response
}

func (v *LegacyExecutorVerifier) handleResponse(response VerifierResponse) {
	v.responseMutex.Lock()
	defer v.responseMutex.Unlock()
	v.responses = append(v.responses, response)
}

func (v *LegacyExecutorVerifier) AddRequest(request VerifierRequest) {
	v.requestChan <- request
}

func (v *LegacyExecutorVerifier) GetAllResponses() []VerifierResponse {
	v.responseMutex.Lock()
	defer v.responseMutex.Unlock()
	result := make([]VerifierResponse, len(v.responses))
	copy(result, v.responses)
	return result
}

func (v *LegacyExecutorVerifier) RemoveResponse(batchNumber uint64) {
	v.responseMutex.Lock()
	defer v.responseMutex.Unlock()

	for index, response := range v.responses {
		if response.BatchNumber == batchNumber {
			v.responses = append(v.responses[:index], v.responses[index+1:]...)
			break
		}
	}
}
