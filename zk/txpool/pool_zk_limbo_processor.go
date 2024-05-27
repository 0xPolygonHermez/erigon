package txpool

import (
	"context"
	"math"
	"time"

	"github.com/gateway-fm/cdk-erigon-lib/kv"
	"github.com/gateway-fm/cdk-erigon-lib/types"
	"github.com/holiman/uint256"
	"github.com/ledgerwatch/erigon/chain"
	"github.com/ledgerwatch/erigon/core/vm"
	"github.com/ledgerwatch/erigon/zk/legacy_executor_verifier"
	"github.com/ledgerwatch/log/v3"
	"github.com/status-im/keycard-go/hexutils"
)

type LimboSubPoolProcessor struct {
	chainConfig *chain.Config
	db          kv.RwDB
	txPool      *TxPool
	verifier    *legacy_executor_verifier.LegacyExecutorVerifier
	quit        <-chan struct{}
}

func NewLimboSubPoolProcessor(ctx context.Context, chainConfig *chain.Config, db kv.RwDB, txPool *TxPool, verifier *legacy_executor_verifier.LegacyExecutorVerifier) *LimboSubPoolProcessor {
	return &LimboSubPoolProcessor{
		chainConfig: chainConfig,
		db:          db,
		txPool:      txPool,
		verifier:    verifier,
		quit:        ctx.Done(),
	}
}

func (_this *LimboSubPoolProcessor) StartWork() {
	go func() {
		tick := time.NewTicker(30 * time.Second)
		defer tick.Stop()
	LOOP:
		for {
			select {
			case <-_this.quit:
				break LOOP
			case <-tick.C:
				// _this.run()
			}
		}
	}()
}

func (_this *LimboSubPoolProcessor) run() {
	log.Info("[Limbo pool processor] Starting")

	ctx := context.Background()
	limboBatchDetails := _this.txPool.GetLimboDetailsCloned()
	size := len(limboBatchDetails)
	if size == 0 {
		return
	}

	tx, err := _this.db.BeginRo(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback()

	// we just need some counter variable with large used values in order verify not to complain
	batchCounters := vm.NewBatchCounterCollector(256, 1, true)
	unlimitedCounters := batchCounters.NewCounters().UsedAsMap()
	for k := range unlimitedCounters {
		unlimitedCounters[k] = math.MaxInt32
	}

	var slots types.TxSlots
	chainId := _this.getChainIdAsInt256()

	parseCtx := types.NewTxParseContext(*chainId).ChainIDRequired()
	parseCtx.ValidateRLP(_this.txPool.ValidateSerializedTxn)
	validTxs := []*string{}
	invalidTxs := []*string{}

	for _, batchDetails := range limboBatchDetails {
		request := legacy_executor_verifier.NewVerifierRequest(batchDetails.BatchNumber, batchDetails.ForkId, batchDetails.Root, unlimitedCounters)
		for i, streamBytes := range batchDetails.StreamBytes {
			err := _this.verifier.VerifySync(tx, request, batchDetails.Witness, streamBytes, batchDetails.TimestampLimit, batchDetails.FirstBlockNumber, batchDetails.L1InfoTreeMinTimestamps)
			idHash := hexutils.BytesToHex(batchDetails.BadTransactionsHashes[i][:])
			if err != nil {
				validTxs = append(validTxs, &idHash)
				// invalidTxMap[idHash] = struct{}{}
				log.Info("[Limbo pool processor]", "invalid tx", batchDetails.BadTransactionsHashes[i])
				continue
			}

			invalidTxs = append(invalidTxs, &idHash)
			log.Info("[Limbo pool processor]", "valid tx", batchDetails.BadTransactionsHashes[i])

			// isTxKnown, err := _this.txPool.IdHashKnown(tx, batchDetails.BadTransactionsHashes[i][:])
			// if isTxKnown {
			// 	fmt.Println(err)
			// 	fmt.Println("")
			// }
			slots.Resize(1)
			slots.Txs[0] = &types.TxSlot{}
			slots.IsLocal[0] = true
			_, err = parseCtx.ParseTransaction(batchDetails.BadTransactionsRLP[i], 0, slots.Txs[0], slots.Senders.At(0), false /* hasEnvelope */, func(hash []byte) error {
				// if known, _ := _this.txPool.IdHashKnown(tx, hash); known {
				// 	return types.ErrAlreadyKnown
				// }
				return nil
			})
			if err != nil {
				panic(err)
			}

			mt := newMetaTx(slots.Txs[0], slots.IsLocal[0], _this.txPool.lastSeenBlock.Load())
			_this.txPool.pending.Add(mt)

			// discardReasons, err := _this.txPool.AddLocalTxs(ctx, slots, tx)
			// if err != nil {
			// 	fmt.Println(discardReasons)
			// 	panic(err)
			// 	return
			// }

			//TODO: requeue to txpool
		}
	}

	// _this.txPool.MarkProcessedLimboDetails(size, validTxs, invalidTxs)
	log.Info("[Limbo pool processor] End")

}

func (_this *LimboSubPoolProcessor) getChainIdAsInt256() *uint256.Int {
	chainId, _ := uint256.FromBig(_this.chainConfig.ChainID)
	return chainId
}
