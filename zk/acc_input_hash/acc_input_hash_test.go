package acc_input_hash

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon-lib/kv/mdbx"
	eritypes "github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/zk/hermez_db"
	"github.com/ledgerwatch/erigon/zk/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type MockBlockReader struct {
	blocks map[uint64]*eritypes.Block
}

func (m *MockBlockReader) ReadBlockByNumber(blockNo uint64) (*eritypes.Block, error) {
	block, exists := m.blocks[blockNo]
	if !exists {
		return nil, fmt.Errorf("block %d not found", blockNo)
	}
	return block, nil
}

type MockAccInputHashReader struct {
	AccInputHashes               map[uint64]common.Hash
	L2BlockNosByBatch            map[uint64][]uint64
	ForkIds                      map[uint64]uint64
	BlockGlobalExitRoots         map[uint64]common.Hash
	L1InfoTreeUpdatesByGer       map[common.Hash]*types.L1InfoTreeUpdate
	L1InfoTreeIndexToRoots       map[uint64]common.Hash
	EffectiveGasPricePercentages map[common.Hash]uint8
}

func (m *MockAccInputHashReader) GetAccInputHashForBatchOrPrevious(batchNo uint64) (common.Hash, uint64, error) {
	for i := batchNo; i >= 0; i-- {
		if hash, ok := m.AccInputHashes[i]; ok {
			return hash, i, nil
		}
		if i == 0 {
			break
		}
	}
	return common.Hash{}, 0, nil
}

func (m *MockAccInputHashReader) GetBlockL1InfoTreeIndex(blockNo uint64) (uint64, error) {
	return 1, nil
}

func (m *MockAccInputHashReader) GetEffectiveGasPricePercentage(txHash common.Hash) (uint8, error) {
	return 100, nil
}

func (m *MockAccInputHashReader) GetL2BlockNosByBatch(batchNo uint64) ([]uint64, error) {
	if blockNos, ok := m.L2BlockNosByBatch[batchNo]; ok {
		return blockNos, nil
	}
	return nil, fmt.Errorf("L2 block numbers not found for batch %d", batchNo)
}

func (m *MockAccInputHashReader) GetForkId(batchNo uint64) (uint64, error) {
	if forkId, ok := m.ForkIds[batchNo]; ok {
		return forkId, nil
	}
	return 0, fmt.Errorf("fork ID not found for batch %d", batchNo)
}

func (m *MockAccInputHashReader) GetBlockGlobalExitRoot(l2BlockNo uint64) (common.Hash, error) {
	if ger, ok := m.BlockGlobalExitRoots[l2BlockNo]; ok {
		return ger, nil
	}
	return common.Hash{}, nil
}

func (m *MockAccInputHashReader) GetL1InfoTreeUpdateByGer(ger common.Hash) (*types.L1InfoTreeUpdate, error) {
	if update, ok := m.L1InfoTreeUpdatesByGer[ger]; ok {
		return update, nil
	}
	return nil, nil
}

func (m *MockAccInputHashReader) GetL1InfoTreeIndexToRoots() (map[uint64]common.Hash, error) {
	if len(m.L1InfoTreeIndexToRoots) == 0 {
		return nil, fmt.Errorf("no L1 info tree indexes found")
	}
	return m.L1InfoTreeIndexToRoots, nil
}

func GetDbTx(ctx context.Context) (tx kv.RwTx, cleanup func()) {
	dbi, err := mdbx.NewTemporaryMdbx(ctx, "")
	if err != nil {
		panic(err)
	}
	tx, err = dbi.BeginRw(ctx)
	if err != nil {
		panic(err)
	}

	err = hermez_db.CreateHermezBuckets(tx)
	if err != nil {
		panic(err)
	}

	return tx, func() {
		tx.Rollback()
		dbi.Close()
	}
}

func createMockBlock(blockNo uint64) *eritypes.Block {
	header := &eritypes.Header{
		Number:   big.NewInt(int64(blockNo)),
		Time:     1234567890 + blockNo,
		Coinbase: common.HexToAddress("0x000000000000000000000000000000000000ba3e"),
	}

	txs := make(eritypes.Transactions, 0)
	return eritypes.NewBlock(header, txs, nil, nil, nil)
}

func TestCalculateAccInputHash(t *testing.T) {
	ctx := context.Background()

	testCases := map[string]struct {
		forkID           uint64
		batchNum         uint64
		expectedHash     common.Hash
		expectError      bool
		expectedErrorMsg string
		setup            func(*testing.T) (*MockAccInputHashReader, *MockBlockReader)
	}{
		"Valid Fork7, Existing Batch": {
			forkID:       7,
			batchNum:     4,
			expectedHash: common.HexToHash("0xbbb"),
			expectError:  false,
			setup: func(t *testing.T) (*MockAccInputHashReader, *MockBlockReader) {
				reader := &MockAccInputHashReader{
					ForkIds: map[uint64]uint64{
						4: 7,
					},
					AccInputHashes: map[uint64]common.Hash{
						4: common.HexToHash("0xbbb"),
					},
				}
				return reader, nil
			},
		},
		"Valid Fork7 Missing Batch Calculate Hash": {
			forkID:       7,
			batchNum:     5,
			expectedHash: common.HexToHash("0xb370e69e462a8a00469cb0ce188399a9754880dfd8ebd98717e24cbe1103efa6"),
			expectError:  false,
			setup: func(t *testing.T) (*MockAccInputHashReader, *MockBlockReader) {
				reader := &MockAccInputHashReader{
					AccInputHashes: map[uint64]common.Hash{
						4: common.HexToHash("0xbbb"),
					},
					ForkIds: map[uint64]uint64{
						4: 7,
						5: 7,
					},
					L2BlockNosByBatch: map[uint64][]uint64{
						3: {48},
						4: {49, 50},
						5: {51, 52},
					},
					BlockGlobalExitRoots: map[uint64]common.Hash{
						50: common.HexToHash("0x1234"),
						51: common.HexToHash("0x5678"),
						52: common.HexToHash("0x9abc"),
					},
					L1InfoTreeUpdatesByGer: map[common.Hash]*types.L1InfoTreeUpdate{
						common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000009abc"): {
							Index: 0,
						},
						common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000001234"): {
							Index: 1,
						},
					},
					L1InfoTreeIndexToRoots: map[uint64]common.Hash{
						0: common.HexToHash("0x9abc"),
					},
				}
				blocks := map[uint64]*eritypes.Block{
					48: createMockBlock(48),
					49: createMockBlock(49),
					50: createMockBlock(50),
					51: createMockBlock(51),
					52: createMockBlock(52),
				}
				mockBlockReader := &MockBlockReader{
					blocks: blocks,
				}
				return reader, mockBlockReader
			},
		},
		"Valid Fork7, No Previous Batch": {
			forkID:       7,
			batchNum:     2,
			expectedHash: common.HexToHash("0x0cd77f88e7eeeef006fa44caaf24baab7a1b46321e26a9fa28f943a293a8811e"),
			expectError:  false,
			setup: func(t *testing.T) (*MockAccInputHashReader, *MockBlockReader) {
				reader := &MockAccInputHashReader{
					AccInputHashes: map[uint64]common.Hash{},
					ForkIds: map[uint64]uint64{
						0: 7,
						1: 7,
						2: 7,
					},
					L2BlockNosByBatch: map[uint64][]uint64{
						1: {1},
						2: {2, 3},
					},
					L1InfoTreeIndexToRoots: map[uint64]common.Hash{
						0: common.HexToHash("0x9abc"),
					},
				}
				blocks := map[uint64]*eritypes.Block{
					0: createMockBlock(0),
					1: createMockBlock(1),
					2: createMockBlock(2),
					3: createMockBlock(3),
				}
				mockBlockReader := &MockBlockReader{
					blocks: blocks,
				}
				return reader, mockBlockReader
			},
		},
		"Invalid Fork ID": {
			forkID:           1000,
			batchNum:         5,
			expectError:      true,
			expectedErrorMsg: "unsupported fork ID: 1000",
			setup: func(t *testing.T) (*MockAccInputHashReader, *MockBlockReader) {
				return &MockAccInputHashReader{}, nil
			},
		},
		"Unsupported PreFork7": {
			forkID:           6,
			batchNum:         5,
			expectError:      true,
			expectedErrorMsg: "unsupported fork ID: 6",
			setup: func(t *testing.T) (*MockAccInputHashReader, *MockBlockReader) {
				return &MockAccInputHashReader{}, nil
			},
		},
		"Cross Boundary Between Fork7 and Fork6": {
			forkID:           7,
			batchNum:         3,
			expectError:      true,
			expectedErrorMsg: "unsupported fork ID: 6",
			setup: func(t *testing.T) (*MockAccInputHashReader, *MockBlockReader) {
				reader := &MockAccInputHashReader{
					AccInputHashes: map[uint64]common.Hash{
						2: common.HexToHash("0xbbb"),
					},
					ForkIds: map[uint64]uint64{
						2: 6,
						3: 7,
					},
				}
				return reader, nil
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			tx, cleanup := GetDbTx(ctx)
			reader, mockBlockReader := tc.setup(t)
			var calculator AccInputHashCalculator
			var err error
			if mockBlockReader != nil {
				calculator, err = NewCalculatorWithBlockReader(ctx, tx, reader, mockBlockReader, tc.forkID)
			} else {
				calculator, err = NewCalculator(ctx, tx, reader, tc.forkID)
			}
			if err != nil {
				if tc.expectError {
					assert.EqualError(t, err, tc.expectedErrorMsg)
				} else {
					require.NoError(t, err)
				}
				return
			}

			hash, err := calculator.Calculate(tc.batchNum)
			if tc.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrorMsg)
				assert.Equal(t, common.Hash{}, hash)
			} else {
				t.Log(hash.Hex())
				require.NoError(t, err)
				if tc.expectedHash != (common.Hash{}) {
					assert.Equal(t, tc.expectedHash, hash, "Hash mismatch for batch number %d", tc.batchNum)
				} else {
					assert.NotEqual(t, common.Hash{}, hash, "Hash should not be empty for batch number %d", tc.batchNum)
				}
			}
			cleanup()
		})
	}
}
