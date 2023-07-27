package stagedsync

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"github.com/ledgerwatch/erigon-lib/chain"
	"github.com/ledgerwatch/erigon-lib/common/hexutility"
	"github.com/ledgerwatch/erigon-lib/kv"
	scalable "github.com/ledgerwatch/erigon/cmd/hack/zkevm"
	"github.com/ledgerwatch/erigon/eth/stagedsync/stages"
	"github.com/ledgerwatch/log/v3"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
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

	useExternalTx := tx != nil
	if !useExternalTx {
		var err error
		tx, err = cfg.db.BeginRw(context.Background())
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}

	if !firstCycle {
		// TODO: non-first cycle handling
		// at the tip the algo below is inefficient (writing the whole json file)
		// in this case we should just write the DB with retrieved hashes and continue
		return nil
	}

	// call rpc to get latest block no
	txNo, err := getHighestTxNo()
	if err != nil {
		return err
	}

	prog, err := stages.GetStageProgress(tx, stages.RpcRoots)
	if err != nil {
		return err
	}

	if prog >= txNo {
		return nil
	}

	// checks for missing rpc hashes up to max. tx num and downloads them from the rpc
	scalable.DownloadScalableHashes(ctx, "zkevm-roots.json", int64(txNo))

	// loads zkevm-roots.json into the db
	err = loadHermezRpcRoots(tx)
	if err != nil {
		return err
	}

	err = stages.SaveStageProgress(tx, stages.RpcRoots, uint64(txNo))
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

func getHighestTxNo() (uint64, error) {
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

	resp, err := http.Post("https://zkevm-rpc.com", "application/json", bytes.NewBuffer(jsonData))
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
