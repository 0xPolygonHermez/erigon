package smt_test

import (
	"context"
	"fmt"
	"math/big"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/c2h5oh/datasize"
	"github.com/holiman/uint256"
	libcommon "github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/kv/mdbx"
	"github.com/ledgerwatch/erigon/core/types/accounts"
	"github.com/ledgerwatch/erigon/migrations"
	"github.com/ledgerwatch/erigon/smt/pkg/db"
	"github.com/ledgerwatch/erigon/smt/pkg/smt"
	"github.com/ledgerwatch/erigon/smt/pkg/utils"
	"github.com/ledgerwatch/log/v3"
	"golang.org/x/sync/semaphore"
	"gotest.tools/v3/assert"
)

type BatchInsertDataHolder struct {
	acc             accounts.Account
	AddressAccount  libcommon.Address
	AddressContract libcommon.Address
	Bytecode        string
	Storage         map[string]string
}

func TestBatchSimpleInsert(t *testing.T) {
	keysRaw := []*big.Int{
		big.NewInt(8),
		big.NewInt(8),
		big.NewInt(1),
		big.NewInt(31),
		big.NewInt(31),
		big.NewInt(0),
		big.NewInt(8),
		// big.NewInt(1),
	}
	valuesRaw := []*big.Int{
		big.NewInt(17),
		big.NewInt(18),
		big.NewInt(19),
		big.NewInt(20),
		big.NewInt(0),
		big.NewInt(0),
		big.NewInt(0),
		// big.NewInt(0),
	}

	keyPointers := []*utils.NodeKey{}
	valuePointers := []*utils.NodeValue8{}

	smtIncremental := smt.NewSMT(nil)
	smtBatch := smt.NewSMT(nil)

	for i := range keysRaw {
		k := utils.ScalarToNodeKey(keysRaw[i])
		vArray := utils.ScalarToArrayBig(valuesRaw[i])
		v, _ := utils.NodeValue8FromBigIntArray(vArray)

		keyPointers = append(keyPointers, &k)
		valuePointers = append(valuePointers, v)

		smtIncremental.InsertKA(k, valuesRaw[i])
	}

	_, err := smtBatch.InsertBatch("", keyPointers, valuePointers, nil, nil)
	assert.NilError(t, err)

	smtIncremental.DumpTree()
	fmt.Println()
	smtBatch.DumpTree()
	fmt.Println()
	fmt.Println()
	fmt.Println()

	smtIncrementalRootHash, _ := smtIncremental.Db.GetLastRoot()
	smtBatchRootHash, _ := smtBatch.Db.GetLastRoot()
	assert.Equal(t, utils.ConvertBigIntToHex(smtBatchRootHash), utils.ConvertBigIntToHex(smtIncrementalRootHash))

	assertSmtDbStructure(t, smtBatch, false)
}

func TestBatchDelete(t *testing.T) {
	keys := []utils.NodeKey{
		utils.NodeKey{10768458483543229763, 12393104588695647110, 7306859896719697582, 4178785141502415085},
		utils.NodeKey{7512520260500009967, 3751662918911081259, 9113133324668552163, 12072005766952080289},
		utils.NodeKey{4755722537892498409, 14621988746728905818, 15452350668109735064, 8819587610951133148},
		utils.NodeKey{6340777516277056037, 6264482673611175884, 1063722098746108599, 9062208133640346025},
		utils.NodeKey{6319287575763093444, 10809750365832475266, 6426706394050518186, 9463173325157812560},
		utils.NodeKey{15155415624738072211, 3736290188193138617, 8461047487943769832, 12188454615342744806},
		utils.NodeKey{15276670325385989216, 10944726794004460540, 9369946489424614125, 817372649097925902},
		utils.NodeKey{2562799672200229655, 18444468184514201072, 17883941549041529369, 407038781355273654},
		utils.NodeKey{10768458483543229763, 12393104588695647110, 7306859896719697582, 4178785141502415085},
		utils.NodeKey{7512520260500009967, 3751662918911081259, 9113133324668552163, 12072005766952080289},
		utils.NodeKey{4755722537892498409, 14621988746728905818, 15452350668109735064, 8819587610951133148},
	}

	valuesTemp := [][8]uint64{
		[8]uint64{0, 1, 0, 0, 0, 0, 0, 0},
		[8]uint64{0, 1, 0, 0, 0, 0, 0, 0},
		[8]uint64{0, 1, 0, 0, 0, 0, 0, 0},
		[8]uint64{1, 0, 0, 0, 0, 0, 0, 0},
		[8]uint64{103184848, 115613322, 0, 0, 0, 0, 0, 0},
		[8]uint64{2, 0, 0, 0, 0, 0, 0, 0},
		[8]uint64{3038602192, 2317586098, 794977000, 2442751483, 2309555181, 2028447238, 1023640522, 2687173865},
		[8]uint64{3100, 0, 0, 0, 0, 0, 0, 0},
		[8]uint64{0, 0, 0, 0, 0, 0, 0, 0},
		[8]uint64{0, 0, 0, 0, 0, 0, 0, 0},
		[8]uint64{0, 0, 0, 0, 0, 0, 0, 0},
	}

	values := make([]utils.NodeValue8, 0)
	for _, vT := range valuesTemp {
		values = append(values, utils.NodeValue8{
			big.NewInt(0).SetUint64(vT[0]),
			big.NewInt(0).SetUint64(vT[1]),
			big.NewInt(0).SetUint64(vT[2]),
			big.NewInt(0).SetUint64(vT[3]),
			big.NewInt(0).SetUint64(vT[4]),
			big.NewInt(0).SetUint64(vT[5]),
			big.NewInt(0).SetUint64(vT[6]),
			big.NewInt(0).SetUint64(vT[7]),
		})
	}

	smtIncremental := smt.NewSMT(nil)
	smtBatch := smt.NewSMT(nil)

	for i, k := range keys {
		smtIncremental.Insert(k, values[i])
		_, err := smtBatch.InsertBatch("", []*utils.NodeKey{&k}, []*utils.NodeValue8{&values[i]}, nil, nil)
		assert.NilError(t, err)

		smtIncrementalRootHash, _ := smtIncremental.Db.GetLastRoot()
		smtBatchRootHash, _ := smtBatch.Db.GetLastRoot()
		assert.Equal(t, utils.ConvertBigIntToHex(smtBatchRootHash), utils.ConvertBigIntToHex(smtIncrementalRootHash))
	}

	smtIncremental.DumpTree()
	fmt.Println()
	smtBatch.DumpTree()
	fmt.Println()

	assertSmtDbStructure(t, smtBatch, false)
}

func TestBatchRawInsert(t *testing.T) {
	keysForBatch := []*utils.NodeKey{}
	valuesForBatch := []*utils.NodeValue8{}

	keysForIncremental := []utils.NodeKey{}
	valuesForIncremental := []utils.NodeValue8{}

	smtIncremental := smt.NewSMT(nil)
	smtBatch := smt.NewSMT(nil)

	rand.Seed(1)
	size := 1 << 16
	for i := 0; i < size; i++ {
		rawKey := big.NewInt(rand.Int63())
		rawValue := big.NewInt(rand.Int63())

		k := utils.ScalarToNodeKey(rawKey)
		vArray := utils.ScalarToArrayBig(rawValue)
		v, _ := utils.NodeValue8FromBigIntArray(vArray)

		keysForBatch = append(keysForBatch, &k)
		valuesForBatch = append(valuesForBatch, v)

		keysForIncremental = append(keysForIncremental, k)
		valuesForIncremental = append(valuesForIncremental, *v)

	}

	startTime := time.Now()
	for i := range keysForIncremental {
		smtIncremental.Insert(keysForIncremental[i], valuesForIncremental[i])
	}
	t.Logf("Incremental insert %d values in %v\n", len(keysForIncremental), time.Since(startTime))

	startTime = time.Now()
	_, err := smtBatch.InsertBatch("", keysForBatch, valuesForBatch, nil, nil)
	assert.NilError(t, err)
	t.Logf("Batch insert %d values in %v\n", len(keysForBatch), time.Since(startTime))

	smtIncrementalRootHash, _ := smtIncremental.Db.GetLastRoot()
	smtBatchRootHash, _ := smtBatch.Db.GetLastRoot()
	assert.Equal(t, utils.ConvertBigIntToHex(smtBatchRootHash), utils.ConvertBigIntToHex(smtIncrementalRootHash))

	assertSmtDbStructure(t, smtBatch, false)

	// DELETE
	keysForBatchDelete := []*utils.NodeKey{}
	valuesForBatchDelete := []*utils.NodeValue8{}

	keysForIncrementalDelete := []utils.NodeKey{}
	valuesForIncrementalDelete := []utils.NodeValue8{}

	sizeToDelete := 1 << 14
	for i := 0; i < sizeToDelete; i++ {
		rawValue := big.NewInt(0)
		vArray := utils.ScalarToArrayBig(rawValue)
		v, _ := utils.NodeValue8FromBigIntArray(vArray)

		deleteIndex := rand.Intn(size)

		keyForBatchDelete := keysForBatch[deleteIndex]
		keyForIncrementalDelete := keysForIncremental[deleteIndex]

		keysForBatchDelete = append(keysForBatchDelete, keyForBatchDelete)
		valuesForBatchDelete = append(valuesForBatchDelete, v)

		keysForIncrementalDelete = append(keysForIncrementalDelete, keyForIncrementalDelete)
		valuesForIncrementalDelete = append(valuesForIncrementalDelete, *v)
	}

	startTime = time.Now()
	for i := range keysForIncrementalDelete {
		smtIncremental.Insert(keysForIncrementalDelete[i], valuesForIncrementalDelete[i])
	}
	t.Logf("Incremental delete %d values in %v\n", len(keysForIncrementalDelete), time.Since(startTime))

	startTime = time.Now()
	_, err = smtBatch.InsertBatch("", keysForBatchDelete, valuesForBatchDelete, nil, nil)
	assert.NilError(t, err)
	t.Logf("Batch delete %d values in %v\n", len(keysForBatchDelete), time.Since(startTime))

	assertSmtDbStructure(t, smtBatch, false)
}

func TestCompareAllTreesInsertTimesAndFinalHashesUsingDiskDb(t *testing.T) {
	incrementalDbPath := "/tmp/smt-incremental"
	smtIncrementalDb, smtIncrementalTx, smtIncrementalSmtDb := initDb(t, incrementalDbPath)

	bulkDbPath := "/tmp/smt-bulk"
	smtBulkDb, smtBulkTx, smtBulkSmtDb := initDb(t, bulkDbPath)

	batchDbPath := "/tmp/smt-batch"
	smtBatchDb, smtBatchTx, smtBatchSmtDb := initDb(t, batchDbPath)

	smtIncremental := smt.NewSMT(smtIncrementalSmtDb)
	smtBulk := smt.NewSMT(smtBulkSmtDb)
	smtBatch := smt.NewSMT(smtBatchSmtDb)

	compareAllTreesInsertTimesAndFinalHashes(t, smtIncremental, smtBulk, smtBatch)

	smtIncrementalTx.Commit()
	smtBulkTx.Commit()
	smtBatchTx.Commit()
	t.Cleanup(func() {
		smtIncrementalDb.Close()
		smtBulkDb.Close()
		smtBatchDb.Close()
		os.RemoveAll(incrementalDbPath)
		os.RemoveAll(bulkDbPath)
		os.RemoveAll(batchDbPath)
	})
}

func TestCompareAllTreesInsertTimesAndFinalHashesUsingInMemoryDb(t *testing.T) {
	smtIncremental := smt.NewSMT(nil)
	smtBulk := smt.NewSMT(nil)
	smtBatch := smt.NewSMT(nil)

	compareAllTreesInsertTimesAndFinalHashes(t, smtIncremental, smtBulk, smtBatch)
}

func compareAllTreesInsertTimesAndFinalHashes(t *testing.T, smtIncremental, smtBulk, smtBatch *smt.SMT) {
	batchInsertDataHolders, totalInserts := prepareData()

	var incrementalError error

	accChanges := make(map[libcommon.Address]*accounts.Account)
	codeChanges := make(map[libcommon.Address]string)
	storageChanges := make(map[libcommon.Address]map[string]string)

	for _, batchInsertDataHolder := range batchInsertDataHolders {
		accChanges[batchInsertDataHolder.AddressAccount] = &batchInsertDataHolder.acc
		codeChanges[batchInsertDataHolder.AddressContract] = batchInsertDataHolder.Bytecode
		storageChanges[batchInsertDataHolder.AddressContract] = batchInsertDataHolder.Storage
	}

	startTime := time.Now()
	for addr, acc := range accChanges {
		if err := smtIncremental.SetAccountStorage(addr, acc); err != nil {
			incrementalError = err
		}
	}

	for addr, code := range codeChanges {
		if err := smtIncremental.SetContractBytecode(addr.String(), code); err != nil {
			incrementalError = err
		}
	}

	for addr, storage := range storageChanges {
		if _, err := smtIncremental.SetContractStorage(addr.String(), storage); err != nil {
			incrementalError = err
		}
	}

	assert.NilError(t, incrementalError)
	t.Logf("Incremental insert %d values in %v\n", totalInserts, time.Since(startTime))

	startTime = time.Now()
	keyPointers, valuePointers, err := smtBatch.SetStorage("", accChanges, codeChanges, storageChanges)
	assert.NilError(t, err)
	t.Logf("Batch insert %d values in %v\n", totalInserts, time.Since(startTime))

	keys := []utils.NodeKey{}
	for i, key := range keyPointers {
		v := valuePointers[i]
		if !v.IsZero() {
			smtBulk.Db.InsertAccountValue(*key, *v)
			keys = append(keys, *key)
		}
	}
	startTime = time.Now()
	smtBulk.GenerateFromKVBulk("", keys)
	t.Logf("Bulk insert %d values in %v\n", totalInserts, time.Since(startTime))

	smtIncrementalRootHash, _ := smtIncremental.Db.GetLastRoot()
	smtBatchRootHash, _ := smtBatch.Db.GetLastRoot()
	smtBulkRootHash, _ := smtBulk.Db.GetLastRoot()
	assert.Equal(t, utils.ConvertBigIntToHex(smtBatchRootHash), utils.ConvertBigIntToHex(smtIncrementalRootHash))
	assert.Equal(t, utils.ConvertBigIntToHex(smtBulkRootHash), utils.ConvertBigIntToHex(smtIncrementalRootHash))

	assertSmtDbStructure(t, smtBatch, true)
}

func initDb(t *testing.T, dbPath string) (kv.RwDB, kv.RwTx, *db.EriDb) {
	ctx := context.Background()

	os.RemoveAll(dbPath)

	dbOpts := mdbx.NewMDBX(log.Root()).Path(dbPath).Label(kv.ChainDB).GrowthStep(16 * datasize.MB).RoTxsLimiter(semaphore.NewWeighted(128))
	database, err := dbOpts.Open()
	if err != nil {
		t.Fatalf("Cannot create db %e", err)
	}

	migrator := migrations.NewMigrator(kv.ChainDB)
	if err := migrator.VerifyVersion(database); err != nil {
		t.Fatalf("Cannot verify db version %e", err)
	}
	if err = migrator.Apply(database, dbPath); err != nil {
		t.Fatalf("Cannot migrate db %e", err)
	}

	// if err := database.Update(context.Background(), func(tx kv.RwTx) (err error) {
	// 	return params.SetErigonVersion(tx, "test")
	// }); err != nil {
	// 	t.Fatalf("Cannot update db")
	// }

	dbTransaction, err := database.BeginRw(ctx)
	if err != nil {
		t.Fatalf("Cannot craete db transaction")
	}

	db.CreateEriDbBuckets(dbTransaction)
	return database, dbTransaction, db.NewEriDb(dbTransaction)
}

func prepareData() ([]*BatchInsertDataHolder, int) {
	treeSize := 1500
	storageSize := 96
	batchInsertDataHolders := make([]*BatchInsertDataHolder, 0)
	rand.Seed(1)
	for i := 0; i < treeSize; i++ {
		storage := make(map[string]string)
		addressAccountBytes := make([]byte, 20)
		addressContractBytes := make([]byte, 20)
		storageKeyBytes := make([]byte, 20)
		storageValueBytes := make([]byte, 20)
		rand.Read(addressAccountBytes)
		rand.Read(addressContractBytes)

		for j := 0; j < storageSize; j++ {
			rand.Read(storageKeyBytes)
			rand.Read(storageValueBytes)
			storage[libcommon.BytesToAddress(storageKeyBytes).Hex()] = libcommon.BytesToAddress(storageValueBytes).Hex()
		}

		acc := accounts.NewAccount()
		acc.Balance = *uint256.NewInt(rand.Uint64())
		acc.Nonce = rand.Uint64()

		batchInsertDataHolders = append(batchInsertDataHolders, &BatchInsertDataHolder{
			acc: acc,
			// Balance:  big.NewInt(rand.Int63()),
			// Nonce:    big.NewInt(rand.Int63()),
			AddressAccount:  libcommon.BytesToAddress(addressAccountBytes),
			AddressContract: libcommon.BytesToAddress(addressContractBytes),
			Bytecode:        "0x60806040526004361061007b5760003560e01c80639623609d1161004e5780639623609d1461012b57806399a88ec41461013e578063f2fde38b1461015e578063f3b7dead1461017e57600080fd5b8063204e1c7a14610080578063715018a6146100c95780637eff275e146100e05780638da5cb5b14610100575b600080fd5b34801561008c57600080fd5b506100a061009b366004610608565b61019e565b60405173ffffffffffffffffffffffffffffffffffffffff909116815260200160405180910390f35b3480156100d557600080fd5b506100de610255565b005b3480156100ec57600080fd5b506100de6100fb36600461062c565b610269565b34801561010c57600080fd5b5060005473ffffffffffffffffffffffffffffffffffffffff166100a0565b6100de610139366004610694565b6102f7565b34801561014a57600080fd5b506100de61015936600461062c565b61038c565b34801561016a57600080fd5b506100de610179366004610608565b6103e8565b34801561018a57600080fd5b506100a0610199366004610608565b6104a4565b60008060008373ffffffffffffffffffffffffffffffffffffffff166040516101ea907f5c60da1b00000000000000000000000000000000000000000000000000000000815260040190565b600060405180830381855afa9150503d8060008114610225576040519150601f19603f3d011682016040523d82523d6000602084013e61022a565b606091505b50915091508161023957600080fd5b8080602001905181019061024d9190610788565b949350505050565b61025d6104f0565b6102676000610571565b565b6102716104f0565b6040517f8f28397000000000000000000000000000000000000000000000000000000000815273ffffffffffffffffffffffffffffffffffffffff8281166004830152831690638f283970906024015b600060405180830381600087803b1580156102db57600080fd5b505af11580156102ef573d6000803e3d6000fd5b505050505050565b6102ff6104f0565b6040517f4f1ef28600000000000000000000000000000000000000000000000000000000815273ffffffffffffffffffffffffffffffffffffffff841690634f1ef28690349061035590869086906004016107a5565b6000604051808303818588803b15801561036e57600080fd5b505af1158015610382573d6000803e3d6000fd5b5050505050505050565b6103946104f0565b6040517f3659cfe600000000000000000000000000000000000000000000000000000000815273ffffffffffffffffffffffffffffffffffffffff8281166004830152831690633659cfe6906024016102c1565b6103f06104f0565b73ffffffffffffffffffffffffffffffffffffffff8116610498576040517f08c379a000000000000000000000000000000000000000000000000000000000815260206004820152602660248201527f4f776e61626c653a206e6577206f776e657220697320746865207a65726f206160448201527f646472657373000000000000000000000000000000000000000000000000000060648201526084015b60405180910390fd5b6104a181610571565b50565b60008060008373ffffffffffffffffffffffffffffffffffffffff166040516101ea907ff851a44000000000000000000000000000000000000000000000000000000000815260040190565b60005473ffffffffffffffffffffffffffffffffffffffff163314610267576040517f08c379a000000000000000000000000000000000000000000000000000000000815260206004820181905260248201527f4f776e61626c653a2063616c6c6572206973206e6f7420746865206f776e6572604482015260640161048f565b6000805473ffffffffffffffffffffffffffffffffffffffff8381167fffffffffffffffffffffffff0000000000000000000000000000000000000000831681178455604051919092169283917f8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e09190a35050565b73ffffffffffffffffffffffffffffffffffffffff811681146104a157600080fd5b60006020828403121561061a57600080fd5b8135610625816105e6565b9392505050565b6000806040838503121561063f57600080fd5b823561064a816105e6565b9150602083013561065a816105e6565b809150509250929050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052604160045260246000fd5b6000806000606084860312156106a957600080fd5b83356106b4816105e6565b925060208401356106c4816105e6565b9150604084013567ffffffffffffffff808211156106e157600080fd5b818601915086601f8301126106f557600080fd5b81358181111561070757610707610665565b604051601f82017fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe0908116603f0116810190838211818310171561074d5761074d610665565b8160405282815289602084870101111561076657600080fd5b8260208601602083013760006020848301015280955050505050509250925092565b60006020828403121561079a57600080fd5b8151610625816105e6565b73ffffffffffffffffffffffffffffffffffffffff8316815260006020604081840152835180604085015260005b818110156107ef578581018301518582016060015282016107d3565b5060006060828601015260607fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe0601f83011685010192505050939250505056fea2646970667358221220372a0e10eebea1b7fa43ae4c976994e6ed01d85eedc3637b83f01d3f06be442064736f6c63430008110033",
			Storage:         storage,
		})
	}

	return batchInsertDataHolders, treeSize*4 + treeSize*storageSize
}

func assertSmtDbStructure(t *testing.T, s *smt.SMT, testMetadata bool) {
	smtBatchRootHash, _ := s.Db.GetLastRoot()

	actualDb, ok := s.Db.(*db.MemDb)
	if !ok {
		return
	}

	usedNodeHashesMap := make(map[string]*utils.NodeKey)
	assertSmtTreeDbStructure(t, s, utils.ScalarToRoot(smtBatchRootHash), usedNodeHashesMap)

	assert.Equal(t, len(usedNodeHashesMap), len(actualDb.Db))
	for k := range usedNodeHashesMap {
		_, found := actualDb.Db[k]
		assert.Equal(t, true, found)
	}

	totalLeaves := assertHashToKeyDbStrcture(t, s, utils.ScalarToRoot(smtBatchRootHash), testMetadata)
	assert.Equal(t, totalLeaves, len(actualDb.DbHashKey))
	if testMetadata {
		assert.Equal(t, totalLeaves, len(actualDb.DbKeySource))
	}
}

func assertSmtTreeDbStructure(t *testing.T, s *smt.SMT, nodeHash utils.NodeKey, usedNodeHashesMap map[string]*utils.NodeKey) {
	if nodeHash.IsZero() {
		return
	}

	dbNodeValue, err := s.Db.Get(nodeHash)
	assert.NilError(t, err)

	nodeHashHex := utils.ConvertBigIntToHex(utils.ArrayToScalar(nodeHash[:]))
	usedNodeHashesMap[nodeHashHex] = &nodeHash

	if dbNodeValue.IsFinalNode() {
		nodeValueHash := utils.NodeKeyFromBigIntArray(dbNodeValue[4:8])
		dbNodeValue, err = s.Db.Get(nodeValueHash)
		assert.NilError(t, err)

		nodeHashHex := utils.ConvertBigIntToHex(utils.ArrayToScalar(nodeValueHash[:]))
		usedNodeHashesMap[nodeHashHex] = &nodeValueHash
		return
	}

	assertSmtTreeDbStructure(t, s, utils.NodeKeyFromBigIntArray(dbNodeValue[0:4]), usedNodeHashesMap)
	assertSmtTreeDbStructure(t, s, utils.NodeKeyFromBigIntArray(dbNodeValue[4:8]), usedNodeHashesMap)
}

func assertHashToKeyDbStrcture(t *testing.T, smtBatch *smt.SMT, nodeHash utils.NodeKey, testMetadata bool) int {
	if nodeHash.IsZero() {
		return 0
	}

	dbNodeValue, err := smtBatch.Db.Get(nodeHash)
	assert.NilError(t, err)

	if dbNodeValue.IsFinalNode() {
		memDb := smtBatch.Db.(*db.MemDb)

		nodeKey, err := smtBatch.Db.GetHashKey(nodeHash)
		assert.NilError(t, err)

		keyConc := utils.ArrayToScalar(nodeHash[:])
		k := utils.ConvertBigIntToHex(keyConc)
		_, found := memDb.DbHashKey[k]
		assert.Equal(t, found, true)

		if testMetadata {
			keyConc = utils.ArrayToScalar(nodeKey[:])

			_, found = memDb.DbKeySource[keyConc.String()]
			assert.Equal(t, found, true)
		}
		return 1
	}

	return assertHashToKeyDbStrcture(t, smtBatch, utils.NodeKeyFromBigIntArray(dbNodeValue[0:4]), testMetadata) + assertHashToKeyDbStrcture(t, smtBatch, utils.NodeKeyFromBigIntArray(dbNodeValue[4:8]), testMetadata)
}
