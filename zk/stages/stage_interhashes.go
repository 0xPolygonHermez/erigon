package stages

import (
	"bytes"
	"fmt"
	"os"

	libcommon "github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/common/length"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/kv/rawdbv3"
	"github.com/ledgerwatch/erigon-lib/state"
	state2 "github.com/ledgerwatch/erigon/core/state"
	"github.com/ledgerwatch/erigon/core/types"
	db2 "github.com/ledgerwatch/erigon/smt/pkg/db"
	"github.com/ledgerwatch/erigon/smt/pkg/smt"
	"github.com/ledgerwatch/erigon/smt/pkg/utils"
	"github.com/ledgerwatch/log/v3"

	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"

	"context"
	"math/big"
	"time"

	"github.com/ledgerwatch/erigon/common/dbutils"
	"github.com/ledgerwatch/erigon/core/systemcontracts"
	"github.com/ledgerwatch/erigon/core/types/accounts"
	"github.com/ledgerwatch/erigon/sync_stages"
	"github.com/ledgerwatch/erigon/turbo/services"
	"github.com/ledgerwatch/erigon/turbo/stages/headerdownload"
	"github.com/ledgerwatch/erigon/turbo/trie"
	"github.com/status-im/keycard-go/hexutils"
)

// TODO: remove debugging structs
type MyStruct struct {
	Storage map[string]string
	Balance *big.Int
	Nonce   *big.Int
}

var collection = make(map[libcommon.Address]*MyStruct)

type ZkInterHashesCfg struct {
	db                kv.RwDB
	checkRoot         bool
	badBlockHalt      bool
	tmpDir            string
	saveNewHashesToDB bool // no reason to save changes when calculating root for mining
	blockReader       services.FullBlockReader
	hd                *headerdownload.HeaderDownload

	historyV3 bool
	agg       *state.AggregatorV3
}

func StageZkInterHashesCfg(db kv.RwDB, checkRoot, saveNewHashesToDB, badBlockHalt bool, tmpDir string, blockReader services.FullBlockReader, hd *headerdownload.HeaderDownload, historyV3 bool, agg *state.AggregatorV3) ZkInterHashesCfg {
	return ZkInterHashesCfg{
		db:                db,
		checkRoot:         checkRoot,
		tmpDir:            tmpDir,
		saveNewHashesToDB: saveNewHashesToDB,
		badBlockHalt:      badBlockHalt,
		blockReader:       blockReader,
		hd:                hd,

		historyV3: historyV3,
		agg:       agg,
	}
}

func SpawnZkIntermediateHashesStage(s *sync_stages.StageState, u sync_stages.Unwinder, tx kv.RwTx, cfg ZkInterHashesCfg, ctx context.Context, quiet bool) (libcommon.Hash, error) {
	quit := ctx.Done()
	_ = quit

	//log.Warn("Interhashes turned off!!!")
	//return trie.EmptyRoot, nil

	useExternalTx := tx != nil
	if !useExternalTx {
		var err error
		tx, err = cfg.db.BeginRw(context.Background())
		if err != nil {
			return trie.EmptyRoot, err
		}
		defer tx.Rollback()
	}

	to, err := s.ExecutionAt(tx)
	if err != nil {
		return trie.EmptyRoot, err
	}

	if s.BlockNumber == to {
		// we already did hash check for this block
		// we don't do the obvious `if s.BlockNumber > to` to support reorgs more naturally
		return trie.EmptyRoot, nil
	}

	var expectedRootHash libcommon.Hash
	var headerHash libcommon.Hash
	var syncHeadHeader *types.Header
	if cfg.checkRoot {
		syncHeadHeader, err = cfg.blockReader.HeaderByNumber(ctx, tx, to)
		if err != nil {
			return trie.EmptyRoot, err
		}
		if syncHeadHeader == nil {
			return trie.EmptyRoot, fmt.Errorf("no header found with number %d", to)
		}
		expectedRootHash = syncHeadHeader.Root
		headerHash = syncHeadHeader.Hash()
	}
	logPrefix := s.LogPrefix()
	if !quiet && to > s.BlockNumber+16 {
		log.Info(fmt.Sprintf("[%s] Generating intermediate hashes", logPrefix), "from", s.BlockNumber, "to", to)
	}

	var root libcommon.Hash
	shouldRegenerate := to > s.BlockNumber && to-s.BlockNumber > 10 // RetainList is in-memory structure and it will OOM if jump is too big, such big jump anyway invalidate most of existing Intermediate hashes
	if !shouldRegenerate && cfg.historyV3 && to-s.BlockNumber > 10 {
		//incremental can work only on DB data, not on snapshots
		_, n, err := rawdbv3.TxNums.FindBlockNum(tx, cfg.agg.EndTxNumMinimax())
		if err != nil {
			return trie.EmptyRoot, err
		}
		shouldRegenerate = s.BlockNumber < n
	}

	if s.BlockNumber == 0 || shouldRegenerate {
		if root, err = RegenerateIntermediateHashes(logPrefix, tx, cfg, &expectedRootHash, ctx, quit); err != nil {
			return trie.EmptyRoot, err
		}
	} else {
		if root, err = ZkIncrementIntermediateHashes(logPrefix, s, tx, to, cfg, &expectedRootHash, quit); err != nil {
			return trie.EmptyRoot, err
		}
	}
	_ = quit

	if cfg.checkRoot && root != expectedRootHash {
		log.Error(fmt.Sprintf("[%s] Wrong trie root of block %d: %x, expected (from header): %x. Block hash: %x", logPrefix, to, root, expectedRootHash, headerHash))
		if cfg.badBlockHalt {
			return trie.EmptyRoot, fmt.Errorf("wrong trie root")
		}
		if cfg.hd != nil {
			cfg.hd.ReportBadHeaderPoS(headerHash, syncHeadHeader.ParentHash)
		}
		if to > s.BlockNumber {
			unwindTo := (to + s.BlockNumber) / 2 // Binary search for the correct block, biased to the lower numbers
			log.Warn("Unwinding due to incorrect root hash", "to", unwindTo)
			u.UnwindTo(unwindTo, headerHash)
		}
	} else if err = s.Update(tx, to); err != nil {
		return trie.EmptyRoot, err
	}

	if !useExternalTx {
		if err := tx.Commit(); err != nil {
			return trie.EmptyRoot, err
		}
	}

	return root, err
}

func UnwindZkIntermediateHashesStage(u *sync_stages.UnwindState, s *sync_stages.StageState, tx kv.RwTx, cfg ZkInterHashesCfg, ctx context.Context) (err error) {
	quit := ctx.Done()
	useExternalTx := tx != nil
	if !useExternalTx {
		tx, err = cfg.db.BeginRw(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}

	syncHeadHeader, err := cfg.blockReader.HeaderByNumber(ctx, tx, u.UnwindPoint)
	if err != nil {
		return err
	}
	if syncHeadHeader == nil {
		return fmt.Errorf("header not found for block number %d", u.UnwindPoint)
	}
	expectedRootHash := syncHeadHeader.Root

	root, err := UnwindZkSMT(s.LogPrefix(), s.BlockNumber, u.UnwindPoint, tx, cfg, &expectedRootHash, quit)
	if err != nil {
		return err
	}
	_ = root

	if err := u.Done(tx); err != nil {
		return err
	}
	if !useExternalTx {
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func insertAccountStateToKV(db smt.DB, keys []utils.NodeKey, ethAddr string, balance, nonce *big.Int) ([]utils.NodeKey, error) {
	keyBalance, err := utils.KeyEthAddrBalance(ethAddr)
	if err != nil {
		return []utils.NodeKey{}, err
	}
	keyNonce, err := utils.KeyEthAddrNonce(ethAddr)
	if err != nil {
		return []utils.NodeKey{}, err
	}

	x := utils.ScalarToArrayBig(balance)
	valueBalance, err := utils.NodeValue8FromBigIntArray(x)
	if err != nil {
		return []utils.NodeKey{}, err
	}

	x = utils.ScalarToArrayBig(nonce)
	valueNonce, err := utils.NodeValue8FromBigIntArray(x)
	if err != nil {
		return []utils.NodeKey{}, err
	}

	if !valueBalance.IsZero() {
		keys = append(keys, keyBalance)
		db.InsertAccountValue(keyBalance, *valueBalance)
	}
	if !valueNonce.IsZero() {
		keys = append(keys, keyNonce)
		db.InsertAccountValue(keyNonce, *valueNonce)
	}
	return keys, nil
}

func insertContractBytecodeToKV(db smt.DB, keys []utils.NodeKey, ethAddr string, bytecode string) ([]utils.NodeKey, error) {
	keyContractCode, err := utils.KeyContractCode(ethAddr)
	if err != nil {
		return []utils.NodeKey{}, err
	}

	keyContractLength, err := utils.KeyContractLength(ethAddr)
	if err != nil {
		return []utils.NodeKey{}, err
	}

	hashedBytecode, err := utils.HashContractBytecode(bytecode)
	if err != nil {
		return []utils.NodeKey{}, err
	}

	var parsedBytecode string

	if strings.HasPrefix(bytecode, "0x") {
		parsedBytecode = bytecode[2:]
	} else {
		parsedBytecode = bytecode
	}

	if len(parsedBytecode)%2 != 0 {
		parsedBytecode = "0" + parsedBytecode
	}

	bytecodeLength := len(parsedBytecode) / 2

	bi := utils.ConvertHexToBigInt(hashedBytecode)

	x := utils.ScalarToArrayBig(bi)
	valueContractCode, err := utils.NodeValue8FromBigIntArray(x)
	if err != nil {
		return []utils.NodeKey{}, err
	}

	x = utils.ScalarToArrayBig(big.NewInt(int64(bytecodeLength)))
	valueContractLength, err := utils.NodeValue8FromBigIntArray(x)
	if err != nil {
		return []utils.NodeKey{}, err
	}

	if !valueContractCode.IsZero() {
		keys = append(keys, keyContractCode)
		db.InsertAccountValue(keyContractCode, *valueContractCode)
	}

	if !valueContractLength.IsZero() {
		keys = append(keys, keyContractLength)
		db.InsertAccountValue(keyContractLength, *valueContractLength)
	}

	return keys, nil
}

func insertContractStorageToKV(db smt.DB, keys []utils.NodeKey, ethAddr string, storage map[string]string) ([]utils.NodeKey, error) {
	a := utils.ConvertHexToBigInt(ethAddr)
	add := utils.ScalarToArrayBig(a)

	for k, v := range storage {
		if v == "" {
			continue
		}

		keyStoragePosition, err := utils.KeyContractStorage(add, k)
		if err != nil {
			return []utils.NodeKey{}, err
		}

		base := 10
		if strings.HasPrefix(v, "0x") {
			v = v[2:]
			base = 16
		}

		val, _ := new(big.Int).SetString(v, base)

		x := utils.ScalarToArrayBig(val)
		parsedValue, err := utils.NodeValue8FromBigIntArray(x)
		if err != nil {
			return []utils.NodeKey{}, err
		}

		if !parsedValue.IsZero() {
			keys = append(keys, keyStoragePosition)
			db.InsertAccountValue(keyStoragePosition, *parsedValue)
		}
	}

	return keys, nil
}

func processAccount(db smt.DB, a *accounts.Account, as map[string]string, inc uint64, psr *state2.PlainStateReader, addr libcommon.Address, keys []utils.NodeKey) ([]utils.NodeKey, error) {

	collection[addr] = &MyStruct{
		Storage: as,
		Balance: a.Balance.ToBig(),
		Nonce:   new(big.Int).SetUint64(a.Nonce),
	}

	// get the account balance and nonce
	keys, err := insertAccountStateToKV(db, keys, addr.String(), a.Balance.ToBig(), new(big.Int).SetUint64(a.Nonce))
	if err != nil {
		return []utils.NodeKey{}, err
	}

	// store the contract bytecode
	cc, err := psr.ReadAccountCode(addr, inc, a.CodeHash)
	if err != nil {
		return []utils.NodeKey{}, err
	}

	ach := hexutils.BytesToHex(cc)
	if len(ach) > 0 {
		hexcc := fmt.Sprintf("0x%s", ach)
		keys, err = insertContractBytecodeToKV(db, keys, addr.String(), hexcc)
		if err != nil {
			return []utils.NodeKey{}, err
		}
	}

	if len(as) > 0 {
		// store the account storage
		keys, err = insertContractStorageToKV(db, keys, addr.String(), as)
		if err != nil {
			return []utils.NodeKey{}, err
		}
	}

	return keys, nil
}

func RegenerateIntermediateHashes(logPrefix string, db kv.RwTx, cfg ZkInterHashesCfg, expectedRootHash *libcommon.Hash, ctx context.Context, quitCh <-chan struct{}) (libcommon.Hash, error) {
	log.Info(fmt.Sprintf("[%s] Regeneration trie hashes started", logPrefix))
	defer log.Info(fmt.Sprintf("[%s] Regeneration ended", logPrefix))

	if err := sync_stages.SaveStageProgress(db, sync_stages.IntermediateHashes, 0); err != nil {
		log.Warn(fmt.Sprint("regenerate SaveStageProgress to zero error: ", err))
	}
	if err := db.ClearBucket("HermezSmt"); err != nil {
		log.Warn(fmt.Sprint("regenerate clear HermezSmt error: ", err))
	}
	if err := db.ClearBucket("HermezSmtLastRoot"); err != nil {
		log.Warn(fmt.Sprint("regenerate clear HermezSmtLastRoot error: ", err))
	}
	if err := db.ClearBucket("HermezSmtAccountValues"); err != nil {
		log.Warn(fmt.Sprint("regenerate clear HermezSmtAccountValues error: ", err))
	}

	eridb, err := db2.NewEriDb(db)
	if err != nil {
		return trie.EmptyRoot, err
	}
	smtIn := smt.NewSMT(eridb)

	var a *accounts.Account
	var addr libcommon.Address
	var as map[string]string
	var inc uint64

	var hash libcommon.Hash
	psr := state2.NewPlainStateReader(db)

	dataCollectStartTime := time.Now()
	log.Info(fmt.Sprintf("[%s] Collecting account data...", logPrefix))
	keys := []utils.NodeKey{}

	stateCt := 0
	err = psr.ForEach(kv.PlainState, nil, func(k, acc []byte) error {
		stateCt++
		return nil
	})
	if err != nil {
		return trie.EmptyRoot, err
	}

	progCt := 0
	progress := make(chan int)
	ctDone := make(chan bool)

	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		var pc int
		var pct int

		for {
			select {
			case newPc := <-progress:
				pc = newPc
				if stateCt > 0 {
					pct = (pc * 100) / stateCt
				}
			case <-ticker.C:
				log.Info(fmt.Sprintf("[%s] Progress: %d/%d (%d%%)", logPrefix, pc, stateCt, pct))
			case <-ctDone:
				return
			}
		}
	}()

	type AccountDump struct {
		Balance  string
		Nonce    uint64
		Storage  map[string]string
		Codehash libcommon.Hash
	}

	storageDump := make(map[string]AccountDump)
	err = psr.ForEach(kv.PlainState, nil, func(k, acc []byte) error {
		progCt++
		progress <- progCt
		var err error
		if len(k) == 20 {
			if a != nil { // don't run process on first loop for first account (or it will miss collecting storage)
				storageDump[addr.String()] = AccountDump{
					Balance:  a.Balance.Hex(),
					Nonce:    a.Nonce,
					Storage:  as,
					Codehash: a.CodeHash,
				}

				keys, err = processAccount(eridb, a, as, inc, psr, addr, keys)
				if err != nil {
					return err
				}
			}

			a = &accounts.Account{}

			if err = a.DecodeForStorage(acc); err != nil {
				// TODO: not an account?
				as = make(map[string]string)
				return nil
			}
			addr = libcommon.BytesToAddress(k)
			inc = a.Incarnation
			// empty storage of previous account
			as = make(map[string]string)
		} else { // otherwise we're reading storage
			_, incarnation, key := dbutils.PlainParseCompositeStorageKey(k)
			if incarnation != inc {
				return nil
			}

			sk := fmt.Sprintf("0x%032x", key)
			v := fmt.Sprintf("0x%032x", acc)

			as[sk] = fmt.Sprint(trimHexString(v))
		}
		return nil
	})

	close(progress)
	close(ctDone)

	if err != nil {
		return trie.EmptyRoot, err
	}

	storageDump[addr.String()] = AccountDump{
		Balance:  a.Balance.Hex(),
		Nonce:    a.Nonce,
		Storage:  as,
		Codehash: a.CodeHash,
	}

	json, _ := json.Marshal(storageDump)
	if err = os.WriteFile("addrDump.json", json, 0644); err != nil {
		return trie.EmptyRoot, err
	}

	// process the final account
	keys, err = processAccount(eridb, a, as, inc, psr, addr, keys)
	if err != nil {
		return trie.EmptyRoot, err
	}

	dataCollectTime := time.Since(dataCollectStartTime)
	log.Info(fmt.Sprintf("[%s] Collecting account data finished in %v", logPrefix, dataCollectTime))

	// generate tree
	_, err = smtIn.GenerateFromKVBulk(logPrefix, keys)
	if err != nil {
		return trie.EmptyRoot, err
	}
	root := smtIn.LastRoot()

	err2 := db.ClearBucket("HermezSmtAccountValues")
	if err2 != nil {
		log.Warn(fmt.Sprint("regenerate SaveStageProgress to zero error: ", err2))
	}

	hash = libcommon.BigToHash(root)

	// [zkevm] - print state
	//jsonData, err := json.MarshalIndent(collection, "", "    ")
	//if err != nil {
	//	fmt.Printf("error: %v\n", err)
	//}
	//_ = jsonData
	//fmt.Println(string(jsonData))

	err = eridb.CommitBatch()
	if err != nil {
		return trie.EmptyRoot, err
	}

	// TODO [zkevm] - max - remove printing of roots
	fmt.Println("[zkevm] interhashes - expected root: ", expectedRootHash.Hex())
	fmt.Println("[zkevm] interhashes - actual root: ", hash.Hex())

	//toPrint := eridb.GetDb()
	//op, err := json.Marshal(toPrint)
	//if err != nil {
	//	fmt.Println(err)
	//}
	////fmt.Println(string(op))
	//// write to file
	//err = ioutil.WriteFile("db.json", op, 0644)
	//if err != nil {
	//	fmt.Println(err)
	//}

	if cfg.checkRoot && hash != *expectedRootHash {
		// [zkevm] - check against the rpc get block by number
		// get block number
		ss := libcommon.HexToAddress("0x000000000000000000000000000000005ca1ab1e")
		key := libcommon.HexToHash("0x0")

		txno, err2 := psr.ReadAccountStorage(ss, 1, &key)
		if err2 != nil {
			return trie.EmptyRoot, err
		}
		// convert txno to big int
		bigTxNo := big.NewInt(0)
		bigTxNo.SetBytes(txno)

		fmt.Println("[zkevm] interhashes - txno: ", bigTxNo)

		sr, err2 := stateRootByTxNo(bigTxNo)
		if err2 != nil {
			return trie.EmptyRoot, err
		}

		if hash != *sr {
			log.Warn(fmt.Sprintf("[%s] Wrong trie root: %x, expected (from header): %x, from rpc: %x", logPrefix, hash, expectedRootHash, *sr))
			return hash, nil
		}

		log.Info("[zkevm] interhashes - trie root matches rpc get block by number")
		*expectedRootHash = *sr
		err = nil
	}
	log.Info(fmt.Sprintf("[%s] Trie root", logPrefix), "hash", hash.Hex())

	return hash, nil
}

func stateRootByTxNo(txNo *big.Int) (*libcommon.Hash, error) {
	requestBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "eth_getBlockByNumber",
		"params":  []interface{}{txNo.Uint64(), true},
		"id":      1,
	}

	requestBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	response, err := http.Post("https://rpc.internal.zkevm-test.net/", "application/json", bytes.NewBuffer(requestBytes))
	if err != nil {
		return nil, err
	}

	defer response.Body.Close()

	responseBytes, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	responseMap := make(map[string]interface{})
	if err := json.Unmarshal(responseBytes, &responseMap); err != nil {
		return nil, err
	}

	result, ok := responseMap["result"].(map[string]interface{})
	if !ok {
		return nil, err
	}

	stateRoot, ok := result["stateRoot"].(string)
	if !ok {
		return nil, err
	}
	h := libcommon.HexToHash(stateRoot)

	return &h, nil
}

func trimHexString(s string) string {
	if strings.HasPrefix(s, "0x") {
		s = s[2:]
	}

	for i := 0; i < len(s); i++ {
		if s[i] != '0' {
			return "0x" + s[i:]
		}
	}

	return "0x0"
}

func ZkIncrementIntermediateHashes(logPrefix string, s *sync_stages.StageState, db kv.RwTx, to uint64, cfg ZkInterHashesCfg, expectedRootHash *libcommon.Hash, quit <-chan struct{}) (libcommon.Hash, error) {
	log.Info(fmt.Sprintf("[%s] Regeneration trie hashes started", logPrefix))
	defer log.Info(fmt.Sprintf("[%s] Regeneration ended", logPrefix))

	fmt.Println("[zkevm] interhashes - previous root @: ", s.BlockNumber)
	fmt.Println("[zkevm] interhashes - calculating root @: ", to)

	eridb, err := db2.NewEriDb(db)
	if err != nil {
		return trie.EmptyRoot, err
	}
	dbSmt := smt.NewSMT(eridb)

	fmt.Println("last root: ", libcommon.BigToHash(dbSmt.LastRoot()))

	eridb.OpenBatch(quit)

	ac, err := db.CursorDupSort(kv.AccountChangeSet)
	if err != nil {
		return trie.EmptyRoot, err
	}
	defer ac.Close()

	sc, err := db.CursorDupSort(kv.StorageChangeSet)
	if err != nil {
		return trie.EmptyRoot, err
	}
	defer sc.Close()

	accChanges := make(map[libcommon.Address]*accounts.Account)
	codeChanges := make(map[libcommon.Address]string)
	storageChanges := make(map[libcommon.Address]map[string]string)

	// NB: changeset tables are zero indexed
	// changeset tables contain historical value at N-1, so we look up values from plainstate
	for i := s.BlockNumber + 1; i <= to; i++ {
		dupSortKey := dbutils.EncodeBlockNumber(i)

		fmt.Println("[zkevm] interhashes - block: ", i)

		// TODO [zkevm]: find out the contractcodelookup

		// i+1 to get state at the beginning of the next batch
		psr := state2.NewPlainState(db, i+1, systemcontracts.SystemContractCodeLookup["Hermez"])

		// collect changes to accounts and code
		for _, v, err := ac.SeekExact(dupSortKey); err == nil && v != nil; _, v, err = ac.NextDup() {
			addr := libcommon.BytesToAddress(v[:length.Addr])

			currAcc, err := psr.ReadAccountData(addr)
			if err != nil {
				return trie.EmptyRoot, err
			}

			// store the account
			accChanges[addr] = currAcc

			// TODO: figure out if we can optimise for performance by making this optional, only on 'creation' or similar
			cc, err := psr.ReadAccountCode(addr, currAcc.Incarnation, currAcc.CodeHash)
			if err != nil {
				return trie.EmptyRoot, err
			}

			ach := hexutils.BytesToHex(cc)
			if len(ach) > 0 {
				hexcc := fmt.Sprintf("0x%s", ach)
				codeChanges[addr] = hexcc
				if err != nil {
					return trie.EmptyRoot, err
				}
			}
		}

		err = db.ForPrefix(kv.StorageChangeSet, dupSortKey, func(sk, sv []byte) error {
			changesetKey := sk[length.BlockNum:]
			address, incarnation := dbutils.PlainParseStoragePrefix(changesetKey)

			sstorageKey := sv[:length.Hash]
			stk := libcommon.BytesToHash(sstorageKey)

			value, err := psr.ReadAccountStorage(address, incarnation, &stk)
			if err != nil {
				return err
			}

			stkk := fmt.Sprintf("0x%032x", stk)
			v := fmt.Sprintf("0x%032x", libcommon.BytesToHash(value))

			m := make(map[string]string)
			m[stkk] = v

			if storageChanges[address] == nil {
				storageChanges[address] = make(map[string]string)
			}
			storageChanges[address][stkk] = v
			return nil
		})
		if err != nil {
			return trie.EmptyRoot, err
		}

		// update the tree
		for addr, acc := range accChanges {
			err := updateAccInTree(dbSmt, addr, acc)
			if err != nil {
				return trie.EmptyRoot, err
			}
		}
		for addr, code := range codeChanges {
			err := updateCodeInTree(dbSmt, addr.String(), code)
			if err != nil {
				return trie.EmptyRoot, err
			}
		}

		for addr, storage := range storageChanges {
			err := updateStorageInTree(dbSmt, addr, storage)
			if err != nil {
				return trie.EmptyRoot, err
			}
		}
	}

	err = verifyLastHash(dbSmt, expectedRootHash, &cfg, logPrefix)
	if err != nil {
		eridb.RollbackBatch()
		log.Error("failed to verify hash")
		return trie.EmptyRoot, err
	}

	err = eridb.CommitBatch()
	if err != nil {
		return trie.EmptyRoot, err
	}

	lr := dbSmt.LastRoot()

	hash := libcommon.BigToHash(lr)
	return hash, nil

}

func updateAccInTree(smt *smt.SMT, addr libcommon.Address, acc *accounts.Account) error {
	if acc != nil {
		n := new(big.Int).SetUint64(acc.Nonce)
		_, err := smt.SetAccountState(addr.String(), acc.Balance.ToBig(), n)
		return err
	}

	_, err := smt.SetAccountState(addr.String(), big.NewInt(0), big.NewInt(0))
	return err
}

func updateStorageInTree(smt *smt.SMT, addr libcommon.Address, as map[string]string) error {
	_, err := smt.SetContractStorage(addr.String(), as)
	return err
}

func updateCodeInTree(smt *smt.SMT, addr string, code string) error {
	return smt.SetContractBytecode(addr, code)
}

func verifyLastHash(dbSmt *smt.SMT, expectedRootHash *libcommon.Hash, cfg *ZkInterHashesCfg, logPrefix string) error {
	hash := libcommon.BigToHash(dbSmt.LastRoot())

	fmt.Println("[zkevm] interhashes - expected root: ", expectedRootHash.Hex())
	fmt.Println("[zkevm] interhashes - actual root: ", hash.Hex())

	if cfg.checkRoot && hash != *expectedRootHash {
		log.Warn(fmt.Sprintf("[%s] Wrong trie root: %x, expected (from header): %x", logPrefix, hash, expectedRootHash))
		return nil
	}
	log.Info(fmt.Sprintf("[%s] Trie root", logPrefix), "hash", hash.Hex())
	return nil
}

func UnwindZkSMT(logPrefix string, from, to uint64, db kv.RwTx, cfg ZkInterHashesCfg, expectedRootHash *libcommon.Hash, quit <-chan struct{}) (libcommon.Hash, error) {
	log.Info(fmt.Sprintf("[%s] Unwind trie hashes started", logPrefix))
	defer log.Info(fmt.Sprintf("[%s] Unwind ended", logPrefix))

	eridb, err := db2.NewEriDb(db)
	if err != nil {
		return trie.EmptyRoot, err
	}
	dbSmt := smt.NewSMT(eridb)

	fmt.Println("last root: ", libcommon.BigToHash(dbSmt.LastRoot()))

	eridb.OpenBatch(quit)

	ac, err := db.CursorDupSort(kv.AccountChangeSet)
	if err != nil {
		return trie.EmptyRoot, err
	}
	defer ac.Close()

	sc, err := db.CursorDupSort(kv.StorageChangeSet)
	if err != nil {
		return trie.EmptyRoot, err
	}
	defer sc.Close()

	accChanges := make(map[libcommon.Address]*accounts.Account)
	accDeletes := make([]libcommon.Address, 0)
	codeChanges := make(map[libcommon.Address]string)
	storageChanges := make(map[libcommon.Address]map[string]string)

	currentPsr := state2.NewPlainStateReader(db)

	// walk backwards through the blocks, applying state changes, and deletes
	// PlainState contains data AT the block
	// History tables contain data BEFORE the block - so need a +1 offset

	for i := from; i >= to+1; i-- {
		dupSortKey := dbutils.EncodeBlockNumber(i)

		fmt.Println("[zkevm] interhashes - block: ", i)

		// collect changes to accounts and code
		for _, v, err2 := ac.SeekExact(dupSortKey); err2 == nil && v != nil; _, v, err2 = ac.NextDup() {
			addr := libcommon.BytesToAddress(v[:length.Addr])

			// if the account was created in this changeset we should delete it
			if len(v[length.Addr:]) == 0 {
				codeChanges[addr] = ""
				accDeletes = append(accDeletes, addr)
				continue
			}

			// decode the old acc from the changeset
			oldAcc := new(accounts.Account)
			err = oldAcc.DecodeForStorage(v[length.Addr:])
			if err != nil {
				return trie.EmptyRoot, err
			}

			// currAcc at block we're unwinding from
			currAcc, err := currentPsr.ReadAccountData(addr)
			if err != nil {
				return trie.EmptyRoot, err
			}

			if oldAcc != nil {
				if oldAcc.Incarnation > 0 {
					if len(v) == 0 { // self-destructed
						accDeletes = append(accDeletes, addr)
					} else {
						if currAcc.Incarnation > oldAcc.Incarnation {
							accDeletes = append(accDeletes, addr)
						}
					}
				}
			}

			if oldAcc != nil {
				// store the account
				fmt.Println("unwinding acc: ", addr.Hex())

				//fmt.Printf("old acc nonce: %d, curr acc nonce: %d\n", oldAcc.Nonce, currAcc.Nonce)
				//fmt.Printf("old acc balance: %d, curr acc balance: %d\n", oldAcc.Balance, currAcc.Balance)
				//fmt.Printf("old acc incarnation: %d, curr acc incarnation: %d\n", oldAcc.Incarnation, currAcc.Incarnation)
				//fmt.Printf("old acc codehash: %x, curr acc codehash: %x\n", oldAcc.CodeHash, currAcc.CodeHash)

				accChanges[addr] = oldAcc

				if oldAcc.CodeHash != currAcc.CodeHash {
					cc, err := currentPsr.ReadAccountCode(addr, oldAcc.Incarnation, oldAcc.CodeHash)
					if err != nil {
						return trie.EmptyRoot, err
					}

					ach := hexutils.BytesToHex(cc)
					if len(ach) > 0 {
						hexcc := fmt.Sprintf("0x%s", ach)
						codeChanges[addr] = hexcc
						if err != nil {
							return trie.EmptyRoot, err
						}
					}
				}
			}
		}

		err = db.ForPrefix(kv.StorageChangeSet, dupSortKey, func(sk, sv []byte) error {
			changesetKey := sk[length.BlockNum:]
			address, _ := dbutils.PlainParseStoragePrefix(changesetKey)

			sstorageKey := sv[:length.Hash]
			stk := libcommon.BytesToHash(sstorageKey)

			value := []byte{0}
			if len(sv[length.Hash:]) != 0 {
				value = sv[length.Hash:]
			}

			stkk := fmt.Sprintf("0x%032x", stk)
			v := fmt.Sprintf("0x%032x", libcommon.BytesToHash(value))

			m := make(map[string]string)
			m[stkk] = v

			if storageChanges[address] == nil {
				storageChanges[address] = make(map[string]string)
			}
			storageChanges[address][stkk] = v
			return nil
		})
		if err != nil {
			return trie.EmptyRoot, err
		}

		// update the tree
		for addr, acc := range accChanges {
			err := updateAccInTree(dbSmt, addr, acc)
			if err != nil {
				return trie.EmptyRoot, err
			}
		}
		for addr, code := range codeChanges {
			err := updateCodeInTree(dbSmt, addr.String(), code)
			if err != nil {
				return trie.EmptyRoot, err
			}
		}

		for addr, storage := range storageChanges {
			err := updateStorageInTree(dbSmt, addr, storage)
			if err != nil {
				return trie.EmptyRoot, err
			}
		}
	}

	for _, k := range accDeletes {
		fmt.Println("deleting acc: ", k.Hex())
		err := updateAccInTree(dbSmt, k, nil)
		if err != nil {
			return trie.EmptyRoot, err
		}
	}

	err = verifyLastHash(dbSmt, expectedRootHash, &cfg, logPrefix)
	if err != nil {
		eridb.RollbackBatch()
		log.Error("failed to verify hash")
		return trie.EmptyRoot, err
	}

	err = eridb.CommitBatch()
	if err != nil {
		return trie.EmptyRoot, err
	}

	lr := dbSmt.LastRoot()

	hash := libcommon.BigToHash(lr)
	return hash, nil
}
