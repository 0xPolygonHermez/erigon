package stages

import (
	"context"
	"time"

	"github.com/gateway-fm/cdk-erigon-lib/common"
	"github.com/gateway-fm/cdk-erigon-lib/kv"

	"bytes"
	"io"

	mapset "github.com/deckarep/golang-set/v2"
	types2 "github.com/gateway-fm/cdk-erigon-lib/types"
	"github.com/ledgerwatch/erigon/core"
	"github.com/ledgerwatch/erigon/core/rawdb"
	"github.com/ledgerwatch/erigon/core/state"
	"github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/core/vm"
	"github.com/ledgerwatch/erigon/rlp"
	"github.com/ledgerwatch/erigon/zk/hermez_db"
	"github.com/ledgerwatch/erigon/zk/tx"
)

func getNextPoolTransactions(cfg SequenceBlockCfg, executionAt, forkId uint64, alreadyYielded mapset.Set[[32]byte]) ([]types.Transaction, error) {
	var transactions []types.Transaction
	var err error
	var count int
	killer := time.NewTicker(50 * time.Millisecond)
LOOP:
	for {
		// ensure we don't spin forever looking for transactions, attempt for a while then exit up to the caller
		select {
		case <-killer.C:
			break LOOP
		default:
		}
		if err := cfg.txPoolDb.View(context.Background(), func(poolTx kv.Tx) error {
			slots := types2.TxsRlp{}
			_, count, err = cfg.txPool.YieldBest(yieldSize, &slots, poolTx, executionAt, getGasLimit(uint16(forkId)), alreadyYielded)
			if err != nil {
				return err
			}
			if count == 0 {
				time.Sleep(500 * time.Microsecond)
				return nil
			}
			transactions, err = extractTransactionsFromSlot(slots)
			if err != nil {
				return err
			}
			return nil
		}); err != nil {
			return nil, err
		}

		if len(transactions) > 0 {
			break
		}
	}

	return transactions, err
}

func getNextL1BatchTransactions(batchNumber uint64, forkId uint64, hermezDb *hermez_db.HermezDb) ([]types.Transaction, []uint8, bool, error) {
	// we expect that the batch we're going to load in next should be in the db already because of the l1 block sync
	// stage, if it is not there we need to panic as we're in a bad state
	batchL2Data, err := hermezDb.GetL1BatchData(batchNumber)
	if err != nil {
		return nil, nil, true, err
	}
	if len(batchL2Data) == 0 {
		// end of the line for batch recovery so return empty
		return []types.Transaction{}, []uint8{}, false, nil
	}

	transactions, _, effectiveGases, err := tx.DecodeTxs(batchL2Data, forkId)

	isWorkRemaining := true
	if len(transactions) == 0 {
		// we need to check if this batch should simply be empty or not so we need to check against the
		// highest known batch number to see if we have work to do still
		highestKnown, err := hermezDb.GetLastL1BatchData()
		if err != nil {
			return nil, nil, true, err
		}
		if batchNumber >= highestKnown {
			isWorkRemaining = false
		}
	}

	return transactions, effectiveGases, isWorkRemaining, err
}

func extractTransactionsFromSlot(slot types2.TxsRlp) ([]types.Transaction, error) {
	transactions := make([]types.Transaction, 0, len(slot.Txs))
	reader := bytes.NewReader([]byte{})
	stream := new(rlp.Stream)
	for idx, txBytes := range slot.Txs {
		reader.Reset(txBytes)
		stream.Reset(reader, uint64(len(txBytes)))
		transaction, err := types.DecodeTransaction(stream)
		if err == io.EOF {
			continue
		}
		if err != nil {
			return nil, err
		}
		var sender common.Address
		copy(sender[:], slot.Senders.At(idx))
		transaction.SetSender(sender)
		transactions = append(transactions, transaction)
	}
	return transactions, nil
}

func attemptAddTransaction(
	cfg SequenceBlockCfg,
	sdb *stageDb,
	ibs *state.IntraBlockState,
	batchCounters *vm.BatchCounterCollector,
	header *types.Header,
	parentHeader *types.Header,
	transaction types.Transaction,
	effectiveGasPrice uint8,
) (*types.Receipt, bool, error) {
	txCounters := vm.NewTransactionCounter(transaction, sdb.smt.GetDepth(), cfg.zk.ShouldCountersBeUnlimited())
	overflow, err := batchCounters.AddNewTransactionCounters(txCounters)
	if err != nil {
		return nil, false, err
	}
	if overflow {
		return nil, true, nil
	}

	gasPool := new(core.GasPool).AddGas(transactionGasLimit)
	getHeader := func(hash common.Hash, number uint64) *types.Header { return rawdb.ReadHeader(sdb.tx, hash, number) }

	// set the counter collector on the config so that we can gather info during the execution
	cfg.zkVmConfig.CounterCollector = txCounters.ExecutionCounters()

	// TODO: possibly inject zero tracer here!

	ibs.Prepare(transaction.Hash(), common.Hash{}, 0)

	receipt, execResult, err := core.ApplyTransaction_zkevm(
		cfg.chainConfig,
		core.GetHashFn(header, getHeader),
		cfg.engine,
		&cfg.zk.AddressSequencer,
		gasPool,
		ibs,
		noop,
		header,
		transaction,
		&header.GasUsed,
		*cfg.zkVmConfig,
		parentHeader.ExcessDataGas,
		effectiveGasPrice)

	if err != nil {
		return nil, false, err
	}

	// we need to keep hold of the effective percentage used
	// todo [zkevm] for now we're hard coding to the max value but we need to calc this properly
	if err = sdb.hermezDb.WriteEffectiveGasPricePercentage(transaction.Hash(), effectiveGasPrice); err != nil {
		return nil, false, err
	}

	err = txCounters.ProcessTx(ibs, execResult.ReturnData)
	if err != nil {
		return nil, false, err
	}

	// now that we have executed we can check again for an overflow
	overflow, err = batchCounters.CheckForOverflow()

	return receipt, overflow, err
} // will be called at the start of every new block created within a batch to figure out if there is a new GER// we can use or not.  In the special case that this is the first block we just return 0 as we need to use the// 0 index first before we can use 1+func calculateNextL1TreeUpdateToUse(tx kv.RwTx, hermezDb *hermez_db.HermezDb) (uint64, *zktypes.L1InfoTreeUpdate, error) {	// always default to 0 and only update this if the next available index has reached finality	var nextL1Index uint64 = 0	// check which was the last used index	lastInfoIndex, err := stages.GetStageProgress(tx, stages.HighestUsedL1InfoIndex)	if err != nil {		return 0, nil, err	}	// check if the next index is there and if it has reached finality or not	l1Info, err := hermezDb.GetL1InfoTreeUpdate(lastInfoIndex + 1)	if err != nil {		return 0, nil, err	}	// check that we have reached finality on the l1 info tree event before using it	// todo: [zkevm] think of a better way to handle finality	now := time.Now()	target := now.Add(-(12 * time.Minute))	if l1Info != nil && l1Info.Timestamp < uint64(target.Unix()) {		nextL1Index = l1Info.Index	}	return nextL1Index, l1Info, nil}func updateSequencerProgress(tx kv.RwTx, newHeight uint64, newBatch uint64, l1InfoIndex uint64) error {	// now update stages that will be used later on in stageloop.go and other stages. As we're the sequencer	// we won't have headers stage for example as we're already writing them here	if err := stages.SaveStageProgress(tx, stages.Execution, newHeight); err != nil {		return err	}	if err := stages.SaveStageProgress(tx, stages.Headers, newHeight); err != nil {		return err	}	if err := stages.SaveStageProgress(tx, stages.HighestSeenBatchNumber, newBatch); err != nil {		return err	}	if err := stages.SaveStageProgress(tx, stages.HighestUsedL1InfoIndex, l1InfoIndex); err != nil {		return err	}	return nil}func UnwindSequenceExecutionStage(u *stagedsync.UnwindState, s *stagedsync.StageState, tx kv.RwTx, ctx context.Context, cfg SequenceBlockCfg, initialCycle bool) (err error) {	if u.UnwindPoint >= s.BlockNumber {		return nil	}	useExternalTx := tx != nil	if !useExternalTx {		tx, err = cfg.db.BeginRw(context.Background())		if err != nil {			return err		}		defer tx.Rollback()	}	logPrefix := u.LogPrefix()	log.Info(fmt.Sprintf("[%s] Unwind Execution", logPrefix), "from", s.BlockNumber, "to", u.UnwindPoint)	if err = unwindExecutionStage(u, s, tx, ctx, cfg, initialCycle); err != nil {		return err	}	if err = u.Done(tx); err != nil {		return err	}	if !useExternalTx {		if err = tx.Commit(); err != nil {			return err		}	}	return nil}func unwindExecutionStage(u *stagedsync.UnwindState, s *stagedsync.StageState, tx kv.RwTx, ctx context.Context, cfg SequenceBlockCfg, initialCycle bool) error {	logPrefix := s.LogPrefix()	stateBucket := kv.PlainState	storageKeyLength := length.Addr + length.Incarnation + length.Hash	var accumulator *shards.Accumulator	if !initialCycle && cfg.stateStream && s.BlockNumber-u.UnwindPoint < stateStreamLimit {		accumulator = cfg.accumulator		hash, err := rawdb.ReadCanonicalHash(tx, u.UnwindPoint)		if err != nil {			return fmt.Errorf("read canonical hash of unwind point: %w", err)		}		txs, err := rawdb.RawTransactionsRange(tx, u.UnwindPoint, s.BlockNumber)		if err != nil {			return err		}		accumulator.StartChange(u.UnwindPoint, hash, txs, true)	}	changes := etl.NewCollector(logPrefix, cfg.dirs.Tmp, etl.NewOldestEntryBuffer(etl.BufferOptimalSize))	defer changes.Close()	errRewind := changeset.RewindData(tx, s.BlockNumber, u.UnwindPoint, changes, ctx.Done())	if errRewind != nil {		return fmt.Errorf("getting rewind data: %w", errRewind)	}	if err := changes.Load(tx, stateBucket, func(k, v []byte, table etl.CurrentTableReader, next etl.LoadNextFunc) error {		if len(k) == 20 {			if len(v) > 0 {				var acc accounts.Account				if err := acc.DecodeForStorage(v); err != nil {					return err				}				// Fetch the code hash				recoverCodeHashPlain(&acc, tx, k)				var address common.Address				copy(address[:], k)				// cleanup contract code bucket				original, err := state.NewPlainStateReader(tx).ReadAccountData(address)				if err != nil {					return fmt.Errorf("read account for %x: %w", address, err)				}				if original != nil {					// clean up all the code incarnations original incarnation and the new one					for incarnation := original.Incarnation; incarnation > acc.Incarnation && incarnation > 0; incarnation-- {						err = tx.Delete(kv.PlainContractCode, dbutils.PlainGenerateStoragePrefix(address[:], incarnation))						if err != nil {							return fmt.Errorf("writeAccountPlain for %x: %w", address, err)						}					}				}				newV := make([]byte, acc.EncodingLengthForStorage())				acc.EncodeForStorage(newV)				if accumulator != nil {					accumulator.ChangeAccount(address, acc.Incarnation, newV)				}				if err := next(k, k, newV); err != nil {					return err				}			} else {				if accumulator != nil {					var address common.Address					copy(address[:], k)					accumulator.DeleteAccount(address)				}				if err := next(k, k, nil); err != nil {					return err				}			}			return nil		}		if accumulator != nil {			var address common.Address			var incarnation uint64			var location common.Hash			copy(address[:], k[:length.Addr])			incarnation = binary.BigEndian.Uint64(k[length.Addr:])			copy(location[:], k[length.Addr+length.Incarnation:])			log.Debug(fmt.Sprintf("un ch st: %x, %d, %x, %x\n", address, incarnation, location, common.Copy(v)))			accumulator.ChangeStorage(address, incarnation, location, common.Copy(v))		}		if len(v) > 0 {			if err := next(k, k[:storageKeyLength], v); err != nil {				return err			}		} else {			if err := next(k, k[:storageKeyLength], nil); err != nil {				return err			}		}		return nil	}, etl.TransformArgs{Quit: ctx.Done()}); err != nil {		return err	}	if err := historyv2.Truncate(tx, u.UnwindPoint+1); err != nil {		return err	}	if err := rawdb.TruncateReceipts(tx, u.UnwindPoint+1); err != nil {		return fmt.Errorf("truncate receipts: %w", err)	}	if err := rawdb.TruncateBorReceipts(tx, u.UnwindPoint+1); err != nil {		return fmt.Errorf("truncate bor receipts: %w", err)	}	if err := rawdb.DeleteNewerEpochs(tx, u.UnwindPoint+1); err != nil {		return fmt.Errorf("delete newer epochs: %w", err)	}	// Truncate CallTraceSet	keyStart := hexutility.EncodeTs(u.UnwindPoint + 1)	c, err := tx.RwCursorDupSort(kv.CallTraceSet)	if err != nil {		return err	}	defer c.Close()	for k, _, err := c.Seek(keyStart); k != nil; k, _, err = c.NextNoDup() {		if err != nil {			return err		}		err = c.DeleteCurrentDuplicates()		if err != nil {			return err		}	}	return nil}func recoverCodeHashPlain(acc *accounts.Account, db kv.Tx, key []byte) {	var address common.Address	copy(address[:], key)	if acc.Incarnation > 0 && acc.IsEmptyCodeHash() {		if codeHash, err2 := db.GetOne(kv.PlainContractCode, dbutils.PlainGenerateStoragePrefix(address[:], acc.Incarnation)); err2 == nil {			copy(acc.CodeHash[:], codeHash)		}	}}func PruneSequenceExecutionStage(s *stagedsync.PruneState, tx kv.RwTx, cfg SequenceBlockCfg, ctx context.Context, initialCycle bool) (err error) {	logPrefix := s.LogPrefix()	useExternalTx := tx != nil	if !useExternalTx {		tx, err = cfg.db.BeginRw(ctx)		if err != nil {			return err		}		defer tx.Rollback()	}	logEvery := time.NewTicker(logInterval)	defer logEvery.Stop()	if cfg.historyV3 {		cfg.agg.SetTx(tx)		if initialCycle {			if err = cfg.agg.Prune(ctx, ethconfig.HistoryV3AggregationStep/10); err != nil { // prune part of retired data, before commit				return err			}		} else {			if err = cfg.agg.PruneWithTiemout(ctx, 1*time.Second); err != nil { // prune part of retired data, before commit				return err			}		}	} else {		if cfg.prune.History.Enabled() {			if err = rawdb.PruneTableDupSort(tx, kv.AccountChangeSet, logPrefix, cfg.prune.History.PruneTo(s.ForwardProgress), logEvery, ctx); err != nil {				return err			}			if err = rawdb.PruneTableDupSort(tx, kv.StorageChangeSet, logPrefix, cfg.prune.History.PruneTo(s.ForwardProgress), logEvery, ctx); err != nil {				return err			}		}		if cfg.prune.Receipts.Enabled() {			if err = rawdb.PruneTable(tx, kv.Receipts, cfg.prune.Receipts.PruneTo(s.ForwardProgress), ctx, math.MaxInt32); err != nil {				return err			}			if err = rawdb.PruneTable(tx, kv.BorReceipts, cfg.prune.Receipts.PruneTo(s.ForwardProgress), ctx, math.MaxUint32); err != nil {				return err			}			// LogIndex.Prune will read everything what not pruned here			if err = rawdb.PruneTable(tx, kv.Log, cfg.prune.Receipts.PruneTo(s.ForwardProgress), ctx, math.MaxInt32); err != nil {				return err			}		}		if cfg.prune.CallTraces.Enabled() {			if err = rawdb.PruneTableDupSort(tx, kv.CallTraceSet, logPrefix, cfg.prune.CallTraces.PruneTo(s.ForwardProgress), logEvery, ctx); err != nil {				return err			}		}	}	if err = s.Done(tx); err != nil {		return err	}	if !useExternalTx {		if err = tx.Commit(); err != nil {			return err		}	}	return nil}// if executionAt == 1 {// 	// from := executionAt + 1// 	fmt.Println("DEBUG 2")// 	to := bn + 1// 	for i := uint64(0); i <= to; i++ {// 		psr := state2.NewPlainState(tx, i+1, systemcontracts.SystemContractCodeLookup["Hermez"])// 		address := libcommon.Address{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 92, 161, 171, 30}// 		sstorageKey := []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}// 		stk := libcommon.BytesToHash(sstorageKey)// 		value, err := psr.ReadAccountStorage(address, 1, &stk)// 		if err != nil {// 			return err// 		}// 		fmt.Printf("Value for block %d\n", i)// 		fmt.Println(value)// 	}// }
