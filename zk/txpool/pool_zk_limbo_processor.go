package txpool

import (
	"context"
	"math"
	"time"

	"github.com/gateway-fm/cdk-erigon-lib/kv"
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
				_this.run()
			}
		}
	}()
}

func (_this *LimboSubPoolProcessor) run() {
	log.Info("[Limbo pool processor] Starting")
	defer log.Info("[Limbo pool processor] End")

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

	invalidTxs := []*string{}

	for _, batchDetails := range limboBatchDetails {
		request := legacy_executor_verifier.NewVerifierRequest(batchDetails.BatchNumber, batchDetails.ForkId, batchDetails.Root, unlimitedCounters)
		for i, streamBytes := range batchDetails.StreamBytes {
			err := _this.verifier.VerifySync(tx, request, batchDetails.Witness, streamBytes, batchDetails.TimestampLimit, batchDetails.FirstBlockNumber, batchDetails.L1InfoTreeMinTimestamps)
			idHash := hexutils.BytesToHex(batchDetails.BadTransactionsHashes[i][:])
			if err != nil {
				invalidTxs = append(invalidTxs, &idHash)
				log.Info("[Limbo pool processor]", "invalid tx", batchDetails.BadTransactionsHashes[i])
				continue
			}

			log.Info("[Limbo pool processor]", "valid tx", batchDetails.BadTransactionsHashes[i])
		}
	}

	_this.txPool.MarkProcessedLimboDetails(size, invalidTxs)

}
