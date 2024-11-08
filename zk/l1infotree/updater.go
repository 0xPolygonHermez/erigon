package l1infotree

import (
	"github.com/ledgerwatch/erigon/eth/ethconfig"
	"github.com/ledgerwatch/erigon/zk/hermez_db"
	"github.com/ledgerwatch/erigon/eth/stagedsync/stages"
	zkTypes "github.com/ledgerwatch/erigon/zk/types"
	"github.com/ledgerwatch/erigon/core/types"
	"time"
	"github.com/ledgerwatch/erigon/zk/contracts"
	"github.com/ledgerwatch/log/v3"
	"fmt"
	"sort"
	"github.com/iden3/go-iden3-crypto/keccak256"
	"errors"
	"github.com/gateway-fm/cdk-erigon-lib/kv"
	"github.com/gateway-fm/cdk-erigon-lib/common"
)

type Syncer interface {
	IsSyncStarted() bool
	Run(lastCheckedBlock uint64)
	GetLogsChan() chan []types.Log
	GetProgressMessageChan() chan string
	IsDownloading() bool
	GetHeader(blockNumber uint64) (*types.Header, error)
	L1QueryHeaders(logs []types.Log) (map[uint64]*types.Header, error)
	Stop()
}

type Updater struct {
	cfg          *ethconfig.Zk
	syncer       Syncer
	progress     uint64
	latestUpdate *zkTypes.L1InfoTreeUpdate
}

func NewUpdater(cfg *ethconfig.Zk, syncer Syncer) *Updater {
	return &Updater{
		cfg:    cfg,
		syncer: syncer,
	}
}

func (u *Updater) GetProgress() uint64 {
	return u.progress
}

func (u *Updater) GetLatestUpdate() *zkTypes.L1InfoTreeUpdate {
	return u.latestUpdate
}

func (u *Updater) WarmUp(tx kv.RwTx) (err error) {
	defer func() {
		if err != nil {
			u.syncer.Stop()
		}
	}()

	hermezDb := hermez_db.NewHermezDb(tx)

	progress, err := stages.GetStageProgress(tx, stages.L1InfoTree)
	if err != nil {
		return err
	}
	if progress == 0 {
		progress = u.cfg.L1FirstBlock - 1
	}

	u.progress = progress

	latestUpdate, err := hermezDb.GetLatestL1InfoTreeUpdate()
	if err != nil {
		return err
	}

	u.latestUpdate = latestUpdate

	if !u.syncer.IsSyncStarted() {
		u.syncer.Run(u.progress)
	}

	return nil
}

func (u *Updater) CheckForInfoTreeUpdates(logPrefix string, tx kv.RwTx) (allLogs []types.Log, err error) {
	defer func() {
		if err != nil {
			u.syncer.Stop()
		}
	}()

	hermezDb := hermez_db.NewHermezDb(tx)
	logChan := u.syncer.GetLogsChan()
	progressChan := u.syncer.GetProgressMessageChan()

	// first get all the logs we need to process
LOOP:
	for {
		select {
		case logs := <-logChan:
			allLogs = append(allLogs, logs...)
		case msg := <-progressChan:
			log.Info(fmt.Sprintf("[%s] %s", logPrefix, msg))
		default:
			if !u.syncer.IsDownloading() {
				break LOOP
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	// sort the logs by block number - it is important that we process them in order to get the index correct
	sort.Slice(allLogs, func(i, j int) bool {
		l1 := allLogs[i]
		l2 := allLogs[j]
		// first sort by block number and if equal then by tx index
		if l1.BlockNumber != l2.BlockNumber {
			return l1.BlockNumber < l2.BlockNumber
		}
		if l1.TxIndex != l2.TxIndex {
			return l1.TxIndex < l2.TxIndex
		}
		return l1.Index < l2.Index
	})

	// chunk the logs into batches, so we don't overload the RPC endpoints too much at once
	chunks := chunkLogs(allLogs, 50)

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	processed := 0

	tree, err := initialiseL1InfoTree(hermezDb)
	if err != nil {
		return nil, err
	}

	// process the logs in chunks
	for _, chunk := range chunks {
		select {
		case <-ticker.C:
			log.Info(fmt.Sprintf("[%s] Processed %d/%d logs, %d%% complete", logPrefix, processed, len(allLogs), processed*100/len(allLogs)))
		default:
		}

		headersMap, err := u.syncer.L1QueryHeaders(chunk)
		if err != nil {
			return nil, err
		}

		for _, l := range chunk {
			switch l.Topics[0] {
			case contracts.UpdateL1InfoTreeTopic:
				header := headersMap[l.BlockNumber]
				if header == nil {
					header, err = u.syncer.GetHeader(l.BlockNumber)
					if err != nil {
						return nil, err
					}
				}

				tmpUpdate, err := createL1InfoTreeUpdate(l, header)
				if err != nil {
					return nil, err
				}

				leafHash := HashLeafData(tmpUpdate.GER, tmpUpdate.ParentHash, tmpUpdate.Timestamp)
				if tree.LeafExists(leafHash) {
					log.Warn("Skipping log as L1 Info Tree leaf already exists", "hash", leafHash)
					continue
				}

				if u.latestUpdate != nil {
					tmpUpdate.Index = u.latestUpdate.Index + 1
				} // if latestUpdate is nil then Index = 0 which is the default value so no need to set it
				u.latestUpdate = tmpUpdate

				newRoot, err := tree.AddLeaf(uint32(u.latestUpdate.Index), leafHash)
				if err != nil {
					return nil, err
				}
				log.Debug("New L1 Index",
					"index", u.latestUpdate.Index,
					"root", newRoot.String(),
					"mainnet", u.latestUpdate.MainnetExitRoot.String(),
					"rollup", u.latestUpdate.RollupExitRoot.String(),
					"ger", u.latestUpdate.GER.String(),
					"parent", u.latestUpdate.ParentHash.String(),
				)

				if err = handleL1InfoTreeUpdate(hermezDb, u.latestUpdate); err != nil {
					return nil, err
				}
				if err = hermezDb.WriteL1InfoTreeLeaf(u.latestUpdate.Index, leafHash); err != nil {
					return nil, err
				}
				if err = hermezDb.WriteL1InfoTreeRoot(common.BytesToHash(newRoot[:]), u.latestUpdate.Index); err != nil {
					return nil, err
				}

				processed++
			default:
				log.Warn("received unexpected topic from l1 info tree stage", "topic", l.Topics[0])
			}
		}
	}

	// save the progress - we add one here so that we don't cause overlap on the next run.  We don't want to duplicate an info tree update in the db
	if len(allLogs) > 0 {
		u.progress = allLogs[len(allLogs)-1].BlockNumber + 1
	}
	if err = stages.SaveStageProgress(tx, stages.L1InfoTree, u.progress); err != nil {
		return nil, err
	}

	return allLogs, nil
}

func chunkLogs(slice []types.Log, chunkSize int) [][]types.Log {
	var chunks [][]types.Log
	for i := 0; i < len(slice); i += chunkSize {
		end := i + chunkSize

		// If end is greater than the length of the slice, reassign it to the length of the slice
		if end > len(slice) {
			end = len(slice)
		}

		chunks = append(chunks, slice[i:end])
	}
	return chunks
}

func initialiseL1InfoTree(hermezDb *hermez_db.HermezDb) (*L1InfoTree, error) {
	leaves, err := hermezDb.GetAllL1InfoTreeLeaves()
	if err != nil {
		return nil, err
	}

	allLeaves := make([][32]byte, len(leaves))
	for i, l := range leaves {
		allLeaves[i] = l
	}

	tree, err := NewL1InfoTree(32, allLeaves)
	if err != nil {
		return nil, err
	}

	return tree, nil
}

func createL1InfoTreeUpdate(l types.Log, header *types.Header) (*zkTypes.L1InfoTreeUpdate, error) {
	if len(l.Topics) != 3 {
		return nil, errors.New("received log for info tree that did not have 3 topics")
	}

	if l.BlockNumber != header.Number.Uint64() {
		return nil, errors.New("received log for info tree that did not match the block number")
	}

	mainnetExitRoot := l.Topics[1]
	rollupExitRoot := l.Topics[2]
	combined := append(mainnetExitRoot.Bytes(), rollupExitRoot.Bytes()...)
	ger := keccak256.Hash(combined)
	update := &zkTypes.L1InfoTreeUpdate{
		GER:             common.BytesToHash(ger),
		MainnetExitRoot: mainnetExitRoot,
		RollupExitRoot:  rollupExitRoot,
		BlockNumber:     l.BlockNumber,
		Timestamp:       header.Time,
		ParentHash:      header.ParentHash,
	}

	return update, nil
}

func handleL1InfoTreeUpdate(
	hermezDb *hermez_db.HermezDb,
	update *zkTypes.L1InfoTreeUpdate,
) error {
	var err error
	if err = hermezDb.WriteL1InfoTreeUpdate(update); err != nil {
		return err
	}
	if err = hermezDb.WriteL1InfoTreeUpdateToGer(update); err != nil {
		return err
	}
	return nil
}
