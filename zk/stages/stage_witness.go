package stages

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	libcommon "github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/common/datadir"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/kv/memdb"
	libstate "github.com/ledgerwatch/erigon-lib/state"
	"github.com/ledgerwatch/erigon/chain"
	"github.com/ledgerwatch/erigon/consensus"
	"github.com/ledgerwatch/erigon/core"
	"github.com/ledgerwatch/erigon/core/rawdb"
	"github.com/ledgerwatch/erigon/core/state"
	"github.com/ledgerwatch/erigon/core/systemcontracts"
	eritypes "github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/core/vm"
	"github.com/ledgerwatch/erigon/eth/stagedsync"
	"github.com/ledgerwatch/erigon/eth/stagedsync/stages"
	db2 "github.com/ledgerwatch/erigon/smt/pkg/db"
	"github.com/ledgerwatch/erigon/smt/pkg/smt"
	"github.com/ledgerwatch/erigon/turbo/services"
	"github.com/ledgerwatch/erigon/turbo/trie"
	dstypes "github.com/ledgerwatch/erigon/zk/datastream/types"
	"github.com/ledgerwatch/erigon/zk/hermez_db"
	"github.com/ledgerwatch/erigon/zk/sequencer"
	zkUtils "github.com/ledgerwatch/erigon/zk/utils"
	"github.com/ledgerwatch/log/v3"
)

type WitnessCfg struct {
	db                 kv.RwDB
	tmpDir             datadir.Dirs
	blockReader        services.FullBlockReader
	engine             consensus.Engine
	chainConfig        *chain.Config
	agg                *libstate.AggregatorV3
	historyV3          bool
	enableWitnessStage bool
	storeNblocks       uint64
}

func GenerateWitnessCfg(db kv.RwDB, tmpDir datadir.Dirs, blockReader services.FullBlockReader, engine consensus.Engine, chainConfig *chain.Config, agg *libstate.AggregatorV3, historyV3 bool, enableWitnessStage bool, storeNblocks uint64) WitnessCfg {
	return WitnessCfg{
		db:                 db,
		tmpDir:             tmpDir,
		blockReader:        blockReader,
		engine:             engine,
		chainConfig:        chainConfig,
		agg:                agg,
		historyV3:          historyV3,
		enableWitnessStage: enableWitnessStage,
		storeNblocks:       storeNblocks,
	}
}

func SpawnWitnessStage(
	s *stagedsync.StageState,
	u stagedsync.Unwinder,
	tx kv.RwTx,
	ctx context.Context,
	cfg WitnessCfg,
	initialCycle bool,
	quiet bool,
) error {
	logPrefix := s.LogPrefix()
	if !cfg.enableWitnessStage {
		log.Info(fmt.Sprintf("[%s] Skipping witness stage", logPrefix))
		return nil
	}

	log.Info(fmt.Sprintf("[%s] Starting witness stage", logPrefix))
	defer log.Info(fmt.Sprintf("[%s] Finished witness stage", logPrefix))

	if sequencer.IsSequencer() {
		log.Info(fmt.Sprintf("[%s] skipping -- sequencer", logPrefix))
		return nil
	}

	if tx == nil {
		log.Debug(fmt.Sprintf("[%s] witness: no tx provided, creating a new one", logPrefix))
		var err error
		tx, err = cfg.db.BeginRw(ctx)
		if err != nil {
			return fmt.Errorf("failed to open tx, %w", err)
		}
		defer tx.Rollback()
	}

	// batch until which witness has been generated and saved to db
	witnessProgress, err := stages.GetStageProgress(tx, stages.Witness)
	if err != nil {
		return fmt.Errorf("save stage progress error: %v", err)
	}

	// batch which has been processed by stage_batches
	latestBlock, err := stages.GetStageProgress(tx, stages.Execution)
	if err != nil {
		return fmt.Errorf("could not retrieve batch no progress")
	}

	hermezDbReader := hermez_db.NewHermezDbReader(tx)

	batchesProgress, err := hermezDbReader.GetBatchNoByL2Block(latestBlock)
	if err != nil {
		return fmt.Errorf("could not retrieve batch for latest block")
	}

	if witnessProgress >= batchesProgress {
		log.Info("All batch witness already in db, skipping")
		return nil
	}

	// witness generation should happen till batch of latest block -1
	// as there might be more blocks incoming for this batch
	witnessToBatch := batchesProgress - 1

	highestBlockInLatestBatch, err := hermezDbReader.GetHighestBlockInBatch(witnessToBatch)
	if err != nil {
		return err
	}

	blockNblocksBehind := highestBlockInLatestBatch - cfg.storeNblocks + 1
	witnessFromBatch, err := hermezDbReader.GetBatchNoByL2Block(blockNblocksBehind)
	if err != nil {
		return err
	}

	wg := NewWitnessGenerator(cfg.tmpDir, cfg.historyV3, cfg.agg, cfg.blockReader, cfg.chainConfig, cfg.engine)

	for b := witnessFromBatch; b <= witnessToBatch; b++ {
		highestBlockInBatch, err := hermezDbReader.GetHighestBlockInBatch(b)
		if err != nil {
			return err
		}

		if highestBlockInBatch == 0 {
			continue
		}

		nextStageProgress, err := stages.GetStageProgress(tx, stages.HashState)
		if err != nil {
			return err
		}
		if nextStageProgress < highestBlockInBatch {
			// Skip this stage
			return nil
		}

		lowestBlockInBatch, err := hermezDbReader.GetLowestBlockInBatch(b)
		if err != nil {
			return err
		}

		// Skip witness generation for this batch as it is bound to fail
		if highestBlockInBatch-lowestBlockInBatch > maxGetProofRewindBlockCount {
			continue
		}

		log.Info(fmt.Sprintf("[%s] Starting witness generation", logPrefix), "currentBatch", b, "highestBatch", witnessToBatch)

		hermezDb := hermez_db.NewHermezDb(tx)

		// If there is an existing witness delete it first
		// Possible in case of reorg
		w, err := hermezDb.GetWitnessByBatchNo(b)
		if err != nil {
			return err
		}
		if w != nil {
			err = hermezDb.DeleteWitnessByBatchNo(b)
			if err != nil {
				return err
			}
		}

		w, err = wg.GenerateWitnessByBatch(tx, ctx, b, false)
		if err != nil {
			return err
		}
		err = hermezDb.WriteWitnessByBatchNo(b, w)
		if err != nil {
			return err
		}

		err = stages.SaveStageProgress(tx, stages.Witness, b)
		if err != nil {
			return err
		}
	}

	// Delete additional batches
	hermezDb := hermez_db.NewHermezDb(tx)
	err = hermezDb.DeleteWitnessTillBatchNo(witnessFromBatch - 1)
	if err != nil {
		return err
	}

	return nil
}

func UnwindWitnessStage(
	u *stagedsync.UnwindState,
	s *stagedsync.StageState,
	tx kv.RwTx,
	ctx context.Context,
	cfg WitnessCfg,
	initialCycle bool,
) (err error) {
	logPrefix := u.LogPrefix()
	if !cfg.enableWitnessStage {
		log.Info(fmt.Sprintf("[%s] Skipping witness stage", logPrefix))
		return nil
	}

	useExternalTx := tx != nil
	if !useExternalTx {
		tx, err = cfg.db.BeginRw(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}

	fromBlock := u.UnwindPoint
	toBlock := u.CurrentBlockNumber
	log.Info(fmt.Sprintf("[%s] Unwinding witness stage from block number", logPrefix), "fromBlock", fromBlock, "toBlock", toBlock)
	defer log.Info(fmt.Sprintf("[%s] Unwinding witness complete", logPrefix))

	hermezDb := hermez_db.NewHermezDb(tx)
	fromBatch, err := hermezDb.GetBatchNoByL2Block(fromBlock)
	if err != nil {
		return fmt.Errorf("get batch no by l2 block error: %v", err)
	}
	toBatch, err := hermezDb.GetBatchNoByL2Block(toBlock)
	if err != nil {
		return fmt.Errorf("get batch no by l2 block error: %v", err)
	}

	hermezDb.DeleteWitnessByBatchRange(fromBatch, toBatch)

	if !useExternalTx {
		if err := tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}

func PruneWitnessStage(
	s *stagedsync.PruneState,
	tx kv.RwTx,
	cfg WitnessCfg,
	ctx context.Context,
	initialCycle bool,
) (err error) {
	logPrefix := s.LogPrefix()
	if !cfg.enableWitnessStage {
		log.Info(fmt.Sprintf("[%s] Skipping witness stage", logPrefix))
		return nil
	}

	useExternalTx := tx != nil
	if !useExternalTx {
		tx, err = cfg.db.BeginRw(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}

	log.Info(fmt.Sprintf("[%s] Pruning witness...", logPrefix))
	defer log.Info(fmt.Sprintf("[%s] Unwinding witness complete", logPrefix))

	hermezDb := hermez_db.NewHermezDb(tx)

	toBatch, err := stages.GetStageProgress(tx, stages.Witness)
	if err != nil {
		return fmt.Errorf("get stage witness progress error: %v", err)
	}

	hermezDb.DeleteWitnessTillBatchNo(toBatch)

	log.Info(fmt.Sprintf("[%s] Deleted witness.", logPrefix))
	log.Info(fmt.Sprintf("[%s] Saving stage progress", logPrefix), "stageProgress", 0)
	if err := stages.SaveStageProgress(tx, stages.Batches, 0); err != nil {
		return fmt.Errorf("save stage progress error: %v", err)
	}

	if !useExternalTx {
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

var (
	maxGetProofRewindBlockCount uint64 = 1_000

	ErrEndBeforeStart = errors.New("end block must be higher than start block")
)

type WitnessGenerator struct {
	dirs        datadir.Dirs
	historyV3   bool
	agg         *libstate.AggregatorV3
	blockReader services.FullBlockReader
	chainCfg    *chain.Config
	engine      consensus.EngineReader
}

func NewWitnessGenerator(
	dirs datadir.Dirs,
	historyV3 bool,
	agg *libstate.AggregatorV3,
	blockReader services.FullBlockReader,
	chainCfg *chain.Config,
	engine consensus.EngineReader,
) *WitnessGenerator {
	return &WitnessGenerator{
		dirs:        dirs,
		historyV3:   historyV3,
		agg:         agg,
		blockReader: blockReader,
		chainCfg:    chainCfg,
		engine:      engine,
	}
}

func (g *WitnessGenerator) GenerateWitnessByBatch(tx kv.Tx, ctx context.Context, batch uint64, debug bool) ([]byte, error) {
	hermezDb := hermez_db.NewHermezDbReader(tx)

	startBlock, err := hermezDb.GetLowestBlockInBatch(batch)
	if err != nil {
		return nil, err
	}

	endBlock, err := hermezDb.GetHighestBlockInBatch(batch)
	if err != nil {
		return nil, err
	}

	// Witness of 0 block is empty
	if startBlock == 0 {
		startBlock = 1
	}

	return g.GenerateWitness(tx, ctx, startBlock, endBlock, debug)
}

func (g *WitnessGenerator) GenerateWitness(tx kv.Tx, ctx context.Context, startBlock, endBlock uint64, debug bool) ([]byte, error) {
	if startBlock > endBlock {
		return nil, ErrEndBeforeStart
	}

	if endBlock == 0 {
		witness := trie.NewWitness([]trie.WitnessOperator{})
		return getWitnessBytes(witness, debug)
	}

	latestBlock, err := stages.GetStageProgress(tx, stages.Execution)
	if err != nil {
		return nil, err
	}

	if latestBlock < endBlock {
		return nil, fmt.Errorf("block number is in the future latest=%d requested=%d", latestBlock, endBlock)
	}

	batch := memdb.NewMemoryBatch(tx, g.dirs.Tmp)
	defer batch.Rollback()
	if err = populateDbTables(batch); err != nil {
		return nil, err
	}

	sBlock, err := rawdb.ReadBlockByNumber(tx, startBlock)
	if err != nil {
		return nil, err
	}
	if sBlock == nil {
		return nil, nil
	}

	if startBlock-1 < latestBlock {
		if latestBlock-startBlock > maxGetProofRewindBlockCount {
			return nil, fmt.Errorf("requested block is too old, block must be within %d blocks of the head block number (currently %d)", maxGetProofRewindBlockCount, latestBlock)
		}

		unwindState := &stagedsync.UnwindState{UnwindPoint: startBlock - 1}
		stageState := &stagedsync.StageState{BlockNumber: latestBlock}

		hashStageCfg := stagedsync.StageHashStateCfg(nil, g.dirs, g.historyV3, g.agg)
		hashStageCfg.SetQuiet(true)
		if err := stagedsync.UnwindHashStateStage(unwindState, stageState, batch, hashStageCfg, ctx); err != nil {
			return nil, err
		}

		interHashStageCfg := StageZkInterHashesCfg(nil, true, true, false, g.dirs.Tmp, g.blockReader, nil, g.historyV3, g.agg, nil)

		err = UnwindZkIntermediateHashesStage(unwindState, stageState, batch, interHashStageCfg, ctx)
		if err != nil {
			return nil, err
		}

		tx = batch
	}

	prevHeader, err := g.blockReader.HeaderByNumber(ctx, tx, startBlock-1)
	if err != nil {
		return nil, err
	}

	tds := state.NewTrieDbState(prevHeader.Root, tx, startBlock-1, nil)
	tds.SetResolveReads(true)
	tds.StartNewBuffer()
	trieStateWriter := tds.TrieStateWriter()

	getHeader := func(hash libcommon.Hash, number uint64) *eritypes.Header {
		h, e := g.blockReader.Header(ctx, tx, hash, number)
		if e != nil {
			log.Error("getHeader error", "number", number, "hash", hash, "err", e)
		}
		return h
	}

	for blockNum := startBlock; blockNum <= endBlock; blockNum++ {
		block, err := rawdb.ReadBlockByNumber(tx, blockNum)

		if err != nil {
			return nil, err
		}

		reader := state.NewPlainState(tx, blockNum, systemcontracts.SystemContractCodeLookup[g.chainCfg.ChainName])

		tds.SetStateReader(reader)

		hermezDb := hermez_db.NewHermezDbReader(tx)

		//[zkevm] get batches between last block and this one
		// plus this blocks ger
		lastBatchInserted, err := hermezDb.GetBatchNoByL2Block(blockNum - 1)
		if err != nil {
			return nil, fmt.Errorf("failed to get batch for block %d: %v", blockNum-1, err)
		}

		currentBatch, err := hermezDb.GetBatchNoByL2Block(blockNum)
		if err != nil {
			return nil, fmt.Errorf("failed to get batch for block %d: %v", blockNum, err)
		}

		gersInBetween, err := hermezDb.GetBatchGlobalExitRoots(lastBatchInserted, currentBatch)
		if err != nil {
			return nil, err
		}

		var globalExitRoots []*dstypes.GerUpdate

		if gersInBetween != nil {
			globalExitRoots = append(globalExitRoots, gersInBetween...)
		}

		blockGer, err := hermezDb.GetBlockGlobalExitRoot(blockNum)
		if err != nil {
			return nil, err
		}
		emptyHash := libcommon.Hash{}

		if blockGer != emptyHash {
			blockGerUpdate := dstypes.GerUpdate{
				GlobalExitRoot: blockGer,
				Timestamp:      block.Header().Time,
			}
			globalExitRoots = append(globalExitRoots, &blockGerUpdate)
		}

		for _, ger := range globalExitRoots {
			// [zkevm] - add GER if there is one for this batch
			if err := zkUtils.WriteGlobalExitRoot(tds, trieStateWriter, ger.GlobalExitRoot, ger.Timestamp); err != nil {
				return nil, err
			}
		}

		engine, ok := g.engine.(consensus.Engine)

		if !ok {
			return nil, fmt.Errorf("engine is not consensus.Engine")
		}

		vmConfig := vm.Config{}

		getHashFn := core.GetHashFn(block.Header(), getHeader)

		chainReader := stagedsync.NewChainReaderImpl(g.chainCfg, tx, nil)

		_, err = core.ExecuteBlockEphemerally(g.chainCfg, &vmConfig, getHashFn, engine, block, tds, trieStateWriter, chainReader, nil, nil, hermezDb)

		if err != nil {
			return nil, err
		}
	}

	// todo [zkevm] we need to use this retain list rather than using the always true retain decider
	rl, err := tds.ResolveSMTRetainList()
	if err != nil {
		return nil, err
	}

	// if you ever need to send the full witness then you can use this always true trimmer and the whole state will be sent
	//rl := &trie.AlwaysTrueRetainDecider{}

	eridb := db2.NewEriDb(batch)
	smtTrie := smt.NewSMT(eridb)

	witness, err := smt.BuildWitness(smtTrie, rl, ctx)
	if err != nil {
		return nil, err
	}

	return getWitnessBytes(witness, debug)
}

func getWitnessBytes(witness *trie.Witness, debug bool) ([]byte, error) {
	var buf bytes.Buffer
	_, err := witness.WriteInto(&buf, debug)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func populateDbTables(batch *memdb.MemoryMutation) error {
	tables := []string{
		db2.TableSmt,
		db2.TableAccountValues,
		db2.TableMetadata,
		db2.TableHashKey,
		db2.TableLastRoot,
		hermez_db.TX_PRICE_PERCENTAGE,
		hermez_db.BLOCKBATCHES,
		hermez_db.BLOCK_GLOBAL_EXIT_ROOTS,
		hermez_db.GLOBAL_EXIT_ROOTS_BATCHES,
		hermez_db.STATE_ROOTS,
	}

	for _, t := range tables {
		if err := batch.CreateBucket(t); err != nil {
			return err
		}
	}

	return nil
}
