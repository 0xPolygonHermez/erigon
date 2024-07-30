package stages

import (
	"bytes"
	"fmt"
	"math"
	"sort"

	"github.com/ledgerwatch/erigon/chain"
	"github.com/ledgerwatch/erigon/core/rawdb"
	"github.com/ledgerwatch/erigon/core/types"
	verifier "github.com/ledgerwatch/erigon/zk/legacy_executor_verifier"
	"github.com/ledgerwatch/erigon/zk/txpool"
	"github.com/ledgerwatch/log/v3"
)

func handleLimbo(
	logPrefix string,
	sdb *stageDb,
	batchNo uint64,
	forkId uint64,
	verifier *verifier.LegacyExecutorVerifier,
	response *verifier.VerifierResponse,
	pool *txpool.TxPool,
	chainConfig *chain.Config,
) error {

	blockNumbers, err := sdb.hermezDb.GetL2BlockNosByBatch(batchNo)
	if err != nil {
		return err
	}
	if len(blockNumbers) == 0 {
		panic("failing to verify a batch without blocks")
	}
	sort.Slice(blockNumbers, func(i, j int) bool {
		return blockNumbers[i] < blockNumbers[j]
	})

	var lowestBlock, highestBlock *types.Block

	l1InfoTreeMinTimestamps := make(map[uint64]uint64)
	_, err = verifier.GetWholeBatchStreamBytes(batchNo, sdb.tx, blockNumbers, sdb.hermezDb.HermezDbReader, l1InfoTreeMinTimestamps, nil)
	if err != nil {
		return err
	}

	limboSendersToPreviousTxMap := make(map[string]uint32)
	limboStreamBytesBuilderHelper := newLimboStreamBytesBuilderHelper()

	limboDetails := txpool.NewLimboBatchDetails()
	limboDetails.Witness = response.Witness
	limboDetails.L1InfoTreeMinTimestamps = l1InfoTreeMinTimestamps
	limboDetails.BatchNumber = response.BatchNumber
	limboDetails.ForkId = forkId

	for _, blockNumber := range blockNumbers {
		block, err := rawdb.ReadBlockByNumber(sdb.tx, blockNumber)
		if err != nil {
			return err
		}
		highestBlock = block
		if lowestBlock == nil {
			// capture the first block, then we can set the bad block hash in the unwind to terminate the
			// stage loop and broadcast the accumulator changes to the txpool before the next stage loop run
			lowestBlock = block
		}

		for i, transaction := range block.Transactions() {
			var b []byte
			buffer := bytes.NewBuffer(b)
			err = transaction.EncodeRLP(buffer)
			if err != nil {
				return err
			}

			signer := types.MakeSigner(chainConfig, blockNumber)
			sender, err := transaction.Sender(*signer)
			if err != nil {
				return err
			}
			senderMapKey := sender.Hex()

			blocksForStreamBytes, transactionsToIncludeByIndex := limboStreamBytesBuilderHelper.append(senderMapKey, blockNumber, i)
			streamBytes, err := verifier.GetWholeBatchStreamBytes(response.BatchNumber, sdb.tx, blocksForStreamBytes, sdb.hermezDb.HermezDbReader, l1InfoTreeMinTimestamps, transactionsToIncludeByIndex)
			if err != nil {
				return err
			}

			previousTxIndex, ok := limboSendersToPreviousTxMap[senderMapKey]
			if !ok {
				previousTxIndex = math.MaxUint32
			}

			hash := transaction.Hash()
			limboTxCount := limboDetails.AppendTransaction(buffer.Bytes(), streamBytes, hash, sender, previousTxIndex)
			limboSendersToPreviousTxMap[senderMapKey] = limboTxCount - 1

			log.Info(fmt.Sprintf("[%s] adding transaction to limbo", logPrefix), "hash", hash)
		}
	}

	limboDetails.TimestampLimit = highestBlock.Time()
	limboDetails.FirstBlockNumber = lowestBlock.NumberU64()
	pool.ProcessLimboBatchDetails(limboDetails)
	return nil
}
