package zkchainconfig

import (
	libcommon "github.com/ledgerwatch/erigon-lib/common"
)

var chainIds = []uint64{
	1101,
	1440,
	1442,
}

var contractAddresses = map[uint64]libcommon.Address{
	1101: libcommon.HexToAddress("0x5132A183E9F3CB7C848b0AAC5Ae0c4f0491B7aB2"),
	1440: libcommon.HexToAddress("0xa997cfD539E703921fD1e3Cf25b4c241a27a4c7A"),
	1442: libcommon.HexToAddress("0xEfF10DB3c6445FB06411c6fc74fDCC8D1019aC7d"),
}

func GetContractAddress(chainId uint64) libcommon.Address {
	return contractAddresses[chainId]
}

func IsZk(chainId uint64) bool {
	for _, validId := range chainIds {
		if chainId == validId {
			return true
		}
	}
	return false
}

func IsTestnet(chainId uint64) bool {
	return chainId == 1442
}

func IsDevnet(chainId uint64) bool {
	return chainId == 1440
}

func CheckForkOrder() error {
	return nil
}
