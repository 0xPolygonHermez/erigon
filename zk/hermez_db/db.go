package hermez_db

import (
	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon/zk/types"
	"github.com/ledgerwatch/log/v3"
)

const L1VERIFICATIONS = "hermez_l1Verifications" // l1blockno, batchno -> l1txhash
const L1SEQUENCES = "hermez_l1Sequences"         // l1blockno, batchno -> l1txhash
const FORKIDS = "hermez_forkIds"                 // batchNo -> forkId
const BLOCKBATCHES = "hermez_blockBatches"       // l2blockno -> batchno

type HermezDb struct {
	tx kv.RwTx
}

func NewHermezDb(tx kv.RwTx) (*HermezDb, error) {
	db := &HermezDb{tx: tx}

	err := db.CreateBuckets()
	if err != nil {
		log.Warn("failed to create buckets", "err", err)
	}

	return db, nil
}

func (db *HermezDb) CreateBuckets() error {
	err := db.tx.CreateBucket(L1VERIFICATIONS)
	if err != nil {
		return err
	}
	err = db.tx.CreateBucket(L1SEQUENCES)
	if err != nil {
		return err
	}
	err = db.tx.CreateBucket(FORKIDS)
	if err != nil {
		return err
	}
	err = db.tx.CreateBucket(BLOCKBATCHES)
	if err != nil {
		return err
	}
	return nil
}

func (db *HermezDb) GetBatchNoByL2Block(l2BlockNo uint64) (uint64, error) {
	c, err := db.tx.Cursor(BLOCKBATCHES)
	if err != nil {
		return 0, err
	}

	k, v, err := c.Seek(UintBytes(l2BlockNo))
	if err != nil {
		return 0, err
	}

	if k == nil {
		return 0, nil
	}

	if BytesUint(k) != l2BlockNo {
		return 0, nil
	}

	return BytesUint(v), nil
}

func (db *HermezDb) GetL2BlockNosByBatch(batchNo uint64) ([]uint64, error) {
	// TODO: not the most efficient way of doing this
	c, err := db.tx.Cursor(BLOCKBATCHES)
	if err != nil {
		return nil, err
	}

	var blockNos []uint64
	var k, v []byte

	for k, v, err = c.First(); k != nil; k, v, err = c.Next() {
		if err != nil {
			break
		}
		if BytesUint(v) == batchNo {
			blockNos = append(blockNos, BytesUint(k))
		}
	}

	return blockNos, err
}

func (db *HermezDb) GetSequenceByL1Block(l1BlockNo uint64) (*types.L1BatchInfo, error) {
	return db.getByL1Block(L1SEQUENCES, l1BlockNo)
}

func (db *HermezDb) GetSequenceByBatchNo(batchNo uint64) (*types.L1BatchInfo, error) {
	return db.getByBatchNo(L1SEQUENCES, batchNo)
}

func (db *HermezDb) GetVerificationByL1Block(l1BlockNo uint64) (*types.L1BatchInfo, error) {
	return db.getByL1Block(L1VERIFICATIONS, l1BlockNo)
}

func (db *HermezDb) GetVerificationByBatchNo(batchNo uint64) (*types.L1BatchInfo, error) {
	return db.getByBatchNo(L1VERIFICATIONS, batchNo)
}

func (db *HermezDb) getByL1Block(table string, l1BlockNo uint64) (*types.L1BatchInfo, error) {
	c, err := db.tx.Cursor(table)
	if err != nil {
		return nil, err
	}

	var k, v []byte
	for k, v, err = c.First(); k != nil; k, v, err = c.Next() {
		if err != nil {
			return nil, err
		}

		l1Block, batchNo, err := SplitKey(k)
		if err != nil {
			return nil, err
		}

		if l1Block == l1BlockNo {
			return &types.L1BatchInfo{
				BatchNo:   batchNo,
				L1BlockNo: l1Block,
				L1TxHash:  common.BytesToHash(v),
			}, nil
		}
	}

	return nil, nil
}

func (db *HermezDb) getByBatchNo(table string, batchNo uint64) (*types.L1BatchInfo, error) {
	c, err := db.tx.Cursor(table)
	if err != nil {
		return nil, err
	}

	var k, v []byte
	for k, v, err = c.First(); k != nil; k, v, err = c.Next() {
		if err != nil {
			return nil, err
		}

		l1Block, batch, err := SplitKey(k)
		if err != nil {
			return nil, err
		}

		if batch == batchNo {
			return &types.L1BatchInfo{
				BatchNo:   batch,
				L1BlockNo: l1Block,
				L1TxHash:  common.BytesToHash(v),
			}, nil
		}
	}

	return nil, nil
}

func (db *HermezDb) GetLatestSequence() (*types.L1BatchInfo, error) {
	return db.getLatest(L1SEQUENCES)
}

func (db *HermezDb) GetLatestVerification() (*types.L1BatchInfo, error) {
	return db.getLatest(L1VERIFICATIONS)
}

func (db *HermezDb) getLatest(table string) (*types.L1BatchInfo, error) {
	c, err := db.tx.Cursor(table)
	if err != nil {
		return nil, err
	}

	k, v, err := c.Last()
	if err != nil {
		return nil, err
	}

	l1BlockNo, batchNo, err := SplitKey(k)
	if err != nil {
		return nil, err
	}

	return &types.L1BatchInfo{
		BatchNo:   batchNo,
		L1BlockNo: l1BlockNo,
		L1TxHash:  common.BytesToHash(v),
	}, nil

}

func (db *HermezDb) WriteSequence(l1BlockNo, batchNo uint64, l1TxHash common.Hash) error {
	return db.tx.Put(L1SEQUENCES, ConcatKey(l1BlockNo, batchNo), l1TxHash.Bytes())
}

func (db *HermezDb) WriteVerification(l1BlockNo, batchNo uint64, l1TxHash common.Hash) error {
	return db.tx.Put(L1VERIFICATIONS, ConcatKey(l1BlockNo, batchNo), l1TxHash.Bytes())
}

func (db *HermezDb) WriteBlockBatch(l2BlockNo, batchNo uint64) error {
	return db.tx.Put(BLOCKBATCHES, UintBytes(l2BlockNo), UintBytes(batchNo))
}

func (db *HermezDb) DeleteBlockBatches(fromBatchNum, toBatchNum uint64) error {
	for i := fromBatchNum; i <= toBatchNum; i++ {
		err := db.tx.Delete(FORKIDS, UintBytes(i))
		if err != nil {
			return err
		}
	}

	return nil
}

func (db *HermezDb) GetForkId(batchNo uint64) (uint64, error) {
	c, err := db.tx.Cursor(FORKIDS)
	if err != nil {
		return 0, err
	}

	var forkId uint64 = 0
	var k, v []byte

	for k, v, err = c.First(); k != nil; k, v, err = c.Next() {
		if err != nil {
			break
		}
		currentBatchNo := BytesUint(k)
		if currentBatchNo <= batchNo {
			forkId = BytesUint(v)
		} else {
			break
		}
	}

	return forkId, err
}

func (db *HermezDb) WriteForkId(batchNo, forkId uint64) error {
	return db.tx.Put(FORKIDS, UintBytes(batchNo), UintBytes(forkId))
}

func (db *HermezDb) DeleteForkIds(fromBatchNum, toBatchNum uint64) error {
	for i := fromBatchNum; i <= toBatchNum; i++ {
		err := db.tx.Delete(FORKIDS, UintBytes(i))
		if err != nil {
			return err
		}
	}

	return nil
}
