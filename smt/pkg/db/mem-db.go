package db

import "github.com/ledgerwatch/erigon/smt/pkg/utils"

type MemDb struct {
	Db map[string][]string
}

func NewMemDb() *MemDb {
	return &MemDb{
		Db: make(map[string][]string),
	}
}

func (m *MemDb) Get(key utils.NodeKey) (utils.NodeValue12, error) {
	keyConc := utils.ArrayToScalar(key[:])

	k := utils.ConvertBigIntToHex(keyConc)

	values := utils.NodeValue12{}
	for i, v := range m.Db[k] {
		values[i] = utils.ConvertHexToBigInt(v)
	}

	return values, nil
}

func (m *MemDb) Insert(key utils.NodeKey, value utils.NodeValue12) error {
	keyConc := utils.ArrayToScalar(key[:])
	k := utils.ConvertBigIntToHex(keyConc)

	values := make([]string, 12)
	for i, v := range value {
		values[i] = utils.ConvertBigIntToHex(v)
	}

	m.Db[k] = values
	return nil
}

func (m *MemDb) PrintDb() {
	for k, v := range m.Db {
		println(k, v)
	}
}
