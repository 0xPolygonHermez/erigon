package legacy_executor_verifier

import (
	"github.com/ledgerwatch/erigon-lib/common"
	"sync"
)

type ILegacyExecutor interface {
	Verify(*Payload, *common.Hash) (bool, error)
}

type LegacyExecutorVerifier struct {
	executors     []ILegacyExecutor
	executorLocks []*sync.Mutex
	available     *sync.Cond
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
	}

	return verifier
}

func (v *LegacyExecutorVerifier) VerifyWithAvailableExecutor(p *Payload, expectedStateRoot *common.Hash) (bool, error) {
	for {
		for i, executor := range v.executors {
			if v.executorLocks[i].TryLock() {
				result, err := executor.Verify(p, expectedStateRoot)
				v.executorLocks[i].Unlock()
				v.available.Broadcast()
				return result, err
			}
		}

		v.available.L.Lock()
		v.available.Wait()
		v.available.L.Unlock()
	}
}
