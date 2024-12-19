package stages

// TODO: take from zkevm branch and modify

// import (
// 	"context"
// 	"math/big"
// 	"testing"
// 	"time"

// 	ethereum "github.com/ledgerwatch/erigon"
// 	"github.com/ledgerwatch/erigon-lib/common"
// 	"github.com/ledgerwatch/erigon-lib/kv/memdb"
// 	"github.com/ledgerwatch/erigon/core/rawdb"
// 	"github.com/ledgerwatch/erigon/core/types"
// 	"github.com/ledgerwatch/erigon/eth/ethconfig"
// 	"github.com/ledgerwatch/erigon/eth/stagedsync"
// 	"github.com/ledgerwatch/erigon/eth/stagedsync/stages"
// 	"github.com/ledgerwatch/erigon/smt/pkg/db"
// 	"github.com/ledgerwatch/erigon/zk/contracts"
// 	"github.com/ledgerwatch/erigon/zk/hermez_db"
// 	"github.com/ledgerwatch/erigon/zk/syncer"
// 	"github.com/ledgerwatch/erigon/zk/syncer/mocks"
// 	"github.com/stretchr/testify/require"
// 	"go.uber.org/mock/gomock"
// )

// func TestSpawnStageL1Syncer(t *testing.T) {
// 	const rollupID = uint64(1)

// 	type testCase struct {
// 		getLogs func(hDB *hermez_db.HermezDb, latestBlockNumber *big.Int, l1ContractAddresses []common.Address) ([]types.Log, error)
// 		assert  func(t *testing.T, hDB *hermez_db.HermezDb, latestBlockNumber *big.Int)
// 	}

// 	testCases := map[string]testCase{
// 		"SequencedBatchTopicPreEtrog": {
// 			getLogs: func(hDB *hermez_db.HermezDb, latestBlockNumber *big.Int, l1ContractAddresses []common.Address) ([]types.Log, error) {
// 				batchNum := uint64(1)
// 				batchNumHash := common.BytesToHash(big.NewInt(0).SetUint64(batchNum).Bytes())
// 				txHash := common.HexToHash("0x1")
// 				log := types.Log{
// 					BlockNumber: latestBlockNumber.Uint64(),
// 					Address:     l1ContractAddresses[0],
// 					Topics:      []common.Hash{contracts.SequencedBatchTopicPreEtrog, batchNumHash},
// 					TxHash:      txHash,
// 					Data:        []byte{},
// 				}
// 				return []types.Log{log}, nil
// 			},
// 			assert: func(t *testing.T, hDB *hermez_db.HermezDb, latestBlockNumber *big.Int) {
// 				l1BatchInfo, err := hDB.GetSequenceByBatchNo(1)
// 				require.NoError(t, err)

// 				require.Equal(t, l1BatchInfo.BatchNo, uint64(1))
// 				require.Equal(t, l1BatchInfo.L1BlockNo, latestBlockNumber.Uint64())
// 				require.Equal(t, l1BatchInfo.L1TxHash.String(), common.HexToHash("0x1").String())
// 				require.Equal(t, l1BatchInfo.StateRoot.String(), common.Hash{}.String())
// 				require.Equal(t, l1BatchInfo.L1InfoRoot.String(), common.Hash{}.String())
// 			},
// 		},
// 		"SequencedBatchTopicEtrog": {
// 			getLogs: func(hDB *hermez_db.HermezDb, latestBlockNumber *big.Int, l1ContractAddresses []common.Address) ([]types.Log, error) {
// 				batchNum := uint64(2)
// 				batchNumHash := common.BytesToHash(big.NewInt(0).SetUint64(batchNum).Bytes())
// 				txHash := common.HexToHash("0x2")
// 				l1InfoRoot := common.HexToHash("0x3")
// 				log := types.Log{
// 					BlockNumber: latestBlockNumber.Uint64(),
// 					Address:     l1ContractAddresses[0],
// 					Topics:      []common.Hash{contracts.SequencedBatchTopicEtrog, batchNumHash},
// 					Data:        l1InfoRoot.Bytes(),
// 					TxHash:      txHash,
// 				}
// 				return []types.Log{log}, nil
// 			},
// 			assert: func(t *testing.T, hDB *hermez_db.HermezDb, latestBlockNumber *big.Int) {
// 				l1BatchInfo, err := hDB.GetSequenceByBatchNo(2)
// 				require.NoError(t, err)

// 				require.Equal(t, uint64(2), l1BatchInfo.BatchNo)
// 				require.Equal(t, latestBlockNumber.Uint64(), l1BatchInfo.L1BlockNo)
// 				require.Equal(t, common.HexToHash("0x2").String(), l1BatchInfo.L1TxHash.String())
// 				require.Equal(t, common.Hash{}.String(), l1BatchInfo.StateRoot.String())
// 				require.Equal(t, common.HexToHash("0x3").String(), l1BatchInfo.L1InfoRoot.String())
// 			},
// 		},
// 		"VerificationTopicPreEtrog": {
// 			getLogs: func(hDB *hermez_db.HermezDb, latestBlockNumber *big.Int, l1ContractAddresses []common.Address) ([]types.Log, error) {
// 				batchNum := uint64(3)
// 				batchNumHash := common.BytesToHash(big.NewInt(0).SetUint64(batchNum).Bytes())
// 				txHash := common.HexToHash("0x4")
// 				stateRoot := common.HexToHash("0x5")
// 				log := types.Log{
// 					BlockNumber: latestBlockNumber.Uint64(),
// 					Address:     l1ContractAddresses[0],
// 					Topics:      []common.Hash{contracts.VerificationTopicPreEtrog, batchNumHash},
// 					Data:        stateRoot.Bytes(),
// 					TxHash:      txHash,
// 				}
// 				return []types.Log{log}, nil
// 			},
// 			assert: func(t *testing.T, hDB *hermez_db.HermezDb, latestBlockNumber *big.Int) {
// 				l1BatchInfo, err := hDB.GetVerificationByBatchNo(3)
// 				require.NoError(t, err)

// 				require.Equal(t, l1BatchInfo.BatchNo, uint64(3))
// 				require.Equal(t, l1BatchInfo.L1BlockNo, latestBlockNumber.Uint64())
// 				require.Equal(t, l1BatchInfo.L1TxHash.String(), common.HexToHash("0x4").String())
// 				require.Equal(t, l1BatchInfo.StateRoot.String(), common.HexToHash("0x5").String())
// 			},
// 		},
// 		"VerificationValidiumTopicEtrog": {
// 			getLogs: func(hDB *hermez_db.HermezDb, latestBlockNumber *big.Int, l1ContractAddresses []common.Address) ([]types.Log, error) {
// 				batchNum := uint64(4)
// 				batchNumHash := common.BytesToHash(big.NewInt(0).SetUint64(batchNum).Bytes())
// 				txHash := common.HexToHash("0x4")
// 				stateRoot := common.HexToHash("0x5")
// 				log := types.Log{
// 					BlockNumber: latestBlockNumber.Uint64(),
// 					Address:     l1ContractAddresses[0],
// 					Topics:      []common.Hash{contracts.VerificationValidiumTopicEtrog, batchNumHash},
// 					Data:        stateRoot.Bytes(),
// 					TxHash:      txHash,
// 				}
// 				return []types.Log{log}, nil
// 			},
// 			assert: func(t *testing.T, hDB *hermez_db.HermezDb, latestBlockNumber *big.Int) {
// 				l1BatchInfo, err := hDB.GetVerificationByBatchNo(4)
// 				require.NoError(t, err)

// 				require.Equal(t, l1BatchInfo.BatchNo, uint64(4))
// 				require.Equal(t, l1BatchInfo.L1BlockNo, latestBlockNumber.Uint64())
// 				require.Equal(t, l1BatchInfo.L1TxHash.String(), common.HexToHash("0x4").String())
// 				require.Equal(t, l1BatchInfo.StateRoot.String(), common.HexToHash("0x5").String())
// 			},
// 		},
// 		"VerificationTopicEtrog": {
// 			getLogs: func(hDB *hermez_db.HermezDb, latestBlockNumber *big.Int, l1ContractAddresses []common.Address) ([]types.Log, error) {
// 				rollupID := uint64(1)
// 				rollupIDHash := common.BytesToHash(big.NewInt(0).SetUint64(rollupID).Bytes())
// 				batchNum := uint64(5)
// 				batchNumHash := common.BytesToHash(big.NewInt(0).SetUint64(batchNum).Bytes())
// 				txHash := common.HexToHash("0x6")
// 				stateRoot := common.HexToHash("0x7")
// 				data := append(batchNumHash.Bytes(), stateRoot.Bytes()...)
// 				log := types.Log{
// 					BlockNumber: latestBlockNumber.Uint64(),
// 					Address:     l1ContractAddresses[0],
// 					Topics:      []common.Hash{contracts.VerificationTopicEtrog, rollupIDHash},
// 					Data:        data,
// 					TxHash:      txHash,
// 				}
// 				return []types.Log{log}, nil
// 			},
// 			assert: func(t *testing.T, hDB *hermez_db.HermezDb, latestBlockNumber *big.Int) {
// 				l1BatchInfo, err := hDB.GetVerificationByBatchNo(5)
// 				require.NoError(t, err)

// 				require.Equal(t, l1BatchInfo.BatchNo, uint64(5))
// 				require.Equal(t, l1BatchInfo.L1BlockNo, latestBlockNumber.Uint64())
// 				require.Equal(t, l1BatchInfo.L1TxHash.String(), common.HexToHash("0x6").String())
// 				require.Equal(t, l1BatchInfo.StateRoot.String(), common.HexToHash("0x7").String())
// 			},
// 		},
// 		"RollbackBatchesTopic": {
// 			getLogs: func(hDB *hermez_db.HermezDb, latestBlockNumber *big.Int, l1ContractAddresses []common.Address) ([]types.Log, error) {
// 				blockNum := uint64(10)
// 				batchNum := uint64(20)
// 				batchNumHash := common.BytesToHash(big.NewInt(0).SetUint64(batchNum).Bytes())
// 				txHash := common.HexToHash("0x888")
// 				stateRoot := common.HexToHash("0x999")
// 				l1InfoRoot := common.HexToHash("0x101010")

// 				// Prepopulate the database with sequences
// 				for i := uint64(15); i <= uint64(25); i++ {
// 					err := hDB.WriteSequence(blockNum, i, txHash, stateRoot, l1InfoRoot)
// 					require.NoError(t, err)
// 				}

// 				log := types.Log{
// 					BlockNumber: latestBlockNumber.Uint64(),
// 					Address:     l1ContractAddresses[0],
// 					Topics:      []common.Hash{contracts.RollbackBatchesTopic, batchNumHash},
// 					TxHash:      txHash,
// 				}
// 				return []types.Log{log}, nil
// 			},
// 			assert: func(t *testing.T, hDB *hermez_db.HermezDb, latestBlockNumber *big.Int) {
// 				// Batches up to batchNum (20) should exist; batches after should not
// 				for i := uint64(15); i <= uint64(20); i++ {
// 					l1BatchInfo, err := hDB.GetSequenceByBatchNo(i)
// 					require.NotNil(t, l1BatchInfo)
// 					require.NoError(t, err)
// 				}
// 				for i := uint64(21); i <= uint64(25); i++ {
// 					l1BatchInfo, err := hDB.GetSequenceByBatchNo(i)
// 					require.Nil(t, l1BatchInfo)
// 					require.NoError(t, err)
// 				}
// 			},
// 		},
// 	}

// 	for name, tc := range testCases {
// 		t.Run(name, func(t *testing.T) {
// 			// Arrange
// 			ctx, db1 := context.Background(), memdb.NewTestDB(t)
// 			tx := memdb.BeginRw(t, db1)
// 			err := hermez_db.CreateHermezBuckets(tx)
// 			require.NoError(t, err)
// 			err = db.CreateEriDbBuckets(tx)
// 			require.NoError(t, err)

// 			l1FirstBlock := big.NewInt(20)
// 			l2BlockNumber := uint64(10)
// 			verifiedBatchNumber := uint64(2)

// 			hDB := hermez_db.NewHermezDb(tx)
// 			err = hDB.WriteBlockBatch(0, 0)
// 			require.NoError(t, err)
// 			err = hDB.WriteBlockBatch(l2BlockNumber-1, verifiedBatchNumber-1)
// 			require.NoError(t, err)
// 			err = hDB.WriteBlockBatch(l2BlockNumber, verifiedBatchNumber)
// 			require.NoError(t, err)
// 			err = stages.SaveStageProgress(tx, stages.L1Syncer, 0)
// 			require.NoError(t, err)
// 			err = stages.SaveStageProgress(tx, stages.IntermediateHashes, l2BlockNumber-1)
// 			require.NoError(t, err)

// 			err = hDB.WriteVerification(l1FirstBlock.Uint64(), verifiedBatchNumber-1, common.HexToHash("0x1"), common.HexToHash("0x99990"))
// 			require.NoError(t, err)
// 			err = hDB.WriteVerification(l1FirstBlock.Uint64(), verifiedBatchNumber, common.HexToHash("0x2"), common.HexToHash("0x99999"))
// 			require.NoError(t, err)

// 			genesisHeader := &types.Header{
// 				Number:      big.NewInt(0).SetUint64(l2BlockNumber - 1),
// 				Time:        0,
// 				Difficulty:  big.NewInt(1),
// 				GasLimit:    8000000,
// 				GasUsed:     0,
// 				ParentHash:  common.HexToHash("0x1"),
// 				TxHash:      common.HexToHash("0x2"),
// 				ReceiptHash: common.HexToHash("0x3"),
// 				Root:        common.HexToHash("0x99990"),
// 			}

// 			txs := []types.Transaction{}
// 			uncles := []*types.Header{}
// 			receipts := []*types.Receipt{}
// 			withdrawals := []*types.Withdrawal{}

// testCases := []testCase{
// 	{
// 		name: "SequencedBatchTopicPreEtrog",
// 		getLog: func(hDB *hermez_db.HermezDb) (types.Log, error) {
// 			batchNum := uint64(1)
// 			batchNumHash := common.BytesToHash(big.NewInt(0).SetUint64(batchNum).Bytes())
// 			txHash := common.HexToHash("0x1")
// 			return types.Log{
// 				BlockNumber: latestBlockNumber.Uint64(),
// 				Address:     l1ContractAddresses[0],
// 				Topics:      []common.Hash{contracts.SequenceBatchesTopicPreEtrog, batchNumHash},
// 				TxHash:      txHash,
// 				Data:        []byte{},
// 			}, nil
// 		},
// 		assert: func(t *testing.T, hDB *hermez_db.HermezDb) {
// 			l1BatchInfo, err := hDB.GetSequenceByBatchNo(1)
// 			require.NoError(t, err)

// 			require.Equal(t, l1BatchInfo.BatchNo, uint64(1))
// 			require.Equal(t, l1BatchInfo.L1BlockNo, latestBlockNumber.Uint64())
// 			require.Equal(t, l1BatchInfo.L1TxHash.String(), common.HexToHash("0x1").String())
// 			require.Equal(t, l1BatchInfo.StateRoot.String(), common.Hash{}.String())
// 			require.Equal(t, l1BatchInfo.L1InfoRoot.String(), common.Hash{}.String())
// 		},
// 	},
// 	{
// 		name: "SequencedBatchTopicEtrog",
// 		getLog: func(hDB *hermez_db.HermezDb) (types.Log, error) {
// 			batchNum := uint64(2)
// 			batchNumHash := common.BytesToHash(big.NewInt(0).SetUint64(batchNum).Bytes())
// 			txHash := common.HexToHash("0x2")
// 			l1InfoRoot := common.HexToHash("0x3")
// 			return types.Log{
// 				BlockNumber: latestBlockNumber.Uint64(),
// 				Address:     l1ContractAddresses[0],
// 				Topics:      []common.Hash{contracts.SequenceBatchesTopicEtrog, batchNumHash},
// 				Data:        l1InfoRoot.Bytes(),
// 				TxHash:      txHash,
// 			}, nil
// 		},
// 		assert: func(t *testing.T, hDB *hermez_db.HermezDb) {
// 			l1BatchInfo, err := hDB.GetSequenceByBatchNo(2)
// 			require.NoError(t, err)

// 			s := &stagedsync.StageState{ID: stages.L1Syncer, BlockNumber: 0}
// 			u := &stagedsync.Sync{}

// 			mockCtrl := gomock.NewController(t)
// 			defer mockCtrl.Finish()
// 			EthermanMock := mocks.NewMockIEtherman(mockCtrl)

// 			l1ContractAddresses := []common.Address{
// 				common.HexToAddress("0x1"),
// 				common.HexToAddress("0x2"),
// 				common.HexToAddress("0x3"),
// 			}
// 			l1ContractTopics := [][]common.Hash{
// 				{common.HexToHash("0x1")},
// 				{common.HexToHash("0x2")},
// 				{common.HexToHash("0x3")},
// 			}

// 			latestBlockParentHash := common.HexToHash("0x123456789")
// 			latestBlockTime := uint64(time.Now().Unix())
// 			latestBlockNumber := big.NewInt(21)
// 			latestBlockHeader := &types.Header{ParentHash: latestBlockParentHash, Number: latestBlockNumber, Time: latestBlockTime}
// 			latestBlock := types.NewBlockWithHeader(latestBlockHeader)

// 			EthermanMock.EXPECT().BlockByNumber(gomock.Any(), nil).Return(latestBlock, nil).AnyTimes()
// 			EthermanMock.EXPECT().CallContract(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

// 			filterQuery := ethereum.FilterQuery{
// 				FromBlock: l1FirstBlock,
// 				ToBlock:   latestBlockNumber,
// 				Addresses: l1ContractAddresses,
// 				Topics:    l1ContractTopics,
// 			}

// 			// Prepare the logs for this test case
// 			filteredLogs, err := tc.getLogs(hDB, latestBlockNumber, l1ContractAddresses)
// 			require.NoError(t, err)

// 			EthermanMock.EXPECT().FilterLogs(gomock.Any(), filterQuery).Return(filteredLogs, nil).AnyTimes()

// 			l1Syncer := syncer.NewL1Syncer(ctx, []syncer.IEtherman{EthermanMock}, l1ContractAddresses, l1ContractTopics, 10, 0, "latest")

// 			zkCfg := &ethconfig.Zk{
// 				L1RollupId:   rollupID,
// 				L1FirstBlock: l1FirstBlock.Uint64(),
// 			}
// 			cfg := StageL1SyncerCfg(db1, l1Syncer, zkCfg)
// 			quiet := false

// 			// Act
// 			err = SpawnStageL1Syncer(s, u, ctx, tx, cfg, quiet)
// 			require.NoError(t, err)

// 			// Assert
// 			tc.assert(t, hDB, latestBlockNumber)
// 		})
// 	}
// }
