package stagedsync

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"math/big"
	"math/bits"
	"sync/atomic"

	libcommon "github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/common/hexutility"
	"github.com/ledgerwatch/erigon-lib/common/length"
	"github.com/ledgerwatch/erigon-lib/etl"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/kv/order"
	"github.com/ledgerwatch/erigon-lib/kv/rawdbv3"
	"github.com/ledgerwatch/erigon-lib/kv/temporal/historyv2"
	"github.com/ledgerwatch/erigon-lib/state"
	"github.com/ledgerwatch/log/v3"
	"golang.org/x/exp/slices"

	state2 "github.com/ledgerwatch/erigon/core/state"
	"github.com/ledgerwatch/erigon/core/types"
	db2 "github.com/ledgerwatch/erigon/smt/pkg/db"
	"github.com/ledgerwatch/erigon/smt/pkg/smt"

	"github.com/ledgerwatch/erigon/core/state/temporal"

	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/ledgerwatch/erigon/common"
	"github.com/ledgerwatch/erigon/common/dbutils"
	"github.com/ledgerwatch/erigon/common/math"
	"github.com/ledgerwatch/erigon/core/types/accounts"
	"github.com/ledgerwatch/erigon/turbo/services"
	"github.com/ledgerwatch/erigon/turbo/stages/headerdownload"
	"github.com/ledgerwatch/erigon/turbo/trie"
	"github.com/ledgerwatch/erigon/zkevm/adapter"
	"github.com/status-im/keycard-go/hexutils"
)

type TrieCfg struct {
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

func StageTrieCfg(db kv.RwDB, checkRoot, saveNewHashesToDB, badBlockHalt bool, tmpDir string, blockReader services.FullBlockReader, hd *headerdownload.HeaderDownload, historyV3 bool, agg *state.AggregatorV3) TrieCfg {
	return TrieCfg{
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

func SpawnIntermediateHashesStage(s *StageState, u Unwinder, tx kv.RwTx, cfg TrieCfg, ctx context.Context, quiet bool) (libcommon.Hash, error) {
	quit := ctx.Done()
	_ = quit
	useExternalTx := tx != nil
	if !useExternalTx {
		var err error
		tx, err = cfg.db.BeginRw(context.Background())
		if err != nil {
			return trie.EmptyRoot, err
		}
		defer tx.Rollback()
	}

	// max: we have to have executed in order to be able to do interhashes (see execution!)
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
	tooBigJump := to > s.BlockNumber && to-s.BlockNumber > 100_000 // RetainList is in-memory structure and it will OOM if jump is too big, such big jump anyway invalidate most of existing Intermediate hashes
	if !tooBigJump && cfg.historyV3 && to-s.BlockNumber > 10 {
		//incremental can work only on DB data, not on snapshots
		_, n, err := rawdbv3.TxNums.FindBlockNum(tx, cfg.agg.EndTxNumMinimax())
		if err != nil {
			return trie.EmptyRoot, err
		}
		tooBigJump = s.BlockNumber < n
	}

	if s.BlockNumber == 0 || tooBigJump {
		if root, err = RegenerateIntermediateHashes(logPrefix, tx, cfg, &expectedRootHash, ctx); err != nil {
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

// TODO [zkevm] remove debugging struct
type MyStruct struct {
	Storage map[string]string
	Balance *big.Int
	Nonce   *big.Int
}

var collection = make(map[libcommon.Address]*MyStruct)

func processAccount(s *smt.SMT, root *big.Int, a *accounts.Account, as map[string]string, inc uint64, psr *state2.PlainStateReader, addr libcommon.Address) (*big.Int, error) {

	//fmt.Printf("addr: %x\n account: %+v\n storage: %+v\n", addr, a, as)
	collection[addr] = &MyStruct{
		Storage: as,
		Balance: a.Balance.ToBig(),
		Nonce:   new(big.Int).SetUint64(a.Nonce),
	}

	// store the account balance and nonce
	_, err := s.SetAccountState(addr.String(), a.Balance.ToBig(), new(big.Int).SetUint64(a.Nonce))
	if err != nil {
		return nil, err
	}

	// store the contract bytecode
	cc, err := psr.ReadAccountCode(addr, inc, a.CodeHash)
	if err != nil {
		return nil, err
	}

	ach := hexutils.BytesToHex(cc)
	if len(ach) > 0 {
		hexcc := fmt.Sprintf("0x%s", ach)
		err = s.SetContractBytecode(addr.String(), hexcc)
		if err != nil {
			return nil, err
		}
	}

	if len(as) > 0 {
		// store the account storage
		_, err = s.SetContractStorage(addr.String(), as)
	}

	return s.LastRoot(), nil
}

func RegenerateIntermediateHashes(logPrefix string, db kv.RwTx, cfg TrieCfg, expectedRootHash *libcommon.Hash, ctx context.Context) (libcommon.Hash, error) {
	log.Info(fmt.Sprintf("[%s] Regeneration trie hashes started", logPrefix))
	defer log.Info(fmt.Sprintf("[%s] Regeneration ended", logPrefix))
	_ = db.ClearBucket(kv.TrieOfAccounts)
	_ = db.ClearBucket(kv.TrieOfStorage)
	clean := kv.ReadAhead(ctx, cfg.db, &atomic.Bool{}, kv.HashedAccounts, nil, math.MaxUint32)
	defer clean()
	clean2 := kv.ReadAhead(ctx, cfg.db, &atomic.Bool{}, kv.HashedStorage, nil, math.MaxUint32)
	defer clean2()

	// [zkEVM] - read back the stored transactions and process with SMT

	eridb := db2.NewEriDb(db)
	smt := smt.NewSMT(eridb)

	// [zkevm] - starts a 30s ticker to lock the db and remove orphans
	doneChan := make(chan bool)
	//	smt.StartPeriodicCheck(doneChan)

	var a *accounts.Account
	var addr libcommon.Address
	var as map[string]string
	var inc uint64

	var root = big.NewInt(0)
	var hash libcommon.Hash
	psr := state2.NewPlainStateReader(db)

	stateCt := 0
	err := psr.ForEach(kv.PlainState, nil, func(k, acc []byte) error {
		stateCt++
		return nil
	})

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

	err = psr.ForEach(kv.PlainState, nil, func(k, acc []byte) error {
		progCt++
		progress <- progCt
		var err error
		if len(k) == 20 {
			if a != nil { // don't run process on first loop for first account (or it will miss collecting storage)
				root, err = processAccount(smt, root, a, as, inc, psr, addr)
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

			as[sk] = fmt.Sprintf(trimHexString(v))
		}
		return nil
	})

	if err != nil {
		return trie.EmptyRoot, err
	}

	// process the final account
	root, err = processAccount(smt, root, a, as, inc, psr, addr)
	if err != nil {
		return trie.EmptyRoot, err
	}

	// [zkevm] - print state
	jsonData, err := json.MarshalIndent(collection, "", "    ")
	if err != nil {
		fmt.Printf("error: %v\n", err)
	}
	_ = jsonData
	fmt.Println(string(jsonData))

	hash = libcommon.BigToHash(root)

	close(progress)
	close(doneChan)
	close(ctDone)

	// TODO [zkevm] - max - remove printing of roots
	fmt.Println("[zkevm] interhashes - expected root: ", expectedRootHash.Hex())
	fmt.Println("[zkevm] interhashes - actual root: ", hash.Hex())

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
			log.Warn(fmt.Sprintf("[%s] Wrong trie root: %x, expected (from header): %x", logPrefix, hash, expectedRootHash))
			return hash, nil
		}

		log.Info("[zkevm] interhashes - trie root matches rpc get block by number")
		*expectedRootHash = *sr
		err = nil
	}
	log.Info(fmt.Sprintf("[%s] Trie root", logPrefix), "hash", hash.Hex())

	return hash, nil
}

type HashPromoter struct {
	tx               kv.RwTx
	ChangeSetBufSize uint64
	TempDir          string
	logPrefix        string
	quitCh           <-chan struct{}
}

func NewHashPromoter(db kv.RwTx, tempDir string, quitCh <-chan struct{}, logPrefix string) *HashPromoter {
	return &HashPromoter{
		tx:               db,
		ChangeSetBufSize: 256 * 1024 * 1024,
		TempDir:          tempDir,
		quitCh:           quitCh,
		logPrefix:        logPrefix,
	}
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

	response, err := http.Post("https://zkevm-rpc.com", "application/json", bytes.NewBuffer(requestBytes))
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

func (p *HashPromoter) PromoteOnHistoryV3(logPrefix string, agg *state.AggregatorV3, from, to uint64, storage bool, load func(k []byte, v []byte) error) error {
	nonEmptyMarker := []byte{1}

	agg.SetTx(p.tx)

	txnFrom, err := rawdbv3.TxNums.Min(p.tx, from+1)
	if err != nil {
		return err
	}
	txnTo := uint64(math.MaxUint64)

	if storage {
		compositeKey := make([]byte, length.Hash+length.Hash)
		it, err := p.tx.(kv.TemporalTx).HistoryRange(temporal.StorageHistory, int(txnFrom), int(txnTo), order.Asc, kv.Unlim)
		if err != nil {
			return err
		}
		for it.HasNext() {
			k, v, err := it.Next()
			if err != nil {
				return err
			}
			addrHash, err := common.HashData(k[:length.Addr])
			if err != nil {
				return err
			}
			secKey, err := common.HashData(k[length.Addr:])
			if err != nil {
				return err
			}
			copy(compositeKey, addrHash[:])
			copy(compositeKey[length.Hash:], secKey[:])
			if len(v) != 0 {
				v = nonEmptyMarker
			}
			if err := load(compositeKey, v); err != nil {
				return err
			}
		}
		return nil
	}

	it, err := p.tx.(kv.TemporalTx).HistoryRange(temporal.AccountsHistory, int(txnFrom), int(txnTo), order.Asc, kv.Unlim)
	if err != nil {
		return err
	}
	for it.HasNext() {
		k, v, err := it.Next()
		if err != nil {
			return err
		}
		newK, err := transformPlainStateKey(k)
		if err != nil {
			return err
		}
		if len(v) != 0 {
			v = nonEmptyMarker
		}
		if err := load(newK, v); err != nil {
			return err
		}
	}
	return nil
}

func (p *HashPromoter) Promote(logPrefix string, from, to uint64, storage bool, load etl.LoadFunc) error {
	var changeSetBucket string
	if storage {
		changeSetBucket = kv.StorageChangeSet
	} else {
		changeSetBucket = kv.AccountChangeSet
	}
	log.Trace(fmt.Sprintf("[%s] Incremental state promotion of intermediate hashes", logPrefix), "from", from, "to", to, "csbucket", changeSetBucket)

	startkey := hexutility.EncodeTs(from + 1)

	decode := historyv2.Mapper[changeSetBucket].Decode
	var deletedAccounts [][]byte
	extract := func(dbKey, dbValue []byte, next etl.ExtractNextFunc) error {
		_, k, v, err := decode(dbKey, dbValue)
		if err != nil {
			return err
		}
		newK, err := transformPlainStateKey(k)
		if err != nil {
			return err
		}
		if !storage && len(v) > 0 {

			var oldAccount accounts.Account
			if err := oldAccount.DecodeForStorage(v); err != nil {
				return err
			}

			if oldAccount.Incarnation > 0 {

				newValue, err := p.tx.GetOne(kv.PlainState, k)
				if err != nil {
					return err
				}

				if len(newValue) == 0 { // self-destructed
					deletedAccounts = append(deletedAccounts, newK)
				} else { // turns incarnation to zero
					var newAccount accounts.Account
					if err := newAccount.DecodeForStorage(newValue); err != nil {
						return err
					}
					if newAccount.Incarnation < oldAccount.Incarnation {
						deletedAccounts = append(deletedAccounts, newK)
					}
				}
			}
		}

		return next(dbKey, newK, v)
	}

	var l OldestAppearedLoad
	l.innerLoadFunc = load

	if err := etl.Transform(
		logPrefix,
		p.tx,
		changeSetBucket,
		"",
		p.TempDir,
		extract,
		l.LoadFunc,
		etl.TransformArgs{
			BufferType:      etl.SortableOldestAppearedBuffer,
			ExtractStartKey: startkey,
			Quit:            p.quitCh,
		},
	); err != nil {
		return err
	}

	if !storage { // delete Intermediate hashes of deleted accounts
		slices.SortFunc(deletedAccounts, func(a, b []byte) bool { return bytes.Compare(a, b) < 0 })
		for _, k := range deletedAccounts {
			if err := p.tx.ForPrefix(kv.TrieOfStorage, k, func(k, v []byte) error {
				if err := p.tx.Delete(kv.TrieOfStorage, k); err != nil {
					return err
				}
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	}
	return nil
}

func (p *HashPromoter) UnwindOnHistoryV3(logPrefix string, agg *state.AggregatorV3, unwindFrom, unwindTo uint64, storage bool, load func(k []byte, v []byte)) error {
	txnFrom, err := rawdbv3.TxNums.Min(p.tx, unwindTo)
	if err != nil {
		return err
	}
	txnTo := uint64(math.MaxUint64)
	var deletedAccounts [][]byte

	if storage {
		it, err := p.tx.(kv.TemporalTx).HistoryRange(temporal.StorageHistory, int(txnFrom), int(txnTo), order.Asc, kv.Unlim)
		if err != nil {
			return err
		}
		for it.HasNext() {
			k, _, err := it.Next()
			if err != nil {
				return err
			}
			// Plain state not unwind yet, it means - if key not-exists in PlainState but has value from ChangeSets - then need mark it as "created" in RetainList
			enc, err := p.tx.GetOne(kv.PlainState, k[:20])
			if err != nil {
				return err
			}
			incarnation := uint64(1)
			if len(enc) != 0 {
				oldInc, _ := accounts.DecodeIncarnationFromStorage(enc)
				incarnation = oldInc
			}
			plainKey := dbutils.PlainGenerateCompositeStorageKey(k[:20], incarnation, k[20:])
			value, err := p.tx.GetOne(kv.PlainState, plainKey)
			if err != nil {
				return err
			}
			newK, err := transformPlainStateKey(plainKey)
			if err != nil {
				return err
			}
			load(newK, value)
		}
		return nil
	}

	it, err := p.tx.(kv.TemporalTx).HistoryRange(temporal.AccountsHistory, int(txnFrom), int(txnTo), order.Asc, kv.Unlim)
	if err != nil {
		return err
	}
	for it.HasNext() {
		k, v, err := it.Next()
		if err != nil {
			return err
		}
		newK, err := transformPlainStateKey(k)
		if err != nil {
			return err
		}
		// Plain state not unwind yet, it means - if key not-exists in PlainState but has value from ChangeSets - then need mark it as "created" in RetainList
		value, err := p.tx.GetOne(kv.PlainState, k)
		if err != nil {
			return err
		}

		if len(value) > 0 {
			oldInc, _ := accounts.DecodeIncarnationFromStorage(value)
			if oldInc > 0 {
				if len(v) == 0 { // self-destructed
					deletedAccounts = append(deletedAccounts, newK)
				} else {
					var newAccount accounts.Account
					if err = accounts.DeserialiseV3(&newAccount, v); err != nil {
						return err
					}
					if newAccount.Incarnation > oldInc {
						deletedAccounts = append(deletedAccounts, newK)
					}
				}
			}
		}

		load(newK, value)
	}

	// delete Intermediate hashes of deleted accounts
	slices.SortFunc(deletedAccounts, func(a, b []byte) bool { return bytes.Compare(a, b) < 0 })
	for _, k := range deletedAccounts {
		if err := p.tx.ForPrefix(kv.TrieOfStorage, k, func(k, v []byte) error {
			if err := p.tx.Delete(kv.TrieOfStorage, k); err != nil {
				return err
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func (p *HashPromoter) Unwind(logPrefix string, s *StageState, u *UnwindState, storage bool, load etl.LoadFunc) error {
	to := u.UnwindPoint
	var changeSetBucket string

	if storage {
		changeSetBucket = kv.StorageChangeSet
	} else {
		changeSetBucket = kv.AccountChangeSet
	}
	log.Info(fmt.Sprintf("[%s] Unwinding", logPrefix), "from", s.BlockNumber, "to", to, "csbucket", changeSetBucket)

	startkey := hexutility.EncodeTs(to + 1)

	decode := historyv2.Mapper[changeSetBucket].Decode
	var deletedAccounts [][]byte
	extract := func(dbKey, dbValue []byte, next etl.ExtractNextFunc) error {
		_, k, v, err := decode(dbKey, dbValue)
		if err != nil {
			return err
		}
		newK, err := transformPlainStateKey(k)
		if err != nil {
			return err
		}
		// Plain state not unwind yet, it means - if key not-exists in PlainState but has value from ChangeSets - then need mark it as "created" in RetainList
		value, err := p.tx.GetOne(kv.PlainState, k)
		if err != nil {
			return err
		}

		if !storage && len(value) > 0 {
			var oldAccount accounts.Account
			if err = oldAccount.DecodeForStorage(value); err != nil {
				return err
			}
			if oldAccount.Incarnation > 0 {
				if len(v) == 0 { // self-destructed
					deletedAccounts = append(deletedAccounts, newK)
				} else {
					var newAccount accounts.Account
					if err = newAccount.DecodeForStorage(v); err != nil {
						return err
					}
					if newAccount.Incarnation > oldAccount.Incarnation {
						deletedAccounts = append(deletedAccounts, newK)
					}
				}
			}
		}
		return next(k, newK, value)
	}

	var l OldestAppearedLoad
	l.innerLoadFunc = load

	if err := etl.Transform(
		logPrefix,
		p.tx,
		changeSetBucket,
		"",
		p.TempDir,
		extract,
		l.LoadFunc,
		etl.TransformArgs{
			BufferType:      etl.SortableOldestAppearedBuffer,
			ExtractStartKey: startkey,
			Quit:            p.quitCh,
		},
	); err != nil {
		return err
	}

	if !storage { // delete Intermediate hashes of deleted accounts
		slices.SortFunc(deletedAccounts, func(a, b []byte) bool { return bytes.Compare(a, b) < 0 })
		for _, k := range deletedAccounts {
			if err := p.tx.ForPrefix(kv.TrieOfStorage, k, func(k, v []byte) error {
				if err := p.tx.Delete(kv.TrieOfStorage, k); err != nil {
					return err
				}
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	}

	return nil
}

func incrementIntermediateHashes(logPrefix string, s *StageState, db kv.RwTx, to uint64, cfg TrieCfg, expectedRootHash libcommon.Hash, quit <-chan struct{}) (libcommon.Hash, error) {
	p := NewHashPromoter(db, cfg.tmpDir, quit, logPrefix)
	rl := trie.NewRetainList(0)
	if cfg.historyV3 {
		cfg.agg.SetTx(db)
		collect := func(k, v []byte) error {
			if len(k) == 32 {
				rl.AddKeyWithMarker(k, len(v) == 0)
				return nil
			}
			accBytes, err := p.tx.GetOne(kv.HashedAccounts, k[:32])
			if err != nil {
				return err
			}
			incarnation := uint64(1)
			if len(accBytes) != 0 {
				incarnation, err = accounts.DecodeIncarnationFromStorage(accBytes)
				if err != nil {
					return err
				}
				if incarnation == 0 {
					return nil
				}
			}
			compositeKey := make([]byte, length.Hash+length.Incarnation+length.Hash)
			copy(compositeKey, k[:32])
			binary.BigEndian.PutUint64(compositeKey[32:], incarnation)
			copy(compositeKey[40:], k[32:])
			rl.AddKeyWithMarker(compositeKey, len(v) == 0)
			return nil
		}
		if err := p.PromoteOnHistoryV3(logPrefix, cfg.agg, s.BlockNumber, to, false, collect); err != nil {
			return trie.EmptyRoot, err
		}
		if err := p.PromoteOnHistoryV3(logPrefix, cfg.agg, s.BlockNumber, to, true, collect); err != nil {
			return trie.EmptyRoot, err
		}
	} else {
		collect := func(k, v []byte, _ etl.CurrentTableReader, _ etl.LoadNextFunc) error {
			rl.AddKeyWithMarker(k, len(v) == 0)
			return nil
		}
		if err := p.Promote(logPrefix, s.BlockNumber, to, false, collect); err != nil {
			return trie.EmptyRoot, err
		}
		if err := p.Promote(logPrefix, s.BlockNumber, to, true, collect); err != nil {
			return trie.EmptyRoot, err
		}
	}
	accTrieCollector := etl.NewCollector(logPrefix, cfg.tmpDir, etl.NewSortableBuffer(etl.BufferOptimalSize))
	defer accTrieCollector.Close()
	accTrieCollectorFunc := accountTrieCollector(accTrieCollector)

	stTrieCollector := etl.NewCollector(logPrefix, cfg.tmpDir, etl.NewSortableBuffer(etl.BufferOptimalSize))
	defer stTrieCollector.Close()
	stTrieCollectorFunc := storageTrieCollector(stTrieCollector)

	loader := trie.NewFlatDBTrieLoader(logPrefix, rl, accTrieCollectorFunc, stTrieCollectorFunc, false)
	hash, err := loader.CalcTrieRoot(db, quit)
	if err != nil {
		return trie.EmptyRoot, err
	}

	if cfg.checkRoot && hash != expectedRootHash {
		return hash, nil
	}

	if err := accTrieCollector.Load(db, kv.TrieOfAccounts, etl.IdentityLoadFunc, etl.TransformArgs{Quit: quit}); err != nil {
		return trie.EmptyRoot, err
	}
	if err := stTrieCollector.Load(db, kv.TrieOfStorage, etl.IdentityLoadFunc, etl.TransformArgs{Quit: quit}); err != nil {
		return trie.EmptyRoot, err
	}
	return hash, nil
}

func UnwindIntermediateHashesStage(u *UnwindState, s *StageState, tx kv.RwTx, cfg TrieCfg, ctx context.Context) (err error) {
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

	logPrefix := s.LogPrefix()
	if err := unwindIntermediateHashesStageImpl(logPrefix, u, s, tx, cfg, expectedRootHash, quit); err != nil {
		return err
	}
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

func UnwindIntermediateHashesForTrieLoader(logPrefix string, rl *trie.RetainList, u *UnwindState, s *StageState, db kv.RwTx, cfg TrieCfg, accTrieCollectorFunc trie.HashCollector2, stTrieCollectorFunc trie.StorageHashCollector2, quit <-chan struct{}) (*trie.FlatDBTrieLoader, error) {
	p := NewHashPromoter(db, cfg.tmpDir, quit, logPrefix)
	if cfg.historyV3 {
		cfg.agg.SetTx(db)
		collect := func(k, v []byte) {
			rl.AddKeyWithMarker(k, len(v) == 0)
		}
		if err := p.UnwindOnHistoryV3(logPrefix, cfg.agg, s.BlockNumber, u.UnwindPoint, false, collect); err != nil {
			return nil, err
		}
		if err := p.UnwindOnHistoryV3(logPrefix, cfg.agg, s.BlockNumber, u.UnwindPoint, true, collect); err != nil {
			return nil, err
		}
	} else {
		collect := func(k, v []byte, _ etl.CurrentTableReader, _ etl.LoadNextFunc) error {
			rl.AddKeyWithMarker(k, len(v) == 0)
			return nil
		}
		if err := p.Unwind(logPrefix, s, u, false /* storage */, collect); err != nil {
			return nil, err
		}
		if err := p.Unwind(logPrefix, s, u, true /* storage */, collect); err != nil {
			return nil, err
		}
	}

	return trie.NewFlatDBTrieLoader(logPrefix, rl, accTrieCollectorFunc, stTrieCollectorFunc, false), nil
}

func unwindIntermediateHashesStageImpl(logPrefix string, u *UnwindState, s *StageState, db kv.RwTx, cfg TrieCfg, expectedRootHash libcommon.Hash, quit <-chan struct{}) error {
	accTrieCollector := etl.NewCollector(logPrefix, cfg.tmpDir, etl.NewSortableBuffer(etl.BufferOptimalSize))
	defer accTrieCollector.Close()
	accTrieCollectorFunc := accountTrieCollector(accTrieCollector)

	stTrieCollector := etl.NewCollector(logPrefix, cfg.tmpDir, etl.NewSortableBuffer(etl.BufferOptimalSize))
	defer stTrieCollector.Close()
	stTrieCollectorFunc := storageTrieCollector(stTrieCollector)

	rl := trie.NewRetainList(0)

	loader, err := UnwindIntermediateHashesForTrieLoader(logPrefix, rl, u, s, db, cfg, accTrieCollectorFunc, stTrieCollectorFunc, quit)
	if err != nil {
		return err
	}

	hash, err := loader.CalcTrieRoot(db, quit)
	if err != nil {
		return err
	}
	if hash != expectedRootHash {
		return fmt.Errorf("wrong trie root: %x, expected (from header): %x", hash, expectedRootHash)
	}
	log.Info(fmt.Sprintf("[%s] Trie root", logPrefix), "hash", hash.Hex())
	if err := accTrieCollector.Load(db, kv.TrieOfAccounts, etl.IdentityLoadFunc, etl.TransformArgs{Quit: quit}); err != nil {
		return err
	}
	if err := stTrieCollector.Load(db, kv.TrieOfStorage, etl.IdentityLoadFunc, etl.TransformArgs{Quit: quit}); err != nil {
		return err
	}
	return nil
}

func assertSubset(a, b uint16) {
	if (a & b) != a { // a & b == a - checks whether a is subset of b
		panic(fmt.Errorf("invariant 'is subset' failed: %b, %b", a, b))
	}
}

func accountTrieCollector(collector *etl.Collector) trie.HashCollector2 {
	newV := make([]byte, 0, 1024)
	return func(keyHex []byte, hasState, hasTree, hasHash uint16, hashes, _ []byte) error {
		if len(keyHex) == 0 {
			return nil
		}
		if hasState == 0 {
			return collector.Collect(keyHex, nil)
		}
		if bits.OnesCount16(hasHash) != len(hashes)/length.Hash {
			panic(fmt.Errorf("invariant bits.OnesCount16(hasHash) == len(hashes) failed: %d, %d", bits.OnesCount16(hasHash), len(hashes)/length.Hash))
		}
		assertSubset(hasTree, hasState)
		assertSubset(hasHash, hasState)
		newV = trie.MarshalTrieNode(hasState, hasTree, hasHash, hashes, nil, newV)
		return collector.Collect(keyHex, newV)
	}
}

func storageTrieCollector(collector *etl.Collector) trie.StorageHashCollector2 {
	newK := make([]byte, 0, 128)
	newV := make([]byte, 0, 1024)
	return func(accWithInc []byte, keyHex []byte, hasState, hasTree, hasHash uint16, hashes, rootHash []byte) error {
		newK = append(append(newK[:0], accWithInc...), keyHex...)
		if hasState == 0 {
			return collector.Collect(newK, nil)
		}
		if len(keyHex) > 0 && hasHash == 0 && hasTree == 0 {
			return nil
		}
		if bits.OnesCount16(hasHash) != len(hashes)/length.Hash {
			panic(fmt.Errorf("invariant bits.OnesCount16(hasHash) == len(hashes) failed: %d, %d", bits.OnesCount16(hasHash), len(hashes)/length.Hash))
		}
		assertSubset(hasTree, hasState)
		assertSubset(hasHash, hasState)
		newV = trie.MarshalTrieNode(hasState, hasTree, hasHash, hashes, rootHash, newV)
		return collector.Collect(newK, newV)
	}
}

func PruneIntermediateHashesStage(s *PruneState, tx kv.RwTx, cfg TrieCfg, ctx context.Context) (err error) {
	useExternalTx := tx != nil
	if !useExternalTx {
		tx, err = cfg.db.BeginRw(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}
	s.Done(tx)

	if !useExternalTx {
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
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

func ZkIncrementIntermediateHashes(logPrefix string, s *StageState, db kv.RwTx, to uint64, cfg TrieCfg, expectedRootHash *libcommon.Hash, quit <-chan struct{}) (libcommon.Hash, error) {
	log.Info(fmt.Sprintf("[%s] Regeneration trie hashes started", logPrefix))
	defer log.Info(fmt.Sprintf("[%s] Regeneration ended", logPrefix))

	fmt.Println("[zkevm] interhashes - previous root @: ", s.BlockNumber)
	fmt.Println("[zkevm] interhashes - calculating root @: ", to)

	psr := state2.NewPlainStateReader(db)

	eridb := db2.NewEriDb(db)
	dbSmt := smt.NewSMT(eridb)

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

	// NB: changeset tables are zero indexed
	// changeset tables contain historical value at N-1, so we look up values from plainstate
	for i := s.BlockNumber; i <= to; i++ {
		dupSortKey := dbutils.EncodeBlockNumber(i)

		fmt.Println("[zkevm] interhashes - block: ", i)
		for k, v, err := ac.SeekExact(dupSortKey); err == nil && v != nil; k, v, err = ac.NextDup() {
			fmt.Println(k)
			addr := libcommon.BytesToAddress(v[:length.Addr])

			currAcc, err := psr.ReadAccountData(addr)
			if err != nil {
				return trie.EmptyRoot, err
			}

			fmt.Println("[zkevm] interhashes - account: ", addr.Hex())

			oldAcc := &accounts.Account{}
			if len(v[length.Addr:]) != 0 {
				err := oldAcc.DecodeForStorage(v[length.Addr:])
				if err != nil {
					return trie.EmptyRoot, err
				}
			}

			fmt.Println(oldAcc)
			fmt.Println(currAcc)

			err = updateAccInTree(dbSmt, addr, currAcc)
			if err != nil {
				return trie.EmptyRoot, err
			}

			// store the contract bytecode
			cc, err := psr.ReadAccountCode(addr, currAcc.Incarnation, currAcc.CodeHash)
			if err != nil {
				return trie.EmptyRoot, err
			}

			x := &MyStruct{
				Balance: currAcc.Balance.ToBig(),
				Nonce:   new(big.Int).SetUint64(currAcc.Nonce),
			}
			if collection[addr] != nil {
				x = collection[addr]
			}

			ach := hexutils.BytesToHex(cc)
			if len(ach) > 0 {
				hexcc := fmt.Sprintf("0x%s", ach)
				err = updateCodeInTree(dbSmt, addr.String(), hexcc)
				if err != nil {
					return trie.EmptyRoot, err
				}
			}

			storageKey := append(dupSortKey, addr.Bytes()...)
			incarnationBytes := adapter.UintBytes(currAcc.Incarnation)
			storageKey = append(storageKey, incarnationBytes...)

			for sk, sv, serr := sc.SeekExact(storageKey); serr == nil && sv != nil; sk, sv, serr = sc.NextDup() {
				fmt.Println(sk)
				changesetKey := sk[length.BlockNum:]
				address, incarnation := dbutils.PlainParseStoragePrefix(changesetKey)

				fmt.Println("updating storage for: " + address.Hex())

				sstorageKey := sv[:length.Hash]
				stk := libcommon.BytesToHash(sstorageKey)

				value, err := psr.ReadAccountStorage(address, incarnation, &stk)
				if err != nil {
					return trie.EmptyRoot, err
				}

				fmt.Println(value)

				stkk := fmt.Sprintf("0x%032x", stk)
				v := fmt.Sprintf("0x%032x", libcommon.BytesToHash(value))

				m := make(map[string]string)
				m[stkk] = v
				if x.Storage == nil {
					x.Storage = make(map[string]string)
				}
				x.Storage[stk.String()] = libcommon.BytesToHash(value).String()

				fmt.Printf("key: %s, value: %s\n", stk.String(), libcommon.BytesToHash(value).String())

				err = updateStorageInTree(dbSmt, address, m)
				if err != nil {
					return trie.EmptyRoot, err
				}
			}

			collection[addr] = x
		}

		fmt.Println(dbSmt.LastRoot())
	}

	jsonData, err := json.MarshalIndent(collection, "", "    ")
	if err != nil {
		fmt.Printf("error: %v\n", err)
	}
	_ = jsonData
	fmt.Println(string(jsonData))

	return libcommon.BigToHash(dbSmt.LastRoot()), nil
}

func updateAccInTree(smt *smt.SMT, addr libcommon.Address, acc *accounts.Account) error {
	n := new(big.Int).SetUint64(acc.Nonce)
	_, err := smt.SetAccountState(addr.String(), acc.Balance.ToBig(), n)
	return err
}

func updateStorageInTree(smt *smt.SMT, addr libcommon.Address, as map[string]string) error {
	_, err := smt.SetContractStorage(addr.String(), as)
	return err
}

func updateCodeInTree(smt *smt.SMT, addr string, code string) error {
	return smt.SetContractBytecode(addr, code)
}
