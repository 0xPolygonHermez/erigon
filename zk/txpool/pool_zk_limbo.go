package txpool

import (
	"github.com/gateway-fm/cdk-erigon-lib/common"
	"github.com/gateway-fm/cdk-erigon-lib/types"
	"github.com/ledgerwatch/erigon/zk/legacy_executor_verifier/proto/github.com/0xPolygonHermez/zkevm-node/state/runtime/executor"
)

type LimboBatchDetails struct {
	Witness                 []byte
	StreamBytes             [][]byte
	L1InfoTreeMinTimestamps map[uint64]uint64
	TimestampLimit          uint64
	FirstBlockNumber        uint64
	BatchNumber             uint64
	ForkId                  uint64
	Root                    common.Hash
	BadTransactionsHashes   []common.Hash
	BadTransactionsRLP      [][]byte
	ExecutorResponse        *executor.ProcessBatchResponseV2
}

const limboStatusUnknown = 1
const limboStatusValid = 2
const limboStatusInvalid = 3

func (p *TxPool) NewLimboBatchDetails(details LimboBatchDetails) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.limboBatches = append(p.limboBatches, details)

	/*
		as we know we're about to enter an unwind we need to ensure that all the transactions have been
		handled after the unwind by the call to OnNewBlock before we can start yielding again.  There
		is a risk that in the small window of time between this call and the next call to yield
		by the stage loop a TX with a nonce too high will be yielded and cause an error during execution

		potential dragons here as if the OnNewBlock is never called the call to yield will always return empty
	*/
	p.awaitingBlockHandling.Store(true)
}

func (p *TxPool) GetLimboDetails() []LimboBatchDetails {
	p.lock.Lock()
	defer p.lock.Unlock()
	return p.limboBatches
}

func (p *TxPool) GetLimboDetailsCloned() []LimboBatchDetails {
	p.lock.Lock()
	defer p.lock.Unlock()

	limboBatchesClone := make([]LimboBatchDetails, len(p.limboBatches))
	copy(limboBatchesClone, p.limboBatches)
	return limboBatchesClone
}

func (p *TxPool) MarkProcessedLimboDetails(size int, validTxs, invalidTxs []*string) {
	p.lock.Lock()
	defer p.lock.Unlock()

	// for _, idHash := range validTxs {
	// 	p.limboStatusMap[*idHash] = limboStatusValid
	// }
	// for _, idHash := range invalidTxs {
	// 	p.limboStatusMap[*idHash] = limboStatusInvalid
	// }

	// invalidMts := []*metaTx{}

	// for i := 0; i < len(p.limbo.best.ms); i++ {
	// 	mt := p.limbo.best.ms[i]
	// 	idHash := hexutils.BytesToHex(mt.Tx.IDHash[:])
	// 	_, found := invalidTxMap[idHash]
	// 	if found {
	// 		invalidMts = append(invalidMts, mt)
	// 	}
	// }

	// for _, mt := range invalidMts {
	// 	p.limbo.Remove(mt)
	// }

	p.limboBatches = p.limboBatches[size:]
}

// should be called from within a locked context from the pool
// func (p *TxPool) getValidSplitsFromLimbo() (types.TxSlots, types.TxSlots) {
// 	validTxs := types.TxSlots{}
// 	invalidTxs := types.TxSlots{}

// 	for idx, slot := range p.limboSlots.Txs {
// 		idHash := hexutils.BytesToHex(slot.IDHash[:])
// 		status, found := p.limboStatusMap[idHash]
// 		if !found {
// 			panic("DA BE")
// 		}
// 		if status == limboStatusValid {
// 			validTxs.Append(slot, p.limboSlots.Senders.At(idx), p.limboSlots.IsLocal[idx])
// 		} else if status == limboStatusInvalid {
// 			invalidTxs.Append(slot, p.limboSlots.Senders.At(idx), p.limboSlots.IsLocal[idx])
// 		}
// 	}

// 	return validTxs, invalidTxs
// }

// should be called from within a locked context from the pool
func (p *TxPool) trimSlotsBasedOnLimbo(slots types.TxSlots) (trimmed types.TxSlots, limbo types.TxSlots) {
	if len(p.limboBatches) > 0 {
		// iterate over the unwind transactions removing any that appear in the limbo list
		for idx, slot := range slots.Txs {
			if p.isTxKnownToLimbo(slot.IDHash) {
				limbo.Append(slot, slots.Senders.At(idx), slots.IsLocal[idx])
			} else {
				trimmed.Append(slot, slots.Senders.At(idx), slots.IsLocal[idx])
			}
		}
		return trimmed, limbo
	} else {
		return slots, types.TxSlots{}
	}
}

// should be called from within a locked context from the pool
func (p *TxPool) isTxKnownToLimbo(hash common.Hash) bool {
	for _, limbo := range p.limboBatches {
		for _, txHash := range limbo.BadTransactionsHashes {
			if txHash == hash {
				return true
			}
		}
	}
	return false
}

// func (p *TxPool) appendLimboTransactions(slots types.TxSlots, blockNum uint64) {
// 	for idx, slot := range slots.Txs {
// 		idHash := hexutils.BytesToHex(slot.IDHash[:])
// 		_, found := p.limboStatusMap[idHash]
// 		if !found {
// 			p.limboStatusMap[idHash] = limboStatusUnknown
// 			p.limboSlots.Append(slot, slots.Senders.At(idx), slots.IsLocal[idx])
// 		}

// 		// mt := newMetaTx(slot, slots.IsLocal[idx], blockNum)
// 		// p.limbo.Add(mt)
// 	}
// }
