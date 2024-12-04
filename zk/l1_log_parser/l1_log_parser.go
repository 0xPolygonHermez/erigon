package l1_log_parser

import (
	"fmt"
	"math/big"

	"github.com/ledgerwatch/erigon-lib/common"
	ethTypes "github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/zk/contracts"
	"github.com/ledgerwatch/erigon/zk/types"
	"github.com/ledgerwatch/erigon/zkevm/log"
)

type IL1Syncer interface {
	GetHeader(blockNumber uint64) (*ethTypes.Header, error)
}

type IHermezDb interface {
	WriteSequence(l1BlockNo uint64, batchNo uint64, l1TxHash common.Hash, stateRoot common.Hash, l1InfoRoot common.Hash) error
	WriteVerification(l1BlockNo uint64, batchNo uint64, l1TxHash common.Hash, stateRoot common.Hash) error
	WriteRollupType(rollupType uint64, forkId uint64) error
	WriteNewForkHistory(forkId uint64, latestVerified uint64) error
	RollbackSequences(batchNo uint64) error
	WriteL1InjectedBatch(info *types.L1InjectedBatch) error
}

type L1SyncMeta struct {
	HighestWrittenL1BlockNo uint64
	NewSequencesCount       int
	NewVerificationsCount   int
	HighestVerification     types.BatchVerificationInfo
}

type L1LogParser struct {
	L1Syncer   IL1Syncer
	HermezDb   IHermezDb
	L1RollupId uint64
}

func NewL1LogParser(l1Syncer IL1Syncer, hermezDb IHermezDb, l1RollupId uint64) *L1LogParser {
	return &L1LogParser{
		L1Syncer:   l1Syncer,
		HermezDb:   hermezDb,
		L1RollupId: l1RollupId,
	}
}

func (p *L1LogParser) ParseAndHandleLog(log *ethTypes.Log, syncMeta *L1SyncMeta) (*L1SyncMeta, error) {
	parsedLog, logType, err := p.parseLogType(log)
	if err != nil {
		return syncMeta, err
	}
	return p.handleLog(parsedLog, logType, syncMeta)
}

func (p *L1LogParser) parseLogType(l *ethTypes.Log) (parsedLog interface{}, logType types.BatchLogType, err error) {
	baseInfo := types.BaseBatchInfo{
		BatchNo:   new(big.Int).SetBytes(l.Topics[1].Bytes()).Uint64(),
		L1BlockNo: l.BlockNumber,
		L1TxHash:  common.BytesToHash(l.TxHash.Bytes()),
	}

	switch l.Topics[0] {

	case contracts.SequencedBatchTopicPreEtrog:
		return types.BatchSequenceInfo{
			BaseBatchInfo: baseInfo,
		}, types.LogSequence, nil

	case contracts.SequencedBatchTopicEtrog:
		return types.BatchSequenceInfo{
			BaseBatchInfo: baseInfo,
			L1InfoRoot:    common.BytesToHash(l.Data[:32]),
		}, types.LogSequenceEtrog, nil

	case contracts.VerificationTopicPreEtrog:
		return types.BatchVerificationInfo{
			BaseBatchInfo: baseInfo,
			StateRoot:     common.BytesToHash(l.Data[:32]),
		}, types.LogVerify, nil

	case contracts.VerificationTopicEtrog:
		return types.BatchVerificationInfo{
			BaseBatchInfo: baseInfo,
			StateRoot:     common.BytesToHash(l.Data[:32]),
		}, types.LogVerifyEtrog, nil

	case contracts.VerificationValidiumTopicEtrog:
		return types.BatchVerificationInfo{
			BaseBatchInfo: baseInfo,
			StateRoot:     common.BytesToHash(l.Data[:32]),
		}, types.LogVerifyEtrog, nil

	case contracts.InitialSequenceBatchesTopic:
		header, err := p.L1Syncer.GetHeader(l.BlockNumber)
		if err != nil {
			return nil, types.LogUnknown, err
		}

		// the log appears to have some trailing some bytes of all 0s in it.  Not sure why but we can't handle the
		// TX without trimming these off
		injectedBatchLogTrailingBytes := getTrailingCutoffLen(l.Data)
		trailingCutoff := len(l.Data) - injectedBatchLogTrailingBytes
		log.Debug(fmt.Sprintf("Handle initial sequence batches, trail len:%v, log data: %v", injectedBatchLogTrailingBytes, l.Data))

		txData := l.Data[injectedBatchLogTransactionStartByte:trailingCutoff]

		return &types.L1InjectedBatch{
			L1BlockNumber:      l.BlockNumber,
			Timestamp:          header.Time,
			L1BlockHash:        header.Hash(),
			L1ParentHash:       header.ParentHash,
			LastGlobalExitRoot: common.BytesToHash(l.Data[injectedBatchLastGerStartByte:injectedBatchLastGerEndByte]),
			Sequencer:          common.BytesToAddress(l.Data[injectedBatchSequencerStartByte:injectedBatchSequencerEndByte]),
			Transaction:        txData,
		}, types.LogInjectedBatch, nil

	case contracts.CreateNewRollupTopic:
		rollupId := new(big.Int).SetBytes(l.Topics[1].Bytes()).Uint64()
		return types.RollupUpdateInfo{
			NewRollup:  rollupId,
			RollupType: new(big.Int).SetBytes(l.Data[0:32]).Uint64(),
		}, types.LogRollupCreate, nil

	case contracts.AddNewRollupTypeTopic:
	case contracts.AddNewRollupTypeTopicBanana:
		return types.RollupUpdateInfo{
			RollupType: new(big.Int).SetBytes(l.Topics[1].Bytes()).Uint64(),
			ForkId:     new(big.Int).SetBytes(l.Data[64:96]).Uint64(),
		}, types.LogAddRollupType, nil

	case contracts.UpdateRollupTopic:
		return types.RollupUpdateInfo{
			NewRollup:      new(big.Int).SetBytes(l.Data[0:32]).Uint64(),
			LatestVerified: new(big.Int).SetBytes(l.Data[32:64]).Uint64(),
		}, types.LogL1InfoTreeUpdate, nil

	case contracts.RollbackBatchesTopic:
		return types.BatchVerificationInfo{
			BaseBatchInfo: baseInfo,
		}, types.LogRollbackBatches, nil

	default:
		return types.UnknownBatchInfo{
			BaseBatchInfo: baseInfo,
		}, types.LogUnknown, nil
	}

	return nil, types.LogUnknown, nil
}

func (p *L1LogParser) handleLog(
	l interface{},
	logType types.BatchLogType,
	syncMeta *L1SyncMeta) (*L1SyncMeta, error) {

	switch logType {
	case types.LogSequence:
		fallthrough

	case types.LogSequenceEtrog:
		info := l.(types.BatchSequenceInfo)
		if logType == types.LogSequenceEtrog && p.L1RollupId > 1 {
			return syncMeta, nil
		}
		// NB: state root is nil post etrog as it is not found in the L1 logs for sequenced batches
		if err := p.HermezDb.WriteSequence(info.L1BlockNo, info.BatchNo, info.L1TxHash, info.StateRoot, info.L1InfoRoot); err != nil {
			return syncMeta, fmt.Errorf("WriteSequence: %w", err)
		}
		if info.L1BlockNo > syncMeta.HighestWrittenL1BlockNo {
			syncMeta.HighestWrittenL1BlockNo = info.L1BlockNo
		}
		syncMeta.NewSequencesCount++

	case types.LogVerify:
		fallthrough

	case types.LogVerifyEtrog:
		info := l.(types.BatchVerificationInfo)
		if logType == types.LogVerify && p.L1RollupId > 1 {
			return syncMeta, nil
		}
		if info.BatchNo > syncMeta.HighestVerification.BatchNo {
			syncMeta.HighestVerification = info
		}
		if err := p.HermezDb.WriteVerification(info.L1BlockNo, info.BatchNo, info.L1TxHash, info.StateRoot); err != nil {
			return syncMeta, fmt.Errorf("WriteVerification for block %d: %w", info.L1BlockNo, err)
		}
		if info.L1BlockNo > syncMeta.HighestWrittenL1BlockNo {
			syncMeta.HighestWrittenL1BlockNo = info.L1BlockNo
		}
		syncMeta.NewVerificationsCount++
		return syncMeta, nil

	case types.LogAddRollupType:
		info := l.(types.RollupUpdateInfo)
		return syncMeta, p.HermezDb.WriteRollupType(info.RollupType, info.ForkId)

	case types.LogL1InfoTreeUpdate:
		info := l.(types.RollupUpdateInfo)
		return syncMeta, p.HermezDb.WriteNewForkHistory(info.ForkId, info.LatestVerified)

	case types.LogRollbackBatches:
		info := l.(types.BatchVerificationInfo)
		if info.L1BlockNo < syncMeta.HighestWrittenL1BlockNo {
			syncMeta.HighestWrittenL1BlockNo = info.L1BlockNo
		}
		return syncMeta, p.HermezDb.RollbackSequences(info.BatchNo)

	case types.LogInjectedBatch:
		info := l.(*types.L1InjectedBatch)
		return syncMeta, p.HermezDb.WriteL1InjectedBatch(info)

	default:
		log.Warn("Unknown log type", "logType", logType)
		return syncMeta, nil
	}

	return syncMeta, nil
}

const (
	injectedBatchLogTransactionStartByte = 128
	injectedBatchLastGerStartByte        = 32
	injectedBatchLastGerEndByte          = 64
	injectedBatchSequencerStartByte      = 76
	injectedBatchSequencerEndByte        = 96
)

func getTrailingCutoffLen(logData []byte) int {
	for i := len(logData) - 1; i >= 0; i-- {
		if logData[i] != 0 {
			return len(logData) - i - 1
		}
	}
	return 0
}
