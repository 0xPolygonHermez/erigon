package nonce_cache

import (
	"fmt"
	"sync"

	"github.com/ledgerwatch/erigon-lib/common"
)

type NonceCache struct {
	highestNoncesBySender map[common.Address]uint64
	mu                    sync.RWMutex
}

func NewNonceCache() *NonceCache {
	return &NonceCache{
		highestNoncesBySender: make(map[common.Address]uint64),
		mu:                    sync.RWMutex{},
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
		fmt.Println("[sequencer - yield] setting nonce for sender", sender, nonce)
		nc.highestNoncesBySender[sender] = nonce
		return true
	}
	return false
}
