package acc_input_hash

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalculateAccInputHash(t *testing.T) {
	ctx := context.Background()
	batchNum := uint64(123)

	testCases := map[string]struct {
		forkID         int
		validium       bool
		expectError    bool
		expectedErrMsg string
	}{
		"Valid Validium Scenario PreFork7": {
			forkID:      1,
			validium:    true,
			expectError: false,
		},
		"Valid Validium Scenario Fork7": {
			forkID:      7,
			validium:    true,
			expectError: false,
		},
		"Invalid Validium Scenario": {
			forkID:         1000,
			validium:       true,
			expectError:    true,
			expectedErrMsg: "unsupported fork ID: 1000",
		},
		"Valid Scenario PreFork7": {
			forkID:      6,
			validium:    false,
			expectError: false,
		},
		"Valid Scenario Fork7": {
			forkID:      7,
			validium:    false,
			expectError: false,
		},
		"Invalid Scenario": {
			forkID:         1000,
			validium:       false,
			expectError:    true,
			expectedErrMsg: "unsupported fork ID: 1000",
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			hash, err := CalculateAccInputHash(ctx, tc.forkID, batchNum, tc.validium)
			if tc.expectError {
				assert.Error(t, err)
				assert.Nil(t, hash)
				assert.EqualError(t, err, tc.expectedErrMsg)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, hash)
			}
		})
	}
}

func TestCalculators(t *testing.T) {
	ctx := context.Background()
	batchNum := uint64(123)
	validium := true

	testCases := map[string]struct {
		calculator AccInputHashCalculator
	}{
		"PreFork7Calculator": {
			calculator: PreFork7Calculator{},
		},
		"Fork7Calculator": {
			calculator: Fork7Calculator{},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			hash, err := tc.calculator.Calculate(ctx, batchNum, validium)
			assert.NoError(t, err)
			assert.NotNil(t, hash)
		})
	}
}
