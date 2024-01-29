package state

import (
	"errors"
	"github.com/holiman/uint256"
	"github.com/iden3/go-iden3-crypto/keccak256"
	libcommon "github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon/common"
	"github.com/ledgerwatch/erigon/zk/hermez_db"
)

var (
	saddr      = libcommon.HexToAddress("0x000000000000000000000000000000005ca1ab1e")
	sl0        = libcommon.HexToHash("0x0")
	gerAddress = libcommon.HexToAddress("0xa40D5f56745a118D0906a34E69aeC8C0Db1cB8fA")
)

type ReadOnlyHermezDb interface {
	GetEffectiveGasPricePercentage(txHash libcommon.Hash) (uint8, error)
	GetStateRoot(l2BlockNo uint64) (libcommon.Hash, error)
}

func (sdb *IntraBlockState) GetTxCount() (uint64, error) {
	counter, ok := sdb.stateReader.(TxCountReader)
	if !ok {
		return 0, errors.New("state reader does not support GetTxCount")
	}
	return counter.GetTxCount()
}

func (sdb *IntraBlockState) ScalableSetTxNum() {
	txNum := uint256.NewInt(0)
	sdb.GetState(saddr, &sl0, txNum)

	txNum.Add(txNum, uint256.NewInt(1))

	if !sdb.Exist(saddr) {
		// create account if not exists
		sdb.CreateAccount(saddr, true)
	}

	// set incremented tx num in state
	sdb.SetState(saddr, &sl0, *txNum)
}

func (sdb *IntraBlockState) ScalableSetSmtRootHash(roHermezDb ReadOnlyHermezDb) error {
	txNum := uint256.NewInt(0)
	sdb.GetState(saddr, &sl0, txNum)

	// create mapping with keccak256(txnum,1) -> smt root
	d1 := common.LeftPadBytes(txNum.Bytes(), 32)
	d2 := common.LeftPadBytes(uint256.NewInt(1).Bytes(), 32)
	mapKey := keccak256.Hash(d1, d2)
	mkh := libcommon.BytesToHash(mapKey)

	rpcHash, err := roHermezDb.GetStateRoot(txNum.Uint64())
	if err != nil {
		return err
	}

	if txNum.Uint64() >= 1 {
		// set mapping of keccak256(txnum,1) -> smt root
		rpcHashU256 := uint256.NewInt(0).SetBytes(rpcHash.Bytes())
		sdb.SetState(saddr, &mkh, *rpcHashU256)
	}

	return nil
}

func (sdb *IntraBlockState) ScalableSetBlockNumberToHash(blockNumber uint64, rodb ReadOnlyHermezDb) error {
	d1 := common.LeftPadBytes(hermez_db.Uint64ToBytes(blockNumber), 32)
	d2 := common.LeftPadBytes(uint256.NewInt(1).Bytes(), 32)
	mapKey := keccak256.Hash(d1, d2)
	mkh := libcommon.BytesToHash(mapKey)
	rpcHash, err := rodb.GetStateRoot(blockNumber)
	if err != nil {
		return err
	}
	rpcU256 := uint256.NewInt(0).SetBytes(rpcHash.Bytes())
	sdb.SetState(saddr, &mkh, *rpcU256)
	return nil
}

func (sdb *IntraBlockState) ReadGerManagerL1BlockHash(ger libcommon.Hash) libcommon.Hash {
	d1 := common.LeftPadBytes(ger.Bytes(), 32)
	d2 := common.LeftPadBytes(uint256.NewInt(0).Bytes(), 32)
	mapKey := keccak256.Hash(d1, d2)
	mkh := libcommon.BytesToHash(mapKey)
	key := uint256.NewInt(0)
	sdb.GetState(gerAddress, &mkh, key)
	if key.Uint64() == 0 {
		return libcommon.Hash{}
	}
	return libcommon.BytesToHash(key.Bytes())
}

func (sdb *IntraBlockState) WriteGerManagerL1BlockHash(ger, l1BlockHash libcommon.Hash) {
	d1 := common.LeftPadBytes(ger.Bytes(), 32)
	d2 := common.LeftPadBytes(uint256.NewInt(0).Bytes(), 32)
	mapKey := keccak256.Hash(d1, d2)
	mkh := libcommon.BytesToHash(mapKey)
	val := uint256.NewInt(0).SetBytes(l1BlockHash.Bytes())
	sdb.SetState(gerAddress, &mkh, *val)
}
