package rawdb

import (
	"encoding/binary"

	"github.com/gateway-fm/cdk-erigon-lib/kv"
)

func GetLatestStateVersion(tx kv.Tx) (uint64, error) {
	c, err := tx.Cursor(kv.PLAIN_STATE_VERSION)
	if err != nil {
		return 0, err
	}
	defer c.Close()

	_, plainStateVersion, err := c.Last()
	if err != nil {
		return 0, err
	}
	if len(plainStateVersion) == 0 {
		return 0, nil
	}

	return binary.BigEndian.Uint64(plainStateVersion), nil
}

func IncrementStateVersionByBlockNumber(tx kv.RwTx, blockNum uint64) (uint64, error) {
	plainStateVersion, err := GetLatestStateVersion(tx)
	if err != nil {
		return 0, err
	}

	c, err := tx.RwCursor(kv.PLAIN_STATE_VERSION)
	if err != nil {
		return 0, err
	}
	defer c.Close()

	newKBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(newKBytes, blockNum)
	newVBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(newVBytes, plainStateVersion+1)
	if err := c.Put(newKBytes, newVBytes); err != nil {
		return 0, err
	}
	return plainStateVersion + 1, nil
}

// delete all keys that are >= fromBlockNumber
// keys are sorted in accending order
func TruncateStateVersion(tx kv.RwTx, fromBlockNumber uint64) error {
	c, err := tx.RwCursor(kv.PLAIN_STATE_VERSION)
	if err != nil {
		return err
	}
	defer c.Close()

	for k, _, err := c.Last(); k != nil; k, _, err = c.Prev() {
		if err != nil {
			return err
		}
		bn := binary.BigEndian.Uint64(k)
		if bn < fromBlockNumber {
			break
		}
		if err = c.DeleteCurrent(); err != nil {
			return err
		}
	}

	return nil
}

// func print(tx kv.RwTx) {
// 	// var blockNumber, value uint64
// 	c, err := tx.Cursor(kv.PLAIN_STATE_VERSION)
// 	if err != nil {
// 		panic(err)
// 	}
// 	defer c.Close()

// 	for k, v, err := c.First(); k != nil; k, v, err = c.Next() {
// 		if err != nil {
// 			panic(err)
// 		}
// 		// binary.BigEndian.PutUint64(k, blockNumber)
// 		// binary.BigEndian.PutUint64(v, value)
// 		fmt.Printf("Block (%d) -> Value (%d)\n", binary.BigEndian.Uint64(k), binary.BigEndian.Uint64(v))
// 	}
// }
