package vm

import (
	"math"

	zk_consts "github.com/ledgerwatch/erigon/zk/constants"
)

const stepDeduction = 200
const safetyPercentage float64 = 0.05

var (
	defaultTotalSteps  = 1 << 23
	forkId10TotalSteps = 1 << 24
	forkId11TotalSteps = 1 << 25

	unlimitedCounters = counterLimits{
		totalSteps: math.MaxInt32,
		arith:      math.MaxInt32,
		binary:     math.MaxInt32,
		memAlign:   math.MaxInt32,
		keccaks:    math.MaxInt32,
		padding:    math.MaxInt32,
		poseidon:   math.MaxInt32,
		sha256:     math.MaxInt32,
	}
)

type counterLimits struct {
	totalSteps, arith, binary, memAlign, keccaks, padding, poseidon, sha256 int
}

func createCountrsByLimits(c counterLimits) *Counters {
	return &Counters{
		S: {
			remaining:     c.totalSteps,
			name:          "defaultTotalSteps",
			initialAmount: c.totalSteps,
		},
		A: {
			remaining:     c.arith,
			name:          "arith",
			initialAmount: c.arith,
		},
		B: {
			remaining:     c.binary,
			name:          "binary",
			initialAmount: c.binary,
		},
		M: {
			remaining:     c.memAlign,
			name:          "memAlign",
			initialAmount: c.memAlign,
		},
		K: {
			remaining:     c.keccaks,
			name:          "keccaks",
			initialAmount: c.keccaks,
		},
		D: {
			remaining:     c.padding,
			name:          "padding",
			initialAmount: c.padding,
		},
		P: {
			remaining:     c.poseidon,
			name:          "poseidon",
			initialAmount: c.poseidon,
		},
		SHA: {
			remaining:     c.sha256,
			name:          "sha256",
			initialAmount: c.sha256,
		},
	}
}

// tp ne used on next forkid counters
func getCounterLimits(forkId uint16) *Counters {
	totalSteps := getTotalSteps(forkId)

	counterLimits := counterLimits{
		totalSteps: applyDeduction(totalSteps, safetyPercentage),
		arith:      applyDeduction(totalSteps>>5, safetyPercentage),
		binary:     applyDeduction(totalSteps>>4, safetyPercentage),
		memAlign:   applyDeduction(totalSteps>>5, safetyPercentage),
		keccaks:    applyDeduction(int(math.Floor(float64(totalSteps)/155286)*44), safetyPercentage),
		padding:    applyDeduction(int(math.Floor(float64(totalSteps)/56)), safetyPercentage),
		poseidon:   applyDeduction(int(math.Floor(float64(totalSteps)/30)), safetyPercentage),
		sha256:     applyDeduction(int(math.Floor(float64(totalSteps-1)/31488))*7, safetyPercentage),
	}

	return createCountrsByLimits(counterLimits)
}

func getTotalSteps(forkId uint16) int {
	var totalSteps int

	switch forkId {
	case uint16(zk_consts.ForkID10):
		totalSteps = forkId10TotalSteps
	case uint16(zk_consts.ForkID11):
		totalSteps = forkId11TotalSteps
	default:
		totalSteps = defaultTotalSteps
	}

	// we need to remove some steps as these will always be used during batch execution
	totalSteps -= stepDeduction

	return totalSteps
}

func applyDeduction(input int, deduction float64) int {
	asFloat := float64(input)
	reduction := asFloat * deduction
	newValue := asFloat - reduction
	return int(math.Ceil(newValue))
}
