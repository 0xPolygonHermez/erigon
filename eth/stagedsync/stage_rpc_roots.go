package stagedsync

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"github.com/ledgerwatch/erigon-lib/chain"
	"github.com/ledgerwatch/erigon-lib/common/hexutility"
	"github.com/ledgerwatch/erigon-lib/kv"
	scalable "github.com/ledgerwatch/erigon/cmd/hack/zkevm"
	"github.com/ledgerwatch/log/v3"
	"os"
)

type RpcRootsCfg struct {
	db          kv.RwDB
	chainConfig *chain.Config
}

func StageRpcRootsCfg(db kv.RwDB, chainConfig *chain.Config) RpcRootsCfg {
	return RpcRootsCfg{db: db, chainConfig: chainConfig}
}

func RpcRootsForward(
	s *StageState,
	u Unwinder,
	ctx context.Context,
	tx kv.RwTx,
	cfg RpcRootsCfg,
	test bool, // Set to true in tests, allows the stage to fail rather than wait indefinitely
	firstCycle bool,
	quiet bool,
) error {

	if !firstCycle {
		return nil
	}

	// Checks for missing rpc hashes up to max. tx num and downloads them from the rpc
	scalable.DownloadScalableHashes(ctx, "zkevm-roots.json")

	useExternalTx := tx != nil
	if !useExternalTx {
		var err error
		tx, err = cfg.db.BeginRw(context.Background())
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}

	// loads zkevm-roots.json into the db
	err := loadHermezRpcRoots(tx)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func loadHermezRpcRoots(tx kv.RwTx) error {
	log.Info("Loading hermez rpc roots")

	rootsFile, err := os.Open("zkevm-roots.json")
	if err != nil {
		return err
	}
	defer rootsFile.Close()

	results := make(map[int64]string, 0)
	jsonParser := json.NewDecoder(rootsFile)
	err = jsonParser.Decode(&results)
	if err != nil {
		return err
	}

	for k, v := range results {
		b := make([]byte, 8)
		binary.BigEndian.PutUint64(b, uint64(k))
		bv := hexutility.FromHex(trimHexString(v))
		err = tx.Put("HermezRpcRoot", b, bv)
		if err != nil {
			return err
		}
	}

	return nil
}
