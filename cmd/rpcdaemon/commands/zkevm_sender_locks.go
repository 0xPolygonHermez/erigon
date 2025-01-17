package commands

import (
	"sync"

	"github.com/gateway-fm/cdk-erigon-lib/common"
)

// SenderLock is a map of sender addresses to the number of locks they have
// This is used to ensure that any calls for an account nonce will wait until all
// pending transactions for that account have been processed.  Without this you can
// get strange race behaviour where a nonce will come back too low if the pool is taking
// a long time to process a transaction.
type SenderLock struct {
	mtx   sync.Mutex
	locks map[common.Address]uint64
}

func NewSenderLock() *SenderLock {
	return &SenderLock{
		locks: make(map[common.Address]uint64),
	}
}

func (sl *SenderLock) GetLock(sender common.Address) uint64 {
	sl.mtx.Lock()
	defer sl.mtx.Unlock()
	if _, ok := sl.locks[sender]; !ok {
		return 0
	}
	return sl.locks[sender]
}

func (sl *SenderLock) ReleaseLock(sender common.Address) {
	sl.mtx.Lock()
	defer sl.mtx.Unlock()
	if current, ok := sl.locks[sender]; ok {
		if current == 0 {
			delete(sl.locks, sender)
		}
		sl.locks[sender] = current - 1
		if sl.locks[sender] == 0 {
			delete(sl.locks, sender)
		}
	}
}

func (sl *SenderLock) AddLock(sender common.Address) {
	sl.mtx.Lock()
	defer sl.mtx.Unlock()
	if _, ok := sl.locks[sender]; !ok {
		sl.locks[sender] = 1
	} else {
		sl.locks[sender] = sl.locks[sender] + 1
	}
}
