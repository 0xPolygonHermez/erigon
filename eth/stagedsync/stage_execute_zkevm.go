package stagedsync

import (
	"math/big"

	"github.com/ledgerwatch/erigon/chain"
	"github.com/ledgerwatch/erigon/zk/hermez_db"
	"github.com/ledgerwatch/erigon/zkevm/log"
)

func updateZkEVMBlockCfg(cfg *ExecuteBlockCfg, hermezDb *hermez_db.HermezDb, logPrefix string) error {
	update := func(forkId uint64, forkBlock **big.Int) error {
		if *forkBlock != nil && *forkBlock != big.NewInt(0) {
			return nil
		}
		blockNum, err := hermezDb.GetForkIdBlock(forkId)
		if err != nil {
			log.Errorf("[%s] Error getting fork id %v from db: %v", logPrefix, forkId, err)
			return err
		}
		if blockNum != 0 {
			*forkBlock = big.NewInt(0).SetUint64(blockNum)
			log.Infof("[%s] Set execute block cfg, fork id %v, block:%v, ", logPrefix, forkId, blockNum)
		}

		return nil
	}

	if err := update(chain.ForkID5Dragonfruit, &cfg.chainConfig.ForkID5DragonfruitBlock); err != nil {
		return err
	}
	if err := update(chain.ForkID6IncaBerry, &cfg.chainConfig.ForkID6IncaBerryBlock); err != nil {
		return err
	}
	if err := update(chain.ForkID7Etrog, &cfg.chainConfig.ForkID7EtrogBlock); err != nil {
		return err
	}
	return nil
}
