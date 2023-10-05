package zkchainconfig

import "math/big"

var chainIds = []uint64{
	1101,
	1442,
}

func IsZk(chainId *big.Int) bool {
	id := chainId.Uint64()
	for _, validId := range chainIds {
		if id == validId {
			return true
		}
	}
	return false
}

func CheckForkOrder() error {
	return nil
}
