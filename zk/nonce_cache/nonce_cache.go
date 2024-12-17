package nonce_cache

import (
	"sync"

	"github.com/ledgerwatch/erigon-lib/common"
)

const MaxNonceCacheSize = 1000

type NonceCache struct {
	highestNoncesBySender map[common.Address]uint64
	mu                    sync.RWMutex
	maxSize               uint64
}

func NewNonceCache(cacheSize uint64) *NonceCache {
	return &NonceCache{
		highestNoncesBySender: make(map[common.Address]uint64),
		mu:                    sync.RWMutex{},
		maxSize:               cacheSize,
	}
}

func (nc *NonceCache) GetHighestNonceForSender(sender common.Address) (uint64, bool) {
	nc.mu.RLock()
	defer nc.mu.RUnlock()
	nonce, ok := nc.highestNoncesBySender[sender]
	return nonce, ok
}

func (nc *NonceCache) TrySetHighestNonceForSender(sender common.Address, nonce uint64) bool {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	if nc.highestNoncesBySender[sender] < nonce {
		nc.highestNoncesBySender[sender] = nonce
		if uint64(len(nc.highestNoncesBySender)) > nc.maxSize {
			var oldestSender common.Address
			var oldestNonce uint64
			for s, n := range nc.highestNoncesBySender {
				if oldestNonce == 0 || n < oldestNonce {
					oldestSender = s
					oldestNonce = n
				}
			}
			delete(nc.highestNoncesBySender, oldestSender)
		}

		return true
	}

	return false
}

func (nc *NonceCache) CacheSize() int {
	nc.mu.RLock()
	defer nc.mu.RUnlock()
	return len(nc.highestNoncesBySender)
}
