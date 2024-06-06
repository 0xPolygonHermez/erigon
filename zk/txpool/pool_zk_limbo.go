package txpool

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/gateway-fm/cdk-erigon-lib/common"
	"github.com/gateway-fm/cdk-erigon-lib/kv"
	"github.com/gateway-fm/cdk-erigon-lib/kv/kvcache"
	"github.com/gateway-fm/cdk-erigon-lib/types"
	"github.com/status-im/keycard-go/hexutils"
)

const (
	TablePoolLimbo                   = "PoolLimbo"
	DbKeyInvalidTxPrefix             = uint8(1)
	DbKeySlotsPrefix                 = uint8(2)
	DbKeyBatchesPrefix               = uint8(3)
	DbKeyAwaitingBlockHandlingPrefix = uint8(4)

	DbKeyBatchesWitnessPrefix          = uint8(1)
	DbKeyBatchesStreamBytesPrefix      = uint8(2)
	DbKeyBatchesL1InfoTreePrefix       = uint8(3)
	DbKeyBatchesTimestampLimitPrefix   = uint8(4)
	DbKeyBatchesFirstBlockNumberPrefix = uint8(5)
	DbKeyBatchesBatchNumberPrefix      = uint8(6)
	DbKeyBatchesForkIdPrefix           = uint8(7)
	DbKeyBatchesRootPrefix             = uint8(8)
	DbKeyBatchesBadTransactionsPrefix  = uint8(9)
)

var emptyHash = common.Hash{}

type LimboBatchTransactionDetails struct {
	Rlp         []byte
	StreamBytes []byte
	Root        common.Hash
	Hash        common.Hash
	Sender      common.Address
	PreviousTx  int
}

func newLimboBatchTransactionDetails(rlp, streamBytes []byte, hash common.Hash, sender common.Address, previousTx int) *LimboBatchTransactionDetails {
	return &LimboBatchTransactionDetails{
		Rlp:         rlp,
		StreamBytes: streamBytes,
		Root:        common.Hash{},
		Hash:        hash,
		Sender:      sender,
		PreviousTx:  previousTx,
	}
}

func (_this *LimboBatchTransactionDetails) hasRoot() bool {
	return _this.Root != emptyHash
}

type Limbo struct {
	invalidTxsMap map[string]uint8 //invalid tx: hash -> handled
	limboSlots    *types.TxSlots
	limboBatches  []*LimboBatchDetails

	// used to denote some process has made the pool aware that an unwind is about to occur and to wait
	// until the unwind has been processed before allowing yielding of transactions again
	awaitingBlockHandling atomic.Bool
}

func newLimbo() *Limbo {
	return &Limbo{
		invalidTxsMap:         make(map[string]uint8),
		limboSlots:            &types.TxSlots{},
		limboBatches:          make([]*LimboBatchDetails, 0),
		awaitingBlockHandling: atomic.Bool{},
	}
}

func (_this *Limbo) resizeBatches(newSize int) {
	for i := len(_this.limboBatches); i < newSize; i++ {
		_this.limboBatches = append(_this.limboBatches, NewLimboBatchDetails())
	}
}

func (_this *Limbo) getFirstTxWithoutRootByBatch(batchNumber uint64) (*LimboBatchDetails, *LimboBatchTransactionDetails) {
	for _, limboBatch := range _this.limboBatches {
		for _, limboTx := range limboBatch.Transactions {
			if !limboTx.hasRoot() {
				if batchNumber < limboBatch.BatchNumber {
					return nil, nil
				}
				if batchNumber > limboBatch.BatchNumber {
					panic(fmt.Errorf("requested batch %d while the network is already on %d", limboBatch.BatchNumber, batchNumber))
				}

				return limboBatch, limboTx
			}
		}
	}

	return nil, nil
}

func (_this *Limbo) getLimboTxDetailsByTxHash(txHash *common.Hash) (*LimboBatchDetails, *LimboBatchTransactionDetails, int) {
	for _, limboBatch := range _this.limboBatches {
		limboTx, i := limboBatch.getTxDetailsByHash(txHash)
		if limboTx != nil {
			return limboBatch, limboTx, i
		}
	}

	return nil, nil, -1
}

type LimboBatchDetails struct {
	Witness                 []byte
	L1InfoTreeMinTimestamps map[uint64]uint64
	TimestampLimit          uint64
	FirstBlockNumber        uint64
	BatchNumber             uint64
	ForkId                  uint64
	Transactions            []*LimboBatchTransactionDetails
}

func NewLimboBatchDetails() *LimboBatchDetails {
	return &LimboBatchDetails{
		L1InfoTreeMinTimestamps: make(map[uint64]uint64),
		Transactions:            make([]*LimboBatchTransactionDetails, 0),
	}
}

func (_this *LimboBatchDetails) AppendTransaction(rlp, streamBytes []byte, hash common.Hash, sender common.Address, previousTx int) int {
	_this.Transactions = append(_this.Transactions, newLimboBatchTransactionDetails(rlp, streamBytes, hash, sender, previousTx))
	return len(_this.Transactions)
}

func (_this *LimboBatchDetails) getTxDetailsByHash(txHash *common.Hash) (*LimboBatchTransactionDetails, int) {
	for i, limboTx := range _this.Transactions {
		if limboTx.Hash == *txHash {
			return limboTx, i
		}
	}

	return nil, -1
}

func (p *TxPool) GetLimboTxHash(batchNumber uint64) (uint64, *common.Hash) {
	p.lock.Lock()
	defer p.lock.Unlock()

	limboBatch, limboTx := p.limbo.getFirstTxWithoutRootByBatch(batchNumber)
	if limboBatch == nil {
		return 0, nil
	}
	return limboBatch.TimestampLimit, &limboTx.Hash
}

func (p *TxPool) GetLimboTxRplsByHash(tx kv.Tx, txHash *common.Hash) (*types.TxsRlp, error) {
	p.lock.Lock()
	defer p.lock.Unlock()

	limboBatch, _, txIndex := p.limbo.getLimboTxDetailsByTxHash(txHash)
	if limboBatch == nil {
		return nil, fmt.Errorf("missing transaction")
	}

	maxSize := len(limboBatch.Transactions)
	txIndices := make([]int, 0, maxSize)

	for {
		if txIndex == -1 {
			break
		}

		txIndices = append(txIndices, txIndex)
		txIndex = limboBatch.Transactions[txIndex].PreviousTx
	}

	txsRlps := &types.TxsRlp{}
	txsRlps.Resize(uint(len(txIndices)))

	txIndicesSize := len(txIndices)
	for i, txIndex := range txIndices {
		reverseIndex := txIndicesSize - 1 - i
		limboTx := limboBatch.Transactions[txIndex]
		txsRlps.Txs[reverseIndex] = limboTx.Rlp
		copy(txsRlps.Senders.At(reverseIndex), limboTx.Sender[:])
		txsRlps.IsLocal[reverseIndex] = true // all limbo tx are considered local //TODO: explain better about local
	}

	return txsRlps, nil
}

func (p *TxPool) UpdateLimboRootByTxHash(txHash *common.Hash, stateRoot *common.Hash) {
	p.lock.Lock()
	defer p.lock.Unlock()

	_, limboTx, _ := p.limbo.getLimboTxDetailsByTxHash(txHash)
	limboTx.Root = *stateRoot
}

func (p *TxPool) ProcessLimboBatchDetails(details *LimboBatchDetails) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.limbo.limboBatches = append(p.limbo.limboBatches, details)

	/*
		as we know we're about to enter an unwind we need to ensure that all the transactions have been
		handled after the unwind by the call to OnNewBlock before we can start yielding again.  There
		is a risk that in the small window of time between this call and the next call to yield
		by the stage loop a TX with a nonce too high will be yielded and cause an error during execution

		potential dragons here as if the OnNewBlock is never called the call to yield will always return empty
	*/
	p.denyYieldingTransactions()
}

func (p *TxPool) GetLimboDetails() []*LimboBatchDetails {
	p.lock.Lock()
	defer p.lock.Unlock()
	return p.limbo.limboBatches
}

func (p *TxPool) GetLimboDetailsCloned() []*LimboBatchDetails {
	p.lock.Lock()
	defer p.lock.Unlock()

	limboBatchesClone := make([]*LimboBatchDetails, len(p.limbo.limboBatches))
	copy(limboBatchesClone, p.limbo.limboBatches)
	return limboBatchesClone
}

func (p *TxPool) MarkProcessedLimboDetails(size int, invalidTxs []*string) {
	p.lock.Lock()
	defer p.lock.Unlock()

	for _, idHash := range invalidTxs {
		p.limbo.invalidTxsMap[*idHash] = 0
	}

	p.limbo.limboBatches = p.limbo.limboBatches[size:]
}

// should be called from within a locked context from the pool
func (p *TxPool) addLimboToUnwindTxs(unwindTxs *types.TxSlots) {
	for idx, slot := range p.limbo.limboSlots.Txs {
		unwindTxs.Append(slot, p.limbo.limboSlots.Senders.At(idx), p.limbo.limboSlots.IsLocal[idx])
	}
}

// should be called from within a locked context from the pool
func (p *TxPool) trimLimboSlots(unwindTxs *types.TxSlots) (types.TxSlots, *types.TxSlots, *types.TxSlots) {
	resultLimboTxs := types.TxSlots{}
	resultUnwindTxs := types.TxSlots{}
	resultForDiscard := types.TxSlots{}

	hasInvalidTxs := len(p.limbo.invalidTxsMap) > 0

	for idx, slot := range unwindTxs.Txs {
		if p.isTxKnownToLimbo(slot.IDHash) {
			resultLimboTxs.Append(slot, unwindTxs.Senders.At(idx), unwindTxs.IsLocal[idx])
		} else {
			if hasInvalidTxs {
				idHash := hexutils.BytesToHex(slot.IDHash[:])
				_, ok := p.limbo.invalidTxsMap[idHash]
				if ok {
					p.limbo.invalidTxsMap[idHash] = 1
					resultForDiscard.Append(slot, unwindTxs.Senders.At(idx), unwindTxs.IsLocal[idx])
					continue
				}
			}
			resultUnwindTxs.Append(slot, unwindTxs.Senders.At(idx), unwindTxs.IsLocal[idx])
		}
	}

	return resultUnwindTxs, &resultLimboTxs, &resultForDiscard
}

// should be called from within a locked context from the pool
func (p *TxPool) finalizeLimboOnNewBlock(limboTxs *types.TxSlots) {
	p.limbo.limboSlots = limboTxs

	forDelete := make([]*string, 0, len(p.limbo.invalidTxsMap))
	for idHash, shouldDelete := range p.limbo.invalidTxsMap {
		if shouldDelete == 1 {
			forDelete = append(forDelete, &idHash)
		}
	}

	for _, idHash := range forDelete {
		delete(p.limbo.invalidTxsMap, *idHash)
	}
}

// should be called from within a locked context from the pool
func (p *TxPool) isTxKnownToLimbo(hash common.Hash) bool {
	for _, limbo := range p.limbo.limboBatches {
		for _, limboTx := range limbo.Transactions {
			if limboTx.Hash == hash {
				return true
			}
		}
	}
	return false
}

func (p *TxPool) isDeniedYieldingTransactions() bool {
	return p.limbo.awaitingBlockHandling.Load()
}

func (p *TxPool) denyYieldingTransactions() {
	p.limbo.awaitingBlockHandling.Store(true)
}

func (p *TxPool) allowYieldingTransactions() {
	p.limbo.awaitingBlockHandling.Store(false)
}

func (p *TxPool) flushLockedLimbo(tx kv.RwTx) (err error) {
	// if err := tx.CreateBucket(TablePoolLimbo); err != nil {
	// 	return err
	// }

	// if err := tx.ClearBucket(TablePoolLimbo); err != nil {
	// 	return err
	// }

	// for hash, handled := range p.limbo.invalidTxsMap {
	// 	hashAsBytes := hexutils.HexToBytes(hash)
	// 	key := append([]byte{DbKeyInvalidTxPrefix}, hashAsBytes...)
	// 	tx.Put(TablePoolLimbo, key, []byte{handled})
	// }

	// v := make([]byte, 0, 1024)
	// for _, txSlot := range p.limbo.limboSlots.Txs {
	// 	v = common.EnsureEnoughSize(v, 20+len(txSlot.Rlp))

	// 	addr, ok := p.senders.senderID2Addr[txSlot.SenderID]
	// 	if !ok {
	// 		log.Warn("[txpool] flush: sender address not found by ID", "senderID", txSlot.SenderID)
	// 		continue
	// 	}

	// 	copy(v[:20], addr.Bytes())
	// 	copy(v[20:], txSlot.Rlp)

	// 	key := append([]byte{DbKeySlotsPrefix}, txSlot.IDHash[:]...)
	// 	if err := tx.Put(TablePoolLimbo, key, v); err != nil {
	// 		return err
	// 	}
	// }

	// keyBytes := make([]byte, 14)
	// vBytes := make([]byte, 8)
	// keyBytes[0] = DbKeyBatchesPrefix

	// for i, limboBatch := range p.limbo.limboBatches {
	// 	binary.LittleEndian.PutUint32(keyBytes[1:5], uint32(i))

	// 	// Witness
	// 	keyBytes[5] = DbKeyBatchesWitnessPrefix
	// 	binary.LittleEndian.PutUint64(keyBytes[6:14], 0)
	// 	if err := tx.Put(TablePoolLimbo, keyBytes, limboBatch.Witness); err != nil {
	// 		return err
	// 	}

	// 	// StreamBytes
	// 	keyBytes[5] = DbKeyBatchesStreamBytesPrefix
	// 	for j, streamBytes := range limboBatch.StreamBytes {
	// 		binary.LittleEndian.PutUint64(keyBytes[6:14], uint64(j))
	// 		if err := tx.Put(TablePoolLimbo, keyBytes, streamBytes); err != nil {
	// 			return err
	// 		}
	// 	}

	// 	// L1InfoTreeMinTimestamps
	// 	keyBytes[5] = DbKeyBatchesL1InfoTreePrefix
	// 	for k, v := range limboBatch.L1InfoTreeMinTimestamps {
	// 		binary.LittleEndian.PutUint64(keyBytes[6:14], uint64(k))
	// 		binary.LittleEndian.PutUint64(vBytes[:], v)
	// 		if err := tx.Put(TablePoolLimbo, keyBytes, vBytes); err != nil {
	// 			return err
	// 		}
	// 	}

	// 	// TimestampLimit
	// 	keyBytes[5] = DbKeyBatchesTimestampLimitPrefix
	// 	binary.LittleEndian.PutUint64(keyBytes[6:14], 0)
	// 	binary.LittleEndian.PutUint64(vBytes[:], limboBatch.TimestampLimit)
	// 	if err := tx.Put(TablePoolLimbo, keyBytes, vBytes); err != nil {
	// 		return err
	// 	}

	// 	// FirstBlockNumber
	// 	keyBytes[5] = DbKeyBatchesFirstBlockNumberPrefix
	// 	binary.LittleEndian.PutUint64(keyBytes[6:14], 0)
	// 	binary.LittleEndian.PutUint64(vBytes[:], limboBatch.FirstBlockNumber)
	// 	if err := tx.Put(TablePoolLimbo, keyBytes, vBytes); err != nil {
	// 		return err
	// 	}

	// 	// BatchNumber
	// 	keyBytes[5] = DbKeyBatchesBatchNumberPrefix
	// 	binary.LittleEndian.PutUint64(keyBytes[6:14], 0)
	// 	binary.LittleEndian.PutUint64(vBytes[:], limboBatch.BatchNumber)
	// 	if err := tx.Put(TablePoolLimbo, keyBytes, vBytes); err != nil {
	// 		return err
	// 	}

	// 	// BatchNumber
	// 	keyBytes[5] = DbKeyBatchesForkIdPrefix
	// 	binary.LittleEndian.PutUint64(keyBytes[6:14], 0)
	// 	binary.LittleEndian.PutUint64(vBytes[:], limboBatch.ForkId)
	// 	if err := tx.Put(TablePoolLimbo, keyBytes, vBytes); err != nil {
	// 		return err
	// 	}

	// 	// Root
	// 	keyBytes[5] = DbKeyBatchesRootPrefix
	// 	for j, hash := range limboBatch.Roots {
	// 		binary.LittleEndian.PutUint64(keyBytes[6:14], uint64(j))
	// 		if err := tx.Put(TablePoolLimbo, keyBytes, hash[:]); err != nil {
	// 			return err
	// 		}
	// 	}

	// 	// BadTransactionsHashes
	// 	keyBytes[5] = DbKeyBatchesBadTransactionsPrefix
	// 	for j, hash := range limboBatch.BadTransactionsHashes {
	// 		binary.LittleEndian.PutUint64(keyBytes[6:14], uint64(j))
	// 		if err := tx.Put(TablePoolLimbo, keyBytes, hash[:]); err != nil {
	// 			return err
	// 		}
	// 	}
	// }

	// v = []byte{0}
	// if p.limbo.awaitingBlockHandling.Load() {
	// 	v[0] = 1
	// }
	// if err := tx.Put(TablePoolLimbo, []byte{DbKeyAwaitingBlockHandlingPrefix}, v); err != nil {
	// 	return err
	// }

	return nil
}

func (p *TxPool) fromDBLimbo(ctx context.Context, tx kv.Tx, cacheView kvcache.CacheView) error {
	// it, err := tx.Range(TablePoolLimbo, nil, nil)
	// if err != nil {
	// 	return err
	// }

	// p.limbo.limboSlots = &types.TxSlots{}
	// parseCtx := types.NewTxParseContext(p.chainID)
	// parseCtx.WithSender(false)

	// for it.HasNext() {
	// 	k, v, err := it.Next()
	// 	if err != nil {
	// 		return err
	// 	}

	// 	switch k[0] {
	// 	case DbKeyInvalidTxPrefix:
	// 		hash := hexutils.BytesToHex(k[1:])
	// 		p.limbo.invalidTxsMap[hash] = v[0]
	// 	case DbKeySlotsPrefix:
	// 		addr, txRlp := *(*[20]byte)(v[:20]), v[20:]
	// 		txn := &types.TxSlot{}

	// 		_, err = parseCtx.ParseTransaction(txRlp, 0, txn, nil, false /* hasEnvelope */, nil)
	// 		if err != nil {
	// 			err = fmt.Errorf("err: %w, rlp: %x", err, txRlp)
	// 			log.Warn("[txpool] fromDB: parseTransaction", "err", err)
	// 			continue
	// 		}

	// 		txn.SenderID, txn.Traced = p.senders.getOrCreateID(addr)
	// 		binary.BigEndian.Uint64(v)

	// 		if reason := p.validateTx(txn, true, cacheView); reason != NotSet && reason != Success {
	// 			return nil
	// 		}
	// 		p.limbo.limboSlots.Append(txn, addr[:], true)
	// 	case DbKeyBatchesPrefix:
	// 		batchesI := binary.LittleEndian.Uint32(k[1:5])
	// 		batchesJ := binary.LittleEndian.Uint64(k[6:14])
	// 		p.limbo.resizeBatches(int(batchesI) + 1)

	// 		switch k[5] {
	// 		case DbKeyBatchesWitnessPrefix:
	// 			p.limbo.limboBatches[batchesI].Witness = v
	// 		case DbKeyBatchesStreamBytesPrefix:
	// 			p.limbo.limboBatches[batchesI].resizeStreamBytes(int(batchesJ) + 1)
	// 			p.limbo.limboBatches[batchesI].StreamBytes[batchesJ] = v
	// 		case DbKeyBatchesL1InfoTreePrefix:
	// 			p.limbo.limboBatches[batchesI].L1InfoTreeMinTimestamps[batchesJ] = binary.LittleEndian.Uint64(v)
	// 		case DbKeyBatchesTimestampLimitPrefix:
	// 			p.limbo.limboBatches[batchesI].TimestampLimit = binary.LittleEndian.Uint64(v)
	// 		case DbKeyBatchesFirstBlockNumberPrefix:
	// 			p.limbo.limboBatches[batchesI].FirstBlockNumber = binary.LittleEndian.Uint64(v)
	// 		case DbKeyBatchesBatchNumberPrefix:
	// 			p.limbo.limboBatches[batchesI].BatchNumber = binary.LittleEndian.Uint64(v)
	// 		case DbKeyBatchesForkIdPrefix:
	// 			p.limbo.limboBatches[batchesI].ForkId = binary.LittleEndian.Uint64(v)
	// 		case DbKeyBatchesRootPrefix:
	// 			p.limbo.limboBatches[batchesI].resizeRoots(int(batchesJ) + 1)
	// 			copy(p.limbo.limboBatches[batchesI].Roots[batchesJ][:], v)
	// 		case DbKeyBatchesBadTransactionsPrefix:
	// 			p.limbo.limboBatches[batchesI].resizeBadTransactionsHashes(int(batchesJ) + 1)
	// 			copy(p.limbo.limboBatches[batchesI].BadTransactionsHashes[batchesJ][:], v)
	// 		}
	// 	case DbKeyAwaitingBlockHandlingPrefix:
	// 		if v[0] == 0 {
	// 			p.limbo.awaitingBlockHandling.Store(false)
	// 		} else {
	// 			p.limbo.awaitingBlockHandling.Store(true)
	// 		}
	// 	}

	// }

	return nil
}

func prepareSendersWithChangedState(txs *types.TxSlots) map[uint64]struct{} {
	sendersWithChangedState := map[uint64]struct{}{}

	for _, txn := range txs.Txs {
		sendersWithChangedState[txn.SenderID] = struct{}{}
	}

	return sendersWithChangedState
}
