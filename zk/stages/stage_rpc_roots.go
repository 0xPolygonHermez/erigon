package stages

import (
	"context"
	"encoding/binary"
	"fmt"

	"github.com/ledgerwatch/erigon-lib/common/hexutility"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon/chain"
	scalable "github.com/ledgerwatch/erigon/cmd/hack/zkevm"
	"github.com/ledgerwatch/erigon/core/state"
	"github.com/ledgerwatch/erigon/sync_stages"
	"github.com/ledgerwatch/log/v3"
)

type RpcRootsCfg struct {
	db          kv.RwDB
	chainConfig *chain.Config
	rpcEndpoint string
	isTestnet   bool
}

func StageRpcRootsCfg(db kv.RwDB, chainConfig *chain.Config, rpcEndpoint string, isTestnet bool) RpcRootsCfg {
	return RpcRootsCfg{db: db, chainConfig: chainConfig, rpcEndpoint: rpcEndpoint, isTestnet: isTestnet}
}

func SpawnStageRpcRoots(
	s *sync_stages.StageState,
	u sync_stages.Unwinder,
	ctx context.Context,
	tx kv.RwTx,
	cfg RpcRootsCfg,
	test bool, // Set to true in tests, allows the stage to fail rather than wait indefinitely
	firstCycle bool,
	quiet bool,
) error {

	logPrefix := s.LogPrefix()
	log.Info(fmt.Sprintf("[%s] Starting Rpc roots download stage", logPrefix))
	defer log.Info(fmt.Sprintf("[%s] Finished Rpc roots download stage ", logPrefix))

	useExternalTx := tx != nil
	if !useExternalTx {
		var err error
		tx, err = cfg.db.BeginRw(context.Background())
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}

	// call rpc to get latest block no
	txNo, err := RpcGetHighestTxNo(cfg.rpcEndpoint)
	if err != nil {
		return err
	}

	prog, err := sync_stages.GetStageProgress(tx, sync_stages.RpcRoots)
	if err != nil {
		return err
	}

	if prog >= txNo {
		log.Info(fmt.Sprintf("[%s] Already have the roots", logPrefix), "savedTxNo", prog, "highestTxNo", txNo)

		return nil
	}
	rpcFileName := "zkevm-roots.json"
	if cfg.isTestnet {
		rpcFileName = "zkevm-roots-testnet.json"
	}
	log.Info(fmt.Sprintf("[%s] Starting to download roots", logPrefix), "savedTxNo", prog, "highestTxNo", txNo)
	if !firstCycle || prog != 0 {
		res := scalable.DownloadScalableHashes(ctx, cfg.rpcEndpoint, logPrefix, rpcFileName, int64(txNo), false, int64(prog))

		if err := putRootsInDb(tx, res); err != nil {
			return err
		}

		if err := sync_stages.SaveStageProgress(tx, sync_stages.RpcRoots, txNo); err != nil {
			return err
		}

		if !useExternalTx {
			return tx.Commit()
		}
		return nil
	}

	// checks for missing rpc hashes up to max. tx num and downloads them from the rpc
	res := scalable.DownloadScalableHashes(ctx, cfg.rpcEndpoint, logPrefix, rpcFileName, int64(txNo), true, 1)

	if err := putRootsInDb(tx, res); err != nil {
		return err
	}

	log.Info(fmt.Sprintf("[%s] Finished downloading roots. Saving progress", logPrefix))

	if err := sync_stages.SaveStageProgress(tx, sync_stages.RpcRoots, uint64(txNo)); err != nil {
		return err
	}

	if !useExternalTx {
		return tx.Commit()
	}

	return nil
}

func UnwindRpcRootsStage(u *sync_stages.UnwindState, tx kv.RwTx, cfg RpcRootsCfg, ctx context.Context) (err error) {
	useExternalTx := tx != nil
	if !useExternalTx {
		tx, err = cfg.db.BeginRw(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}

	//TODO: implement unwind verifications stage!
	return nil
}

func PruneRpcRootsStage(s *sync_stages.PruneState, tx kv.RwTx, cfg RpcRootsCfg, ctx context.Context) (err error) {
	useExternalTx := tx != nil
	if !useExternalTx {
		tx, err = cfg.db.BeginRw(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}

	//TODO: implement prune verifications stage!
	return nil
}

func putRootsInDb(tx kv.RwTx, results map[int64]string) error {
	for k, v := range results {
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, uint64(k))
		bv := hexutility.FromHex(TrimHexString(v))
		if err := tx.Put(state.RpcRootsBucketName, b, bv); err != nil {
			return err
		}
	}
	return nil
}
