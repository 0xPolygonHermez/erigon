package db

import (
	"fmt"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon/ethdb"
	"github.com/ledgerwatch/erigon/ethdb/olddb"
	"github.com/ledgerwatch/erigon/smt/pkg/utils"
	"math/big"
)

type SmtDbTx interface {
	GetOne(bucket string, key []byte) ([]byte, error)
	Put(bucket string, key []byte, value []byte) error
	Has(bucket string, key []byte) (bool, error)
	Delete(bucket string, key []byte) error
	ForEach(bucket string, start []byte, fn func(k, v []byte) error) error
	ForPrefix(bucket string, prefix []byte, fn func(k, v []byte) error) error
	Commit() error
	Rollback()
}

type EriDb struct {
	kvTx kv.RwTx
	tx   SmtDbTx
}

func NewEriDb(tx kv.RwTx) *EriDb {
	err := tx.CreateBucket("HermezSmt")
	if err != nil {
		fmt.Println(err)
	}

	err = tx.CreateBucket("HermezSmtLastRoot")
	if err != nil {
		fmt.Println(err)
	}

	return &EriDb{
		kvTx: tx,
		tx:   tx,
	}
}

func (m *EriDb) OpenBatch(quitCh <-chan struct{}) {
	var batch ethdb.DbWithPendingMutations
	batch = olddb.NewHashBatch(m.kvTx, quitCh, "./tempdb")
	defer func() {
		batch.Rollback()
	}()
	m.tx = batch
}

func (m *EriDb) CommitBatch() error {
	if _, ok := m.tx.(ethdb.DbWithPendingMutations); !ok {
		return nil // don't commit a kvRw tx
	}
	err := m.tx.Commit()
	if err != nil {
		m.tx.Rollback()
		return err
	}
	return nil
}

func (m *EriDb) RollbackBatch() {
	if _, ok := m.tx.(ethdb.DbWithPendingMutations); !ok {
		return // don't roll back a kvRw tx
	}
	m.tx.Rollback()
}

func (m *EriDb) GetLastRoot() (*big.Int, error) {
	data, err := m.tx.GetOne("HermezSmtLastRoot", []byte("lastRoot"))
	if err != nil {
		return big.NewInt(0), err
	}

	if data == nil {
		return big.NewInt(0), nil
	}

	return utils.ConvertHexToBigInt(string(data)), nil
}

func (m *EriDb) SetLastRoot(r *big.Int) error {
	v := utils.ConvertBigIntToHex(r)
	return m.tx.Put("HermezSmtLastRoot", []byte("lastRoot"), []byte(v))
}

func (m *EriDb) Get(key utils.NodeKey) (utils.NodeValue12, error) {
	keyConc := utils.ArrayToScalar(key[:])
	k := utils.ConvertBigIntToHex(keyConc)

	data, err := m.tx.GetOne("HermezSmt", []byte(k))
	if err != nil {
		return utils.NodeValue12{}, err
	}

	if data == nil {
		return utils.NodeValue12{}, nil
	}

	vConc := utils.ConvertHexToBigInt(string(data))
	val := utils.ScalarToNodeValue(vConc)

	return val, nil
}

func (m *EriDb) Insert(key utils.NodeKey, value utils.NodeValue12) error {
	keyConc := utils.ArrayToScalar(key[:])
	k := utils.ConvertBigIntToHex(keyConc)

	vals := make([]*big.Int, 12)
	for i, v := range value {
		vals[i] = v
	}

	vConc := utils.ArrayToScalarBig(vals[:])
	v := utils.ConvertBigIntToHex(vConc)

	return m.tx.Put("HermezSmt", []byte(k), []byte(v))
}

func (m *EriDb) Delete(key string) error {
	return m.tx.Delete("HermezSmt", []byte(key))
}

func (m *EriDb) PrintDb() {
	err := m.tx.ForEach("HermezSmt", []byte{}, func(k, v []byte) error {
		println(string(k), string(v))
		return nil
	})
	if err != nil {
		println(err)
	}
}

func (m *EriDb) GetDb() map[string]string {
	db := make(map[string]string)

	err := m.tx.ForEach("HermezSmt", []byte{}, func(k, v []byte) error {
		kStr := string(k)
		vStr := string(v)

		db[kStr] = vStr

		return nil
	})

	if err != nil {
		fmt.Println(err.Error())
	}

	return db
}
