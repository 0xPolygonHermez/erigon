package stages

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"

	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

	"github.com/ledgerwatch/erigon-lib/common/hexutility"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon/chain"
	scalable "github.com/ledgerwatch/erigon/cmd/hack/zkevm"
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
	txNo, err := getHighestTxNo(cfg.rpcEndpoint)

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
	log.Info(fmt.Sprintf("[%s] Starting to wodnload roots", logPrefix), "savedTxNo", prog, "highestTxNo", txNo)
	if !firstCycle || prog != 0 {
		res := scalable.DownloadScalableHashes(ctx, logPrefix, cfg.rpcEndpoint, rpcFileName, int64(txNo), false, int64(prog))

		err = putRootsInDb(tx, res)
		if err != nil {
			return err
		}

		err = sync_stages.SaveStageProgress(tx, sync_stages.RpcRoots, txNo)
		if err != nil {
			return err
		}

		if !useExternalTx {
			return tx.Commit()
		}
		return nil
	}

	// checks for missing rpc hashes up to max. tx num and downloads them from the rpc
	res := scalable.DownloadScalableHashes(ctx, cfg.rpcEndpoint, logPrefix, rpcFileName, int64(txNo), true, 1)

	err = putRootsInDb(tx, res)
	if err != nil {
		return err
	}

	log.Info(fmt.Sprintf("[%s] Finished downloading roots. Saving progress", logPrefix))

	err = sync_stages.SaveStageProgress(tx, sync_stages.RpcRoots, uint64(txNo))
	if err != nil {
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
		bv := hexutility.FromHex(trimHexString(v))
		err := tx.Put("HermezRpcRoot", b, bv)
		if err != nil {
			return err
		}
	}
	return nil
}

func getHighestTxNo(rpcEndpoint string) (uint64, error) {

	data := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "eth_getBlockByNumber",
		"params":  []interface{}{"latest", true},
		"id":      1,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return 0, err
	}

	resp, err := http.Post(rpcEndpoint, "application/json", bytes.NewBuffer(jsonData))

	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)

	var result map[string]interface{}
	err = json.Unmarshal(body, &result)
	if err != nil {
		return 0, err
	}

	number := result["result"].(map[string]interface{})["number"]
	hexString := strings.TrimPrefix(number.(string), "0x")
	val, err := strconv.ParseUint(hexString, 16, 64)
	if err != nil {
		return 0, err
	}

	return val, nil
}
