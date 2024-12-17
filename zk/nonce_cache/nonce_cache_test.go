package nonce_cache

import (
	"github.com/ledgerwatch/erigon-lib/common"
	"testing"
)

const testCacheSize = 4

func Test_NonceCacheSize(t *testing.T) {
	scenarios := map[string]struct {
		setupActions   []func(*NonceCache)
		expectedSize   int
		expectedExists map[string]bool
	}{
		"Initial state with 4 entries": {
			setupActions: []func(*NonceCache){
				func(nc *NonceCache) { nc.TrySetHighestNonceForSender(common.HexToAddress("0x1"), 1) },
				func(nc *NonceCache) { nc.TrySetHighestNonceForSender(common.HexToAddress("0x2"), 2) },
				func(nc *NonceCache) { nc.TrySetHighestNonceForSender(common.HexToAddress("0x3"), 3) },
				func(nc *NonceCache) { nc.TrySetHighestNonceForSender(common.HexToAddress("0x4"), 4) },
			},
			expectedSize: 4,
			expectedExists: map[string]bool{
				"0x1": true, "0x2": true, "0x3": true, "0x4": true,
			},
		},
		"Evict when adding 5th entry": {
			setupActions: []func(*NonceCache){
				func(nc *NonceCache) { nc.TrySetHighestNonceForSender(common.HexToAddress("0x1"), 1) },
				func(nc *NonceCache) { nc.TrySetHighestNonceForSender(common.HexToAddress("0x2"), 2) },
				func(nc *NonceCache) { nc.TrySetHighestNonceForSender(common.HexToAddress("0x3"), 3) },
				func(nc *NonceCache) { nc.TrySetHighestNonceForSender(common.HexToAddress("0x4"), 4) },
				func(nc *NonceCache) { nc.TrySetHighestNonceForSender(common.HexToAddress("0x5"), 5) },
			},
			expectedSize: testCacheSize,
			expectedExists: map[string]bool{
				"0x1": false, "0x2": true, "0x3": true, "0x4": true, "0x5": true,
			},
		},
		"Try write loads of entries": {
			setupActions: []func(*NonceCache){
				func(nc *NonceCache) { nc.TrySetHighestNonceForSender(common.HexToAddress("0x1"), 1) },
				func(nc *NonceCache) { nc.TrySetHighestNonceForSender(common.HexToAddress("0x2"), 2) },
				func(nc *NonceCache) { nc.TrySetHighestNonceForSender(common.HexToAddress("0x3"), 3) },
				func(nc *NonceCache) { nc.TrySetHighestNonceForSender(common.HexToAddress("0x4"), 4) },
				func(nc *NonceCache) { nc.TrySetHighestNonceForSender(common.HexToAddress("0x5"), 5) },
				func(nc *NonceCache) { nc.TrySetHighestNonceForSender(common.HexToAddress("0x6"), 6) },
				func(nc *NonceCache) { nc.TrySetHighestNonceForSender(common.HexToAddress("0x7"), 7) },
				func(nc *NonceCache) { nc.TrySetHighestNonceForSender(common.HexToAddress("0x8"), 8) },
			},
			expectedSize: testCacheSize,
			expectedExists: map[string]bool{
				"0x1": false, "0x2": false, "0x3": false, "0x4": false, "0x5": true, "0x6": true, "0x7": true, "0x8": true,
			},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			cache := NewNonceCache(testCacheSize)

			for _, action := range scenario.setupActions {
				action(cache)
			}

			if size := cache.CacheSize(); size != scenario.expectedSize {
				t.Errorf("Expected cache size %d, got %d", scenario.expectedSize, size)
			}

			for addr, shouldExist := range scenario.expectedExists {
				_, exists := cache.GetHighestNonceForSender(common.HexToAddress(addr))
				if exists != shouldExist {
					t.Errorf("Expected sender %s to exist: %t, got %t", addr, shouldExist, exists)
				}
			}
		})
	}
}
