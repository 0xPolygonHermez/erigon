package witness

import (
	"bytes"
	"context"
	"fmt"

	coreState "github.com/ledgerwatch/erigon/core/state"
	db2 "github.com/ledgerwatch/erigon/smt/pkg/db"
	"github.com/ledgerwatch/erigon/smt/pkg/smt"
	"github.com/ledgerwatch/erigon/turbo/trie"

	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/common/datadir"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/state"
	"github.com/ledgerwatch/erigon/core/rawdb"
	eritypes "github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/eth/stagedsync"
	dstypes "github.com/ledgerwatch/erigon/zk/datastream/types"
	zkSmt "github.com/ledgerwatch/erigon/zk/smt"
	zkUtils "github.com/ledgerwatch/erigon/zk/utils"
	"github.com/ledgerwatch/log/v3"
)

func UnwindForWitness(ctx context.Context, tx kv.RwTx, startBlock, latestBlock uint64, dirs datadir.Dirs, historyV3 bool, agg *state.Aggregator) (err error) {
	unwindState := &stagedsync.UnwindState{UnwindPoint: startBlock - 1}
	stageState := &stagedsync.StageState{BlockNumber: latestBlock}

	hashStageCfg := stagedsync.StageHashStateCfg(nil, dirs, historyV3, agg)
	if err := stagedsync.UnwindHashStateStage(unwindState, stageState, tx, hashStageCfg, ctx, log.New(), true); err != nil {
		return fmt.Errorf("UnwindHashStateStage: %w", err)
	}

	var expectedRootHash common.Hash
	syncHeadHeader, err := rawdb.ReadHeaderByNumber_zkevm(tx, unwindState.UnwindPoint)
	if err != nil {
		return fmt.Errorf("ReadHeaderByNumber_zkevm for block %d: %v", unwindState.UnwindPoint, err)
	}
	if syncHeadHeader == nil {
		log.Warn("header not found for block number", "block", unwindState.UnwindPoint)
	} else {
		expectedRootHash = syncHeadHeader.Root
	}

	if _, err := zkSmt.UnwindZkSMT(ctx, "api.generateWitness", stageState.BlockNumber, unwindState.UnwindPoint, tx, true, &expectedRootHash, true, ctx.Done()); err != nil {
		return fmt.Errorf("UnwindZkSMT: %w", err)
	}

	return nil
}

type gerForWitnessDb interface {
	GetBatchNoByL2Block(blockNum uint64) (uint64, error)
	GetBatchGlobalExitRoots(lastBatch, currentBatch uint64) (*[]dstypes.GerUpdate, error)
	GetBlockGlobalExitRoot(blockNum uint64) (common.Hash, error)
}

func PrepareGersForWitness(block *eritypes.Block, db gerForWitnessDb, tds *coreState.TrieDbState, trieStateWriter *coreState.TrieStateWriter) error {
	blockNum := block.NumberU64()
	//[zkevm] get batches between last block and this one
	// plus this blocks ger
	lastBatchInserted, err := db.GetBatchNoByL2Block(blockNum - 1)
	if err != nil {
		return fmt.Errorf("GetBatchNoByL2Block for block %d: %w", blockNum-1, err)
	}

	currentBatch, err := db.GetBatchNoByL2Block(blockNum)
	if err != nil {
		return fmt.Errorf("GetBatchNoByL2Block for block %d: %v", blockNum, err)
	}

	gersInBetween, err := db.GetBatchGlobalExitRoots(lastBatchInserted, currentBatch)
	if err != nil {
		return fmt.Errorf("GetBatchGlobalExitRoots for block %d: %v", blockNum, err)
	}

	var globalExitRoots []dstypes.GerUpdate

	if gersInBetween != nil {
		globalExitRoots = append(globalExitRoots, *gersInBetween...)
	}

	blockGer, err := db.GetBlockGlobalExitRoot(blockNum)
	if err != nil {
		return fmt.Errorf("GetBlockGlobalExitRoot for block %d: %v", blockNum, err)
	}
	emptyHash := common.Hash{}

	if blockGer != emptyHash {
		blockGerUpdate := dstypes.GerUpdate{
			GlobalExitRoot: blockGer,
			Timestamp:      block.Header().Time,
		}
		globalExitRoots = append(globalExitRoots, blockGerUpdate)
	}

	for _, ger := range globalExitRoots {
		// [zkevm] - add GER if there is one for this batch
		if err := zkUtils.WriteGlobalExitRoot(tds, trieStateWriter, ger.GlobalExitRoot, ger.Timestamp); err != nil {
			return fmt.Errorf("WriteGlobalExitRoot: %w", err)
		}
	}

	return nil
}

type trieDbState interface {
	ResolveSMTRetainList() (*trie.RetainList, error)
}

func BuildWitnessFromTrieDbState(ctx context.Context, tx kv.Tx, tds trieDbState, witnessFull bool) (witness *trie.Witness, err error) {
	var rl trie.RetainDecider
	// if full is true, we will send all the nodes to the witness
	rl = &trie.AlwaysTrueRetainDecider{}

	if !witnessFull {
		rl, err = tds.ResolveSMTRetainList()
		if err != nil {
			return nil, err
		}
	}

	eridb := db2.NewRoEriDb(tx)
	smtTrie := smt.NewRoSMT(eridb)

	witness, err = smt.BuildWitness(smtTrie, rl, ctx)
	if err != nil {
		return nil, fmt.Errorf("BuildWitness: %w", err)
	}

	return
}

func GetWitnessBytes(witness *trie.Witness, debug bool) ([]byte, error) {
	var buf bytes.Buffer
	_, err := witness.WriteInto(&buf, debug)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
