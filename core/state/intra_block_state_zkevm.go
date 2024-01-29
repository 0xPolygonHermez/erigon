package state

import (
	"errors"
	"fmt"

	"github.com/holiman/uint256"
	"github.com/iden3/go-iden3-crypto/keccak256"
	libcommon "github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon/chain"
	"github.com/ledgerwatch/erigon/common"
	"github.com/ledgerwatch/erigon/core/types"
)

const (
	LAST_BLOCK_STORAGE_POS      = "0x0"
	STATE_ROOT_STORAGE_POS      = "0x1"
	TIMESTAMP_STORAGE_POS       = "0x2"
	BLOCK_INFO_ROOT_STORAGE_POS = "0x3"
	ADDRESS_SCALABLE_L2         = "0x000000000000000000000000000000005ca1ab1e"
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

func (sdb *IntraBlockState) PostExecuteStateSet(chainConfig *chain.Config, blockNum uint64, l1InfoRoot, stateRoot *libcommon.Hash) {
	//ETROG
	if chainConfig.IsEtrog(blockNum) {
		saddr := libcommon.HexToAddress(ADDRESS_SCALABLE_L2)
		sdb.scalableSetBlockInfoRoot(&saddr, l1InfoRoot)
	}
}

func (sdb *IntraBlockState) PreExecuteStateSet(chainConfig *chain.Config, block *types.Block, stateRoot *libcommon.Hash) {
	saddr := libcommon.HexToAddress(ADDRESS_SCALABLE_L2)

	if !sdb.Exist(saddr) {
		// create account if not exists
		sdb.CreateAccount(saddr, true)
	}

	blockNum := block.Number().Uint64()
	//save block number
	sdb.scalableSetBlockNum(&saddr, blockNum)

	//ETROG
	if chainConfig.IsEtrog(blockNum) {
		//save block timestamp
		sdb.scalableSetTimestamp(&saddr, block.Time())

		//save prev block hash
		sdb.scalableSetBlockHash(&saddr, blockNum-1, stateRoot)
	}
}

func (sdb *IntraBlockState) scalableSetBlockInfoRoot(saddr *libcommon.Address, l1InfoRoot *libcommon.Hash) {
	l1InfoRootSlot := libcommon.HexToHash(BLOCK_INFO_ROOT_STORAGE_POS)

	fmt.Println("l1InfoRoot", l1InfoRoot.Hex())
	l1InfoRootBigU := uint256.NewInt(0).SetBytes(l1InfoRoot.Bytes())

	sdb.SetState(*saddr, &l1InfoRootSlot, *l1InfoRootBigU)

}
func (sdb *IntraBlockState) scalableSetBlockNum(saddr *libcommon.Address, blockNum uint64) {
	txNumSlot := libcommon.HexToHash(LAST_BLOCK_STORAGE_POS)

	sdb.SetState(*saddr, &txNumSlot, *uint256.NewInt(blockNum))
}

func (sdb *IntraBlockState) scalableSetTimestamp(saddr *libcommon.Address, timestamp uint64) {
	timestampSlot := libcommon.HexToHash(TIMESTAMP_STORAGE_POS)

	sdb.SetState(*saddr, &timestampSlot, *uint256.NewInt(timestamp))
}

func (sdb *IntraBlockState) scalableSetBlockHash(saddr *libcommon.Address, blockNum uint64, blockHash *libcommon.Hash) {
	// create mapping with keccak256(blockNum,position) -> smt root
	d1 := common.LeftPadBytes(uint256.NewInt(blockNum).Bytes(), 32)
	posI, _ := uint256.FromHex(STATE_ROOT_STORAGE_POS)
	d2 := common.LeftPadBytes(posI.Bytes(), 32)
	mapKey := keccak256.Hash(d1, d2)
	mkh := libcommon.BytesToHash(mapKey)

	hashAsBigU := uint256.NewInt(0).SetBytes(blockHash.Bytes())

	sdb.SetState(*saddr, &mkh, *hashAsBigU)
}

func (sdb *IntraBlockState) ScalableSetSmtRootHash(roHermezDb ReadOnlyHermezDb) error {
	saddr := libcommon.HexToAddress("0x000000000000000000000000000000005ca1ab1e")
	sl0 := libcommon.HexToHash("0x0")

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
