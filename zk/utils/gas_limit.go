package utils

const (
	preForkId7BlockGasLimit = 30_000_000
	forkId7BlockGasLimit    = 18446744073709551615 // 0xffffffffffffffff
	forkId8BlockGasLimit    = 1125899906842624     // 0x4000000000000
)

func GetExecutionGasLimit(forkId uint64) uint64 {
	if forkId >= 8 {
		return forkId8BlockGasLimit
	}

	// [hack] the rpc returns forkid8 value, but forkid7 is used in execution
	if forkId == 7 {
		return forkId8BlockGasLimit
		// return forkId7BlockGasLimit
	}

	return preForkId7BlockGasLimit
}
