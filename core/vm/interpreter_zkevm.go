package vm

import (
	"github.com/ledgerwatch/erigon/zk/sequencer"
	"github.com/ledgerwatch/log/v3"
)

// Config are the configuration options for the Interpreter
type Config struct {
	Debug         bool      // Enables debugging
	Tracer        EVMLogger // Opcode logger
	NoRecursion   bool      // Disables call, callcode, delegate call and create
	NoBaseFee     bool      // Forces the EIP-1559 baseFee to 0 (needed for 0 price calls)
	SkipAnalysis  bool      // Whether we can skip jumpdest analysis based on the checked history
	TraceJumpDest bool      // Print transaction hashes where jumpdest analysis was useful
	NoReceipts    bool      // Do not calculate receipts
	ReadOnly      bool      // Do no perform any block finalisation
	StatelessExec bool      // true is certain conditions (like state trie root hash matching) need to be relaxed for stateless EVM execution
	RestoreState  bool      // Revert all changes made to the state (useful for constant system calls)

	ExtraEips []int // Additional EIPS that are to be enabled

	// zkevm
	CounterCollector *CounterCollector
}

// NewZKEVMInterpreter returns a new instance of the Interpreter.
func NewZKEVMInterpreter(evm VMInterpreter, cfg Config) *EVMInterpreter {
	var jt *JumpTable
	switch {
	// to add our own IsRohan chain rule, we would need to fork or code or chain.Config
	// that is why we hard code it here for POC
	// our fork extends berlin anyways and starts from block 1
	case evm.ChainRules().IsMordor:
		jt = &zkevmForkID5InstructionSet
	case evm.ChainRules().IsBerlin:
		jt = &zkevmForkID4InstructionSet
	}
	jt = copyJumpTable(jt)
	if len(cfg.ExtraEips) > 0 {
		for i, eip := range cfg.ExtraEips {
			if err := EnableEIP(eip, jt); err != nil {
				// Disable it, so caller can check if it's activated or not
				cfg.ExtraEips = append(cfg.ExtraEips[:i], cfg.ExtraEips[i+1:]...)
				log.Error("EIP activation failed", "eip", eip, "err", err)
			}
		}
	}

	// TODOSEQ - replace counter manager with a transaction counter collector
	if sequencer.IsSequencer() {
		if cfg.CounterCollector == nil {
			cfg.CounterCollector = NewCounterCollector()
		}

		WrapJumpTableWithZkCounters(jt, SimpleCounterOperations(cfg.CounterCollector))
	}

	return &EVMInterpreter{
		VM: &VM{
			evm: evm,
			cfg: cfg,
		},
		jt: jt,
	}
}
