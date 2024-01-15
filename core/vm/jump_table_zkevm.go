package vm

var (
	zkEVMForkID4InstructionSet = newZkEVMForkID4InstructionSet()
	zkEVMForkID5InstructionSet = newZkEVMForkID5InstructionSet()
	zkEVMForkID7InstructionSet = newZkEVMForkID7InstructionSet()
)

// newZkEVMForkID4InstructionSet returns the instruction set for the forkID4
func newZkEVMForkID4InstructionSet() JumpTable {
	instructionSet := newBerlinInstructionSet()

	enable2929_zkevm(&instructionSet) // Access lists for trie accesses https://eips.ethereum.org/EIPS/eip-2929

	instructionSet[CALLDATALOAD].execute = opCallDataLoad_zkevmIncompatible
	instructionSet[CALLDATACOPY].execute = opCallDataCopy_zkevmIncompatible

	instructionSet[STATICCALL].execute = opStaticCall_zkevm

	instructionSet[NUMBER].execute = opNumber_zkevm

	instructionSet[DIFFICULTY].execute = opDifficulty_zkevm

	instructionSet[BLOCKHASH].execute = opBlockhash_zkevm

	instructionSet[EXTCODEHASH].execute = opExtCodeHash_zkevm

	instructionSet[SENDALL] = &operation{
		execute:    opSendAll_zkevm,
		dynamicGas: gasSelfdestruct_zkevm,
		numPop:     1,
		numPush:    0,
	}

	// SELFDESTRUCT is replaces by SENDALL
	instructionSet[SELFDESTRUCT] = instructionSet[SENDALL]

	validateAndFillMaxStack(&instructionSet)
	return instructionSet
}

// newZkEVMForkID5InstructionSet returns the instruction set for the forkID5
func newZkEVMForkID5InstructionSet() JumpTable {
	instructionSet := newZkEVMForkID4InstructionSet()

	instructionSet[CALLDATACOPY].execute = opCallDataCopy
	instructionSet[CALLDATALOAD].execute = opCallDataLoad
	instructionSet[STATICCALL].execute = opStaticCall

	enable3855(&instructionSet) // EIP-3855: Enable PUSH0 opcode

	validateAndFillMaxStack(&instructionSet)
	return instructionSet
}

// newZkEVMForkID7InstructionSet returns the instruction set for the forkID7
func newZkEVMForkID7InstructionSet() JumpTable {
	instructionSet := newZkEVMForkID5InstructionSet()
	// TODO add the new instructions for fork7

	return instructionSet
}
