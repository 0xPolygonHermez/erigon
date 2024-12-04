package stages

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/log/v3"

	"sync"

	"github.com/ledgerwatch/erigon-lib/chain"
	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/metrics"
	"github.com/ledgerwatch/erigon/core/rawdb"
	ethTypes "github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/eth/ethconfig"
	"github.com/ledgerwatch/erigon/eth/stagedsync"
	"github.com/ledgerwatch/erigon/eth/stagedsync/stages"
	"github.com/ledgerwatch/erigon/zk/hermez_db"
	"github.com/ledgerwatch/erigon/zk/l1_log_parser"
	"github.com/ledgerwatch/erigon/zk/sequencer"
	"github.com/ledgerwatch/erigon/zk/types"
)

type IL1Syncer interface {
	// atomic
	IsSyncStarted() bool
	IsDownloading() bool
	GetLastCheckedL1Block() uint64

	// Channels
	GetLogsChan() chan []ethTypes.Log
	GetProgressMessageChan() chan string

	L1QueryHeaders(logs []ethTypes.Log) (map[uint64]*ethTypes.Header, error)
	GetBlock(number uint64) (*ethTypes.Block, error)
	GetHeader(number uint64) (*ethTypes.Header, error)
	RunQueryBlocks(lastCheckedBlock uint64)
	StopQueryBlocks()
	ConsumeQueryBlocks()
	WaitQueryBlocksToFinish()
	CheckL1BlockFinalized(blockNo uint64) (bool, uint64, error)

	CallGetRollupSequencedBatches(ctx context.Context, addr *common.Address, rollupId, batchNum uint64) (common.Hash, uint64, error)
}

var (
	ErrStateRootMismatch      = errors.New("state root mismatch")
	lastCheckedL1BlockCounter = metrics.GetOrCreateGauge(`last_checked_l1_block`)
)

type L1SyncerCfg struct {
	db     kv.RwDB
	syncer IL1Syncer

	zkCfg *ethconfig.Zk
}

func StageL1SyncerCfg(db kv.RwDB, syncer IL1Syncer, zkCfg *ethconfig.Zk) L1SyncerCfg {
	return L1SyncerCfg{
		db:     db,
		syncer: syncer,
		zkCfg:  zkCfg,
	}
}

func SpawnStageL1Syncer(
	s *stagedsync.StageState,
	u stagedsync.Unwinder,
	ctx context.Context,
	tx kv.RwTx,
	cfg L1SyncerCfg,
	quiet bool,
) (funcErr error) {
	///// DEBUG BISECT /////
	if cfg.zkCfg.DebugLimit > 0 {
		return nil
	}
	///// DEBUG BISECT /////

	logPrefix := s.LogPrefix()
	log.Info(fmt.Sprintf("[%s] Starting L1 sync stage", logPrefix))
	// if sequencer.IsSequencer() {
	// 	log.Info(fmt.Sprintf("[%s] skipping -- sequencer", logPrefix))
	// 	return nil
	// }
	defer log.Info(fmt.Sprintf("[%s] Finished L1 sync stage ", logPrefix))

	var internalTxOpened bool
	if tx == nil {
		internalTxOpened = true
		log.Debug("l1 sync: no tx provided, creating a new one")
		var err error
		tx, err = cfg.db.BeginRw(ctx)
		if err != nil {
			return fmt.Errorf("cfg.db.BeginRw: %w", err)
		}
		defer tx.Rollback()
	}

	// pass tx to the hermezdb
	hermezDb := hermez_db.NewHermezDb(tx)

	// get l1 block progress from this stage's progress
	l1BlockProgress, err := stages.GetStageProgress(tx, stages.L1Syncer)
	if err != nil {
		return fmt.Errorf("GetStageProgress, %w", err)
	}

	// start syncer if not started
	if !cfg.syncer.IsSyncStarted() {
		if l1BlockProgress == 0 {
			l1BlockProgress = cfg.zkCfg.L1FirstBlock - 1
		}

		// start the syncer
		cfg.syncer.RunQueryBlocks(l1BlockProgress)
		defer func() {
			if funcErr != nil {
				cfg.syncer.StopQueryBlocks()
				cfg.syncer.ConsumeQueryBlocks()
				cfg.syncer.WaitQueryBlocksToFinish()
			}
		}()
	}

	logsChan := cfg.syncer.GetLogsChan()
	progressMessageChan := cfg.syncer.GetProgressMessageChan()

	l1LogParser := l1_log_parser.NewL1LogParser(cfg.syncer, hermezDb, cfg.zkCfg.L1RollupId)

	syncMeta := &l1_log_parser.L1SyncMeta{
		HighestVerification: types.BatchVerificationInfo{
			BaseBatchInfo: types.BaseBatchInfo{
				BatchNo:   0,
				L1BlockNo: 0,
				L1TxHash:  common.Hash{},
			},
		},
		HighestWrittenL1BlockNo: 0,
		NewVerificationsCount:   0,
		NewSequencesCount:       0,
	}
Loop:
	for {
		select {
		case logs := <-logsChan:
			for _, l := range logs {
				lg := l
				syncMeta, err = l1LogParser.ParseAndHandleLog(&lg, syncMeta)
				if err != nil {
					return fmt.Errorf("ParseAndHandleLog: %w", err)
				}
			}
		case progressMessage := <-progressMessageChan:
			log.Info(fmt.Sprintf("[%s] %s", logPrefix, progressMessage))
		default:
			if !cfg.syncer.IsDownloading() {
				break Loop
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	// // do this separately to allow upgrading nodes to back-fill the table
	// err = getAccInputHashes(ctx, logPrefix, hermezDb, cfg.syncer, &cfg.zkCfg.AddressRollup, cfg.zkCfg.L1RollupId, highestVerification.BatchNo)
	// if err != nil {
	// 	return fmt.Errorf("getAccInputHashes: %w", err)
	// }

	// // get l1 block timestamps
	// err = getL1BlockTimestamps(ctx, logPrefix, hermezDb, cfg.syncer, &cfg.zkCfg.AddressRollup, cfg.zkCfg.L1RollupId, highestVerification.BatchNo)
	// if err != nil {
	// 	return fmt.Errorf("getL1BlockTimestamps: %w", err)
	// }

	latestCheckedBlock := cfg.syncer.GetLastCheckedL1Block()

	lastCheckedL1BlockCounter.Set(float64(latestCheckedBlock))

	if syncMeta.HighestWrittenL1BlockNo > l1BlockProgress {
		log.Info(fmt.Sprintf("[%s] Saving L1 syncer progress", logPrefix), "latestCheckedBlock", latestCheckedBlock, "newVerificationsCount", syncMeta.NewVerificationsCount, "newSequencesCount", syncMeta.NewSequencesCount, "highestWrittenL1BlockNo", syncMeta.HighestWrittenL1BlockNo)

		if err := stages.SaveStageProgress(tx, stages.L1Syncer, syncMeta.HighestWrittenL1BlockNo); err != nil {
			return fmt.Errorf("SaveStageProgress: %w", err)
		}
		if syncMeta.HighestVerification.BatchNo > 0 {
			log.Info(fmt.Sprintf("[%s]", logPrefix), "highestVerificationBatchNo", syncMeta.HighestVerification.BatchNo)
			if err := stages.SaveStageProgress(tx, stages.L1VerificationsBatchNo, syncMeta.HighestVerification.BatchNo); err != nil {
				return fmt.Errorf("SaveStageProgress: %w", err)
			}
		}

		// State Root Verifications Check
		if err = verifyAgainstLocalBlocks(tx, hermezDb, logPrefix); err != nil {
			if errors.Is(err, ErrStateRootMismatch) {
				panic(err)
			}
			// do nothing in hope the node will recover if it isn't a stateroot mismatch
		}
	} else {
		log.Info(fmt.Sprintf("[%s] No new L1 blocks to sync", logPrefix))
	}

	if internalTxOpened {
		log.Debug("l1 sync: first cycle, committing tx")
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("tx.Commit: %w", err)
		}
	}

	return nil
}

func UnwindL1SyncerStage(u *stagedsync.UnwindState, tx kv.RwTx, cfg L1SyncerCfg, ctx context.Context) (err error) {
	// we want to keep L1 data during an unwind, as we only sync finalised data there should be
	// no need to unwind here
	return nil
}

func PruneL1SyncerStage(s *stagedsync.PruneState, tx kv.RwTx, cfg L1SyncerCfg, ctx context.Context) (err error) {
	// no need to prune this data
	return nil
}

func verifyAgainstLocalBlocks(tx kv.RwTx, hermezDb *hermez_db.HermezDb, logPrefix string) (err error) {
	// get the highest hashed block
	hashedBlockNo, err := stages.GetStageProgress(tx, stages.IntermediateHashes)
	if err != nil {
		return fmt.Errorf("GetStageProgress: %w", err)
	}

	// no need to check - interhashes has not yet run
	if hashedBlockNo == 0 {
		return nil
	}

	// get the highest verified block
	verifiedBlockNo, err := hermezDb.GetHighestVerifiedBlockNo()
	if err != nil {
		return fmt.Errorf("GetHighestVerifiedBlockNo: %w", err)
	}

	// no verifications on l1
	if verifiedBlockNo == 0 {
		return nil
	}

	// 3 scenarios:
	//     1. verified and node both equal
	//     2. node behind l1 - verification block is higher than hashed block - use hashed block to find verification block
	//     3. l1 behind node - verification block is lower than hashed block - use verification block to find hashed block
	var blockToCheck uint64
	if verifiedBlockNo <= hashedBlockNo {
		blockToCheck = verifiedBlockNo
	} else {
		// in this case we need to find the blocknumber that is highest for the last batch
		// get the batch of the last hashed block
		hashedBatch, err := hermezDb.GetBatchNoByL2Block(hashedBlockNo)
		if err != nil && !errors.Is(err, hermez_db.ErrorNotStored) {
			return fmt.Errorf("GetBatchNoByL2Block: %w", err)
		}

		if hashedBatch == 0 {
			log.Warn(fmt.Sprintf("[%s] No batch number found for block %d", logPrefix, hashedBlockNo))
			return nil
		}

		// we don't know if this is the latest block in this batch, so check for the previous one
		// find the higher blocknum for previous batch
		blockNumbers, err := hermezDb.GetL2BlockNosByBatch(hashedBatch)
		if err != nil {
			return fmt.Errorf("GetL2BlockNosByBatch: %w", err)
		}

		if len(blockNumbers) == 0 {
			log.Warn(fmt.Sprintf("[%s] No block numbers found for batch %d", logPrefix, hashedBatch))
			return nil
		}

		for _, num := range blockNumbers {
			if num > blockToCheck {
				blockToCheck = num
			}
		}
	}

	// already checked
	highestChecked, err := stages.GetStageProgress(tx, stages.VerificationsStateRootCheck)
	if err != nil {
		return fmt.Errorf("GetStageProgress: %w", err)
	}
	if highestChecked >= blockToCheck {
		return nil
	}

	if !sequencer.IsSequencer() {
		if err = blockComparison(tx, hermezDb, blockToCheck, logPrefix); err == nil {
			log.Info(fmt.Sprintf("[%s] State root verified in block %d", logPrefix, blockToCheck))
			if err := stages.SaveStageProgress(tx, stages.VerificationsStateRootCheck, verifiedBlockNo); err != nil {
				return fmt.Errorf("SaveStageProgress: %w", err)
			}
		}
	}

	return err
}

func blockComparison(tx kv.RwTx, hermezDb *hermez_db.HermezDb, blockNo uint64, logPrefix string) error {
	v, err := hermezDb.GetVerificationByL2BlockNo(blockNo)
	if err != nil {
		return fmt.Errorf("GetVerificationByL2BlockNo: %w", err)
	}

	block, err := rawdb.ReadBlockByNumber(tx, blockNo)
	if err != nil {
		return fmt.Errorf("ReadBlockByNumber: %w", err)
	}

	if v == nil || block == nil {
		log.Info("block or verification is nil", "block", block, "verification", v)
		return nil
	}

	if v.StateRoot != block.Root() {
		log.Error(fmt.Sprintf("[%s] State root mismatch in block %d. Local=0x%x, L1 verification=0x%x", logPrefix, blockNo, block.Root(), v.StateRoot))
		return ErrStateRootMismatch
	}

	return nil
}

// call the l1 to get accInputHashes working backwards from the highest known batch, to the highest stored batch
// could be all the way to 0 for a new or upgrading node
func getAccInputHashes(ctx context.Context, logPrefix string, hermezDb *hermez_db.HermezDb, syncer IL1Syncer, rollupAddr *common.Address, rollupId uint64, highestSeenBatchNo uint64) error {
	if highestSeenBatchNo == 0 {
		log.Info(fmt.Sprintf("[%s] No (new) batches seen on L1, skipping accinputhash retreival", logPrefix))
		return nil
	}

	startTime := time.Now()

	highestStored, _, err := hermezDb.GetHighestStoredBatchAccInputHash()
	if err != nil {
		return fmt.Errorf("GetHighestStoredBatchAccInputHash: %w", err)
	}

	sequences, err := hermezDb.GetAllSequencesAboveBatchNo(highestStored)
	if err != nil {
		return fmt.Errorf("GetAllSequences: %w", err)
	}

	// only fetch for sequences above highest stored
	filteredSequences := make([]*types.BatchSequenceInfo, 0, len(sequences))
	for _, seq := range sequences {
		if seq.BatchNo > highestStored {
			filteredSequences = append(filteredSequences, seq)
		}
	}
	sequences = filteredSequences

	type Result struct {
		BatchNo      uint64
		AccInputHash common.Hash
		Error        error
	}

	resultsCh := make(chan Result)
	semaphore := make(chan struct{}, 5)

	totalSequences := len(sequences)
	processedSequences := 0

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// l1 queries
	go func() {
		var wg sync.WaitGroup
		defer close(resultsCh)

		for _, seq := range sequences {
			select {
			case <-ctx.Done():
				return
			default:
				semaphore <- struct{}{}
				wg.Add(1)

				go func(seq *types.BatchSequenceInfo) {
					defer wg.Done()
					defer func() { <-semaphore }()

					batchNo := seq.BatchNo

					accInputHash, _, err := syncer.CallGetRollupSequencedBatches(ctx, rollupAddr, rollupId, batchNo)
					if err != nil {
						log.Error(fmt.Sprintf("[%s] CallGetRollupSequencedBatches failed", logPrefix), "batch", batchNo, "err", err)
						select {
						case resultsCh <- Result{BatchNo: batchNo, Error: err}:
						case <-ctx.Done():
						}
						return
					}

					log.Debug(fmt.Sprintf("[%s] Got accinputhash from L1", logPrefix), "batch", batchNo, "hash", accInputHash)

					select {
					case resultsCh <- Result{BatchNo: batchNo, AccInputHash: accInputHash}:
					case <-ctx.Done():
					}
				}(seq)
			}
		}

		wg.Wait()
	}()

	// process results
	for {
		select {
		case res, ok := <-resultsCh:
			if !ok {
				duration := time.Since(startTime)
				log.Info(fmt.Sprintf("[%s] Completed fetching accinputhashes", logPrefix), "total_batches", totalSequences, "processed_batches", processedSequences, "duration", duration)
				return nil
			}
			if res.Error != nil {
				log.Warn(fmt.Sprintf("[%s] Error fetching accinputhash", logPrefix), "batch", res.BatchNo, "err", res.Error)
			}
			// Write to Db
			if err := hermezDb.WriteBatchAccInputHash(res.BatchNo, res.AccInputHash); err != nil {
				log.Error(fmt.Sprintf("[%s] WriteBatchAccInputHash failed", logPrefix), "batch", res.BatchNo, "err", err)
				return err
			}
			processedSequences++
		case <-ticker.C:
			log.Info(fmt.Sprintf("[%s] Progress update", logPrefix), "total_batches", totalSequences, "processed_batches", processedSequences)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// get l1 block timestamps - for all etrog sequences
func getL1BlockTimestamps(ctx context.Context, logPrefix string, hermezDb *hermez_db.HermezDb, syncer IL1Syncer, rollupAddr *common.Address, rollupId uint64, highestSeenBatchNo uint64) error {
	firstBatchNo, lastBatchNo, err := hermezDb.GetBatchLimitsBetweenForkIds(uint64(chain.ForkID7Etrog), uint64(chain.ForkID8Elderberry))
	if err != nil {
		return fmt.Errorf("GetEtrog7FirstAndLastBatchNubmers: %w", err)
	}

	if firstBatchNo == 0 || lastBatchNo == 0 {
		log.Info(fmt.Sprintf("[%s] No etrog or elderberry batches found, skipping L1 block timestamp retrieval", logPrefix))
		return nil
	}

	// find progress
	highestStoredTsBatchNo, _, _, err := hermezDb.GetHighestL1BlockTimestamp()
	if err != nil {
		return fmt.Errorf("GetHighestL1BlockTimestamp: %w", err)
	}

	// either go from progress, or firstBatchNo to lastBatch No (whichever is higher)
	startBatchNo := firstBatchNo
	if highestStoredTsBatchNo > firstBatchNo {
		startBatchNo = highestStoredTsBatchNo
	}
	endBatchNo := lastBatchNo

	startTime := time.Now()

	// get the sequences from the db, starting from etrog to elderberry (don't go any higher than elderberry)
	sequences, err := hermezDb.GetAllSequencesAboveBatchNo(startBatchNo)
	if err != nil {
		return fmt.Errorf("GetAllSequencesAboveBatchNo: %w", err)
	}

	// fetch missing from L1 RPC
	for _, seq := range sequences {
		if seq.BatchNo > endBatchNo {
			break
		}
		block, err := syncer.GetBlock(seq.L1BlockNo)
		if err != nil {
			return fmt.Errorf("GetBlock: %w", err)
		}

		err = hermezDb.WriteL1BlockTimestamp(seq.BatchNo, block.Time())
		if err != nil {
			log.Error(fmt.Sprintf("[%s] WriteL1BlockTimestamp failed", logPrefix), "batch", seq.BatchNo, "err", err)
			continue
		}
	}

	log.Info(fmt.Sprintf("[%s] Completed fetching L1 block timestamps", logPrefix), "total_batches", len(sequences), "duration", time.Since(startTime))

	return nil
}
