package db

import (
	"fmt"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon/smt/pkg/utils"
	"math/big"
)

type EriDb struct {
	tx kv.RwTx
}

func NewEriDb(tx kv.RwTx) *EriDb {

	_ = tx.CreateBucket("HermezSmt")
	_ = tx.CreateBucket("HermezSmtLastRoot")

	return &EriDb{
		tx: tx,
	}
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

func (m *EriDb) IsEmpty() bool {
	s, err := m.tx.BucketSize("HermezSmt")
	if err != nil {
		return true
	}

	return s == 0
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
