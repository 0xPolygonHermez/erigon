package l1infotree

import (
	"context"
	"encoding/json"
	"github.com/ledgerwatch/erigon/eth/ethconfig"
	zkTypes "github.com/ledgerwatch/erigon/zk/types"
	jsonClient "github.com/ledgerwatch/erigon/zkevm/jsonrpc/client"
	"github.com/ledgerwatch/log/v3"
	"sync/atomic"
	"time"
)

const (
	EXIT_ROOT_TABLE = "zkevm_getExitRootTable"
)

type L2InfoTreeSyncer struct {
	ctx            context.Context
	zkCfg          *ethconfig.Zk
	isSyncStarted  atomic.Bool
	isSyncFinished atomic.Bool
	infoTreeChan   chan []zkTypes.L1InfoTreeUpdate
}

func NewL2InfoTreeSyncer(ctx context.Context, zkCfg *ethconfig.Zk) *L2InfoTreeSyncer {
	return &L2InfoTreeSyncer{
		ctx:          ctx,
		zkCfg:        zkCfg,
		infoTreeChan: make(chan []zkTypes.L1InfoTreeUpdate),
	}
}

func (s *L2InfoTreeSyncer) IsSyncStarted() bool {
	return s.isSyncStarted.Load()
}

func (s *L2InfoTreeSyncer) IsSyncFinished() bool {
	return s.isSyncFinished.Load()
}

func (s *L2InfoTreeSyncer) GetInfoTreeChan() chan []zkTypes.L1InfoTreeUpdate {
	return s.infoTreeChan
}

func (s *L2InfoTreeSyncer) ConsumeInfoTree() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.infoTreeChan:
		default:
			if !s.isSyncStarted.Load() {
				return
			}
			time.Sleep(time.Second)
		}
	}
}

func (s *L2InfoTreeSyncer) RunSyncInfoTree() {
	if s.isSyncStarted.Load() {
		return
	}
	s.isSyncStarted.Store(true)
	s.isSyncFinished.Store(false)

	totalSynced := uint64(0)
	batchSize := s.zkCfg.L2InfoTreeUpdatesBatchSize

	go func() {
		retry := 0
		for {
			select {
			case <-s.ctx.Done():
				s.isSyncFinished.Store(true)
				break
			default:
				query := exitRootQuery{
					From: totalSynced + 1,
					To:   totalSynced + batchSize,
				}
				infoTree, err := getExitRootTable(s.zkCfg.L2RpcUrl, query)
				if err != nil {
					log.Debug("getExitRootTable retry error", "err", err)
					retry++
					if retry > 5 {
						return
					}
					time.Sleep(time.Duration(retry*2) * time.Second)
				}

				if len(infoTree) == 0 {
					s.isSyncFinished.Store(true)
					return
				}

				s.infoTreeChan <- infoTree
				totalSynced = query.To
			}
		}
	}()
}

type exitRootQuery struct {
	From uint64 `json:"from"`
	To   uint64 `json:"to"`
}

func getExitRootTable(endpoint string, query exitRootQuery) ([]zkTypes.L1InfoTreeUpdate, error) {
	res, err := jsonClient.JSONRPCCall(endpoint, EXIT_ROOT_TABLE, query)
	if err != nil {
		return nil, err
	}

	var updates []zkTypes.L1InfoTreeUpdate
	if err = json.Unmarshal(res.Result, &updates); err != nil {
		return nil, err
	}

	return updates, nil
}
