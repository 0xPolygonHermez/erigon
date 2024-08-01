package stages

import (
	"bytes"
	"fmt"
	"math"
	"sort"

	"github.com/ledgerwatch/erigon/core/rawdb"
	"github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/zk/legacy_executor_verifier"
	"github.com/ledgerwatch/erigon/zk/txpool"
	"github.com/ledgerwatch/log/v3"
)

type limboStreamBytesGroup struct {
	blockNumber                uint64
	transactionsIndicesInBlock []int
}

func newLimboStreamBytesGroup(blockNumber uint64) *limboStreamBytesGroup {
	return &limboStreamBytesGroup{
		blockNumber:                blockNumber,
		transactionsIndicesInBlock: make([]int, 0, 1),
	}
}

type limboStreamBytesBuilderHelper struct {
	sendersToGroupMap map[string][]*limboStreamBytesGroup
}

func newLimboStreamBytesBuilderHelper() *limboStreamBytesBuilderHelper {
	return &limboStreamBytesBuilderHelper{
		sendersToGroupMap: make(map[string][]*limboStreamBytesGroup),
	}
}

func (_this *limboStreamBytesBuilderHelper) append(senderMapKey string, blockNumber uint64, transactionIndex int) ([]uint64, [][]int) {
	limboStreamBytesGroups := _this.add(senderMapKey, blockNumber, transactionIndex)

	size := len(limboStreamBytesGroups)
	resultBlocks := make([]uint64, size)
	resultTransactionsSet := make([][]int, size)

	for i := 0; i < size; i++ {
		group := limboStreamBytesGroups[i]
		resultBlocks[i] = group.blockNumber
		resultTransactionsSet[i] = group.transactionsIndicesInBlock
	}

	return resultBlocks, resultTransactionsSet
}

func (_this *limboStreamBytesBuilderHelper) add(senderMapKey string, blockNumber uint64, transactionIndex int) []*limboStreamBytesGroup {
	limboStreamBytesGroups, ok := _this.sendersToGroupMap[senderMapKey]
	if !ok {
		limboStreamBytesGroups = []*limboStreamBytesGroup{newLimboStreamBytesGroup(blockNumber)}
		_this.sendersToGroupMap[senderMapKey] = limboStreamBytesGroups
	}
	group := limboStreamBytesGroups[len(limboStreamBytesGroups)-1]
	if group.blockNumber != blockNumber {
		group = newLimboStreamBytesGroup(blockNumber)
		limboStreamBytesGroups = append(limboStreamBytesGroups, group)
		_this.sendersToGroupMap[senderMapKey] = limboStreamBytesGroups
	}
	group.transactionsIndicesInBlock = append(group.transactionsIndicesInBlock, transactionIndex)

	return limboStreamBytesGroups
}

func handleLimbo(
	batchContext *BatchContext,
	batchState *BatchState,
	verifierBundle *legacy_executor_verifier.VerifierBundle,
	// pool *txpool.TxPool,
	// chainConfig *chain.Config,
) error {
	request := verifierBundle.Request
	response := verifierBundle.Response
	legacyVerifier := batchContext.cfg.legacyVerifier

	log.Info(fmt.Sprintf("[%s] identified an invalid batch, entering limbo", batchContext.s.LogPrefix()), "batch", request.BatchNumber)
	// we have an invalid batch, so we need to notify the txpool that these transactions are spurious
	// and need to go into limbo and then trigger a rewind.  The rewind will put all TX back into the
	// pool, but as it knows about these limbo transactions it will place them into limbo instead
	// of queueing them again

	// now we need to figure out the highest block number in the batch
	// and grab all the transaction hashes along the way to inform the
	// pool of hashes to avoid
	blockNumbers := request.BlockNumbers
	sort.Slice(blockNumbers, func(i, j int) bool {
		return blockNumbers[i] < blockNumbers[j]
	})

	var lowestBlock, highestBlock *types.Block
	forkId, err := batchContext.sdb.hermezDb.GetForkId(request.BatchNumber)
	if err != nil {
		return err
	}

	l1InfoTreeMinTimestamps := make(map[uint64]uint64)
	if _, err = legacyVerifier.GetWholeBatchStreamBytes(request.BatchNumber, batchContext.sdb.tx, blockNumbers, batchContext.sdb.hermezDb.HermezDbReader, l1InfoTreeMinTimestamps, nil); err != nil {
		return err
	}

	limboSendersToPreviousTxMap := make(map[string]uint32)
	limboStreamBytesBuilderHelper := newLimboStreamBytesBuilderHelper()

	limboDetails := txpool.NewLimboBatchDetails()
	limboDetails.Witness = response.Witness
	limboDetails.L1InfoTreeMinTimestamps = l1InfoTreeMinTimestamps
	limboDetails.BatchNumber = request.BatchNumber
	limboDetails.ForkId = forkId

	for _, blockNumber := range blockNumbers {
		block, err := rawdb.ReadBlockByNumber(batchContext.sdb.tx, blockNumber)
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

			signer := types.MakeSigner(batchContext.cfg.chainConfig, blockNumber)
			sender, err := transaction.Sender(*signer)
			if err != nil {
				return err
			}
			senderMapKey := sender.Hex()

			blocksForStreamBytes, transactionsToIncludeByIndex := limboStreamBytesBuilderHelper.append(senderMapKey, blockNumber, i)
			streamBytes, err := legacyVerifier.GetWholeBatchStreamBytes(request.BatchNumber, batchContext.sdb.tx, blocksForStreamBytes, batchContext.sdb.hermezDb.HermezDbReader, l1InfoTreeMinTimestamps, transactionsToIncludeByIndex)
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

			log.Info(fmt.Sprintf("[%s] adding transaction to limbo", batchContext.s.LogPrefix()), "hash", hash)
		}
	}

	limboDetails.TimestampLimit = highestBlock.Time()
	limboDetails.FirstBlockNumber = lowestBlock.NumberU64()
	batchContext.cfg.txPool.ProcessLimboBatchDetails(limboDetails)
	return nil
}
