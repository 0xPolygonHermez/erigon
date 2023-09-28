package types

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gotest.tools/v3/assert"
)

func TestL2BlockDecode(t *testing.T) {
	type testCase struct {
		name           string
		input          []byte
		expectedResult L2Block
		expectedError  error
	}
	testCases := []testCase{
		{
			name: "Happy path",
			input: L2Block{
				BatchNumber:    101,
				L2BlockNumber:  1337,
				Timestamp:      124124124124,
				GlobalExitRoot: [32]byte{10, 11, 12, 13, 14, 15, 16, 17, 10, 11, 12, 13, 14, 15, 16, 17, 10, 11, 12, 13, 14, 15, 16, 17, 10, 11, 12, 13, 14, 15, 16, 17},
				Coinbase:       [20]byte{20, 21, 22, 23, 24, 20, 21, 22, 23, 24, 20, 21, 22, 23, 24, 20, 21, 22, 23, 24},
			}.Encode(),
			expectedResult: L2Block{
				BatchNumber:    101,
				L2BlockNumber:  1337,
				Timestamp:      124124124124,
				GlobalExitRoot: [32]byte{10, 11, 12, 13, 14, 15, 16, 17, 10, 11, 12, 13, 14, 15, 16, 17, 10, 11, 12, 13, 14, 15, 16, 17, 10, 11, 12, 13, 14, 15, 16, 17},
				Coinbase:       [20]byte{20, 21, 22, 23, 24, 20, 21, 22, 23, 24, 20, 21, 22, 23, 24, 20, 21, 22, 23, 24},
			},
			expectedError: nil,
		},
		{
			name:           "Invalid byte array length",
			input:          []byte{20, 21, 22, 23, 24, 20, 21, 22, 23, 24, 20, 21, 22, 23, 24, 20, 21, 22, 23, 24},
			expectedResult: L2Block{},
			expectedError:  fmt.Errorf("expected data length: 76, got: 20"),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			decodedL2Block, err := DecodeL2Block(testCase.input)
			require.Equal(t, testCase.expectedError, err)
			assert.Equal(t, testCase.expectedResult, *decodedL2Block)
		})
	}
}
