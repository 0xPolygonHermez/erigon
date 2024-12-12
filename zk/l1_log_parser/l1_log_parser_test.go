package l1_log_parser_test

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ledgerwatch/erigon-lib/common"
	ethTypes "github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/zk/contracts"
	"github.com/ledgerwatch/erigon/zk/l1_log_parser"
	"github.com/ledgerwatch/erigon/zk/types"
	"github.com/stretchr/testify/require"
)

type MockHermezDb struct {
	WriteSequenceFunc        func(l1BlockNo uint64, batchNo uint64, l1TxHash common.Hash, stateRoot common.Hash, l1InfoRoot common.Hash) error
	WriteVerificationFunc    func(l1BlockNo uint64, batchNo uint64, l1TxHash common.Hash, stateRoot common.Hash) error
	WriteRollupTypeFunc      func(rollupType uint64, forkId uint64) error
	WriteNewForkHistoryFunc  func(forkId uint64, latestVerified uint64) error
	RollbackSequencesFunc    func(batchNo uint64) error
	WriteL1InjectedBatchFunc func(batch *types.L1InjectedBatch) error
}

func (m *MockHermezDb) WriteSequence(l1BlockNo uint64, batchNo uint64, l1TxHash common.Hash, stateRoot common.Hash, l1InfoRoot common.Hash) error {
	if m.WriteSequenceFunc != nil {
		return m.WriteSequenceFunc(l1BlockNo, batchNo, l1TxHash, stateRoot, l1InfoRoot)
	}
	return nil
}

func (m *MockHermezDb) WriteVerification(l1BlockNo uint64, batchNo uint64, l1TxHash common.Hash, stateRoot common.Hash) error {
	if m.WriteVerificationFunc != nil {
		return m.WriteVerificationFunc(l1BlockNo, batchNo, l1TxHash, stateRoot)
	}
	return nil
}

func (m *MockHermezDb) WriteRollupType(rollupType uint64, forkId uint64) error {
	if m.WriteRollupTypeFunc != nil {
		return m.WriteRollupTypeFunc(rollupType, forkId)
	}
	return nil
}

func (m *MockHermezDb) WriteNewForkHistory(forkId uint64, latestVerified uint64) error {
	if m.WriteNewForkHistoryFunc != nil {
		return m.WriteNewForkHistoryFunc(forkId, latestVerified)
	}
	return nil
}

func (m *MockHermezDb) RollbackSequences(batchNo uint64) error {
	if m.RollbackSequencesFunc != nil {
		return m.RollbackSequencesFunc(batchNo)
	}
	return nil
}

func (m *MockHermezDb) WriteL1InjectedBatch(batch *types.L1InjectedBatch) error {
	if m.WriteL1InjectedBatchFunc != nil {
		return m.WriteL1InjectedBatchFunc(batch)
	}
	return nil
}

// MockIL1Syncer is a simple mock of the IL1Syncer interface
type MockIL1Syncer struct {
	GetHeaderFunc func(blockNumber uint64) (*ethTypes.Header, error)
}

func (m *MockIL1Syncer) GetHeader(blockNumber uint64) (*ethTypes.Header, error) {
	if m.GetHeaderFunc != nil {
		return m.GetHeaderFunc(blockNumber)
	}
	return nil, errors.New("not implemented")
}

func TestParseAndHandleLog(t *testing.T) {
	const l1RollupId = uint64(1)

	type testCase struct {
		log               *ethTypes.Log
		expectedError     error
		hermezDbMockSetup func(db *MockHermezDb)
		l1SyncerMockSetup func(syncer *MockIL1Syncer)
		syncMeta          *l1_log_parser.L1SyncMeta
		expectedSyncMeta  *l1_log_parser.L1SyncMeta
	}

	testCases := map[string]testCase{
		"SequencedBatchTopicPreEtrog": {
			log: &ethTypes.Log{
				BlockNumber: 100,
				Topics: []common.Hash{
					contracts.SequencedBatchTopicPreEtrog,
					common.BigToHash(big.NewInt(1)),
				},
				TxHash: common.HexToHash("0x1"),
				Data:   []byte{},
			},
			expectedError: nil,
			hermezDbMockSetup: func(db *MockHermezDb) {
				db.WriteSequenceFunc = func(l1BlockNo uint64, batchNo uint64, l1TxHash common.Hash, stateRoot common.Hash, l1InfoRoot common.Hash) error {
					require.Equal(t, uint64(100), l1BlockNo)
					require.Equal(t, uint64(1), batchNo)
					require.Equal(t, common.HexToHash("0x1"), l1TxHash)
					require.Equal(t, common.Hash{}, stateRoot)
					require.Equal(t, common.Hash{}, l1InfoRoot)
					return nil
				}
			},
			syncMeta: &l1_log_parser.L1SyncMeta{
				HighestWrittenL1BlockNo: 0,
				NewSequencesCount:       0,
			},
			expectedSyncMeta: &l1_log_parser.L1SyncMeta{
				HighestWrittenL1BlockNo: 100,
				NewSequencesCount:       1,
			},
		},
		"SequencedBatchTopicEtrog": {
			log: &ethTypes.Log{
				BlockNumber: 101,
				Topics: []common.Hash{
					contracts.SequencedBatchTopicEtrog,
					common.BigToHash(big.NewInt(2)),
				},
				TxHash: common.HexToHash("0x2"),
				Data:   common.HexToHash("0x3").Bytes(),
			},
			expectedError: nil,
			hermezDbMockSetup: func(db *MockHermezDb) {
				db.WriteSequenceFunc = func(l1BlockNo uint64, batchNo uint64, l1TxHash common.Hash, stateRoot common.Hash, l1InfoRoot common.Hash) error {
					require.Equal(t, uint64(101), l1BlockNo)
					require.Equal(t, uint64(2), batchNo)
					require.Equal(t, common.HexToHash("0x2"), l1TxHash)
					require.Equal(t, common.Hash{}, stateRoot)
					require.Equal(t, common.HexToHash("0x3"), l1InfoRoot)
					return nil
				}
			},
			syncMeta: &l1_log_parser.L1SyncMeta{
				HighestWrittenL1BlockNo: 0,
				NewSequencesCount:       0,
			},
			expectedSyncMeta: &l1_log_parser.L1SyncMeta{
				HighestWrittenL1BlockNo: 101,
				NewSequencesCount:       1,
			},
		},
		"VerificationTopicPreEtrog": {
			log: &ethTypes.Log{
				BlockNumber: 102,
				Topics: []common.Hash{
					contracts.VerificationTopicPreEtrog,
					common.BigToHash(big.NewInt(3)),
				},
				TxHash: common.HexToHash("0x4"),
				Data:   common.HexToHash("0x5").Bytes(),
			},
			expectedError: nil,
			hermezDbMockSetup: func(db *MockHermezDb) {
				db.WriteVerificationFunc = func(l1BlockNo uint64, batchNo uint64, l1TxHash common.Hash, stateRoot common.Hash) error {
					require.Equal(t, uint64(102), l1BlockNo)
					require.Equal(t, uint64(3), batchNo)
					require.Equal(t, common.HexToHash("0x4"), l1TxHash)
					require.Equal(t, common.HexToHash("0x5"), stateRoot)
					return nil
				}
			},
			syncMeta: &l1_log_parser.L1SyncMeta{
				HighestWrittenL1BlockNo: 0,
				NewVerificationsCount:   0,
			},
			expectedSyncMeta: &l1_log_parser.L1SyncMeta{
				HighestWrittenL1BlockNo: 102,
				NewVerificationsCount:   1,
				HighestVerification: types.BatchVerificationInfo{
					BaseBatchInfo: types.BaseBatchInfo{
						BatchNo:   3,
						L1BlockNo: 102,
						L1TxHash:  common.HexToHash("0x4"),
					},
					StateRoot: common.HexToHash("0x5"),
				},
			},
		},
		"VerificationValidiumTopicEtrog": {
			log: &ethTypes.Log{
				BlockNumber: 103,
				Topics: []common.Hash{
					contracts.VerificationValidiumTopicEtrog,
					common.BigToHash(big.NewInt(4)),
				},
				TxHash: common.HexToHash("0x6"),
				Data:   common.HexToHash("0x7").Bytes(),
			},
			expectedError: nil,
			hermezDbMockSetup: func(db *MockHermezDb) {
				db.WriteVerificationFunc = func(l1BlockNo uint64, batchNo uint64, l1TxHash common.Hash, stateRoot common.Hash) error {
					require.Equal(t, uint64(103), l1BlockNo)
					require.Equal(t, uint64(4), batchNo)
					require.Equal(t, common.HexToHash("0x6"), l1TxHash)
					require.Equal(t, common.HexToHash("0x7"), stateRoot)
					return nil
				}
			},
			syncMeta: &l1_log_parser.L1SyncMeta{
				HighestWrittenL1BlockNo: 0,
				NewVerificationsCount:   0,
			},
			expectedSyncMeta: &l1_log_parser.L1SyncMeta{
				HighestWrittenL1BlockNo: 103,
				NewVerificationsCount:   1,
				HighestVerification: types.BatchVerificationInfo{
					BaseBatchInfo: types.BaseBatchInfo{
						BatchNo:   4,
						L1BlockNo: 103,
						L1TxHash:  common.HexToHash("0x6"),
					},
					StateRoot: common.HexToHash("0x7"),
				},
			},
		},
		"RollbackBatchesTopic": {
			log: &ethTypes.Log{
				BlockNumber: 104,
				Topics: []common.Hash{
					contracts.RollbackBatchesTopic,
					common.BigToHash(big.NewInt(20)),
				},
				TxHash: common.HexToHash("0x888"),
			},
			expectedError: nil,
			hermezDbMockSetup: func(db *MockHermezDb) {
				db.RollbackSequencesFunc = func(batchNo uint64) error {
					require.Equal(t, uint64(20), batchNo)
					return nil
				}
			},
			syncMeta: &l1_log_parser.L1SyncMeta{
				HighestWrittenL1BlockNo: 0,
			},
			expectedSyncMeta: &l1_log_parser.L1SyncMeta{
				HighestWrittenL1BlockNo: 0, // we should rollback!
			},
		},
	}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			hermezDbMock := &MockHermezDb{}
			l1SyncerMock := &MockIL1Syncer{}

			if tc.hermezDbMockSetup != nil {
				tc.hermezDbMockSetup(hermezDbMock)
			}

			if tc.l1SyncerMockSetup != nil {
				tc.l1SyncerMockSetup(l1SyncerMock)
			}

			parser := l1_log_parser.NewL1LogParser(l1SyncerMock, hermezDbMock, l1RollupId)

			syncMeta := tc.syncMeta
			if syncMeta == nil {
				syncMeta = &l1_log_parser.L1SyncMeta{}
			}

			resultSyncMeta, err := parser.ParseAndHandleLog(tc.log, syncMeta)

			if tc.expectedError != nil {
				require.ErrorIs(t, err, tc.expectedError)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tc.expectedSyncMeta, resultSyncMeta)
		})
	}
}
