package blockinfo

import (
	"context"
	"fmt"
	"math/big"

	ethTypes "github.com/ledgerwatch/erigon/core/types"

	"github.com/ledgerwatch/erigon/smt/pkg/smt"
	"github.com/ledgerwatch/erigon/smt/pkg/utils"
	zktx "github.com/ledgerwatch/erigon/zk/tx"

	"github.com/gateway-fm/cdk-erigon-lib/common"
	"github.com/ledgerwatch/log/v3"
)

type ExecutedTxInfo struct {
	Tx                ethTypes.Transaction
	EffectiveGasPrice uint8
	Receipt           *ethTypes.Receipt
	Signer            *common.Address
}

func BuildBlockInfoTree(
	coinbase *common.Address,
	blockNumber,
	blockTime,
	blockGasLimit,
	blockGasUsed uint64,
	ger common.Hash,
	l1BlockHash common.Hash,
	previousStateRoot common.Hash,
	transactionInfos *[]ExecutedTxInfo,
) (*common.Hash, error) {
	infoTree := NewBlockInfoTree()
	keys, vals, err := infoTree.GenerateBlockHeader(&previousStateRoot, coinbase, blockNumber, blockGasLimit, blockTime, &ger, &l1BlockHash)
	if err != nil {
		return nil, err
	}

	log.Trace("info-tree-header",
		"blockNumber", blockNumber,
		"previousStateRoot", previousStateRoot.String(),
		"coinbase", coinbase.String(),
		"blockGasLimit", blockGasLimit,
		"blockTime", blockTime,
		"ger", ger.String(),
		"l1BlockHash", l1BlockHash.String(),
	)
	var logIndex int64 = 0
	for i, txInfo := range *transactionInfos {
		receipt := txInfo.Receipt
		t := txInfo.Tx

		l2TxHash, err := zktx.ComputeL2TxHash(
			t.GetChainID().ToBig(),
			t.GetValue(),
			t.GetPrice(),
			t.GetNonce(),
			t.GetGas(),
			t.GetTo(),
			txInfo.Signer,
			t.GetData(),
		)
		if err != nil {
			return nil, err
		}

		log.Trace("info-tree-tx", "block", blockNumber, "idx", i, "hash", l2TxHash.String())

		genKeys, genVals, err := infoTree.GenerateBlockTxKeysVals(&l2TxHash, i, receipt, logIndex, receipt.CumulativeGasUsed, txInfo.EffectiveGasPrice)
		if err != nil {
			return nil, err
		}
		keys = append(keys, genKeys...)
		vals = append(vals, genVals...)

		logIndex += int64(len(receipt.Logs))
	}

	key, val, err := generateBlockGasUsed(blockGasUsed)
	if err != nil {
		return nil, err
	}
	keys = append(keys, key)
	vals = append(vals, val)

	root, err := infoTree.smt.InsertBatch(context.Background(), "", keys, vals, nil, nil)
	if err != nil {
		return nil, err
	}
	rootHash := common.BigToHash(root.NewRootScalar.ToBigInt())

	log.Trace("info-tree-root", "block", blockNumber, "root", rootHash.String())

	return &rootHash, nil
}

type BlockInfoTree struct {
	smt *smt.SMT
}

func NewBlockInfoTree() *BlockInfoTree {
	return &BlockInfoTree{
		smt: smt.NewSMT(nil),
	}
}
func (b *BlockInfoTree) GetRoot() *big.Int {
	return b.smt.LastRoot()
}

func (b *BlockInfoTree) GenerateBlockHeader(oldBlockHash *common.Hash, coinbase *common.Address, blockNumber, gasLimit, timestamp uint64, ger, l1BlochHash *common.Hash) (keys []*utils.NodeKey, vals []*utils.NodeValue8, err error) {
	var key *utils.NodeKey
	var val *utils.NodeValue8
	keys = make([]*utils.NodeKey, 7)
	vals = make([]*utils.NodeValue8, 7)

	if key, val, err = generateL2BlockHash(oldBlockHash); err != nil {
		return nil, nil, err
	}
	keys[0] = key
	vals[0] = val

	if key, val, err = generateCoinbase(b.smt, coinbase); err != nil {
		return nil, nil, err
	}
	keys[1] = key
	vals[1] = val

	if key, val, err = generateBlockNumber(b.smt, blockNumber); err != nil {
		return nil, nil, err
	}
	keys[2] = key
	vals[2] = val

	if key, val, err = generateGasLimit(b.smt, gasLimit); err != nil {
		return nil, nil, err
	}
	keys[3] = key
	vals[3] = val

	if key, val, err = generateTimestamp(b.smt, timestamp); err != nil {
		return nil, nil, err
	}
	keys[4] = key
	vals[4] = val

	if key, val, err = generateGer(b.smt, ger); err != nil {
		return nil, nil, err
	}
	keys[5] = key
	vals[5] = val

	if key, val, err = generateL1BlockHash(b.smt, l1BlochHash); err != nil {
		return nil, nil, err
	}
	keys[6] = key
	vals[6] = val

	return keys, vals, nil
}

func generateL2BlockHash(blockHash *common.Hash) (key *utils.NodeKey, value *utils.NodeValue8, err error) {
	if key, err = KeyBlockHeaderParams(big.NewInt(IndexBlockHeaderParamBlockHash)); err != nil {
		return nil, nil, err
	}
	if value, err = bigInt2NodeVal8(blockHash.Big()); err != nil {
		return nil, nil, err
	}
	return key, value, nil
}

func generateCoinbase(smt *smt.SMT, coinbase *common.Address) (key *utils.NodeKey, value *utils.NodeValue8, err error) {
	if key, err = KeyBlockHeaderParams(big.NewInt(IndexBlockHeaderParamCoinbase)); err != nil {
		return nil, nil, err
	}

	if value, err = bigInt2NodeVal8(coinbase.Hash().Big()); err != nil {
		return nil, nil, err
	}

	return key, value, nil
}

func generateGasLimit(smt *smt.SMT, gasLimit uint64) (key *utils.NodeKey, value *utils.NodeValue8, err error) {
	if key, err = KeyBlockHeaderParams(big.NewInt(IndexBlockHeaderParamGasLimit)); err != nil {
		return nil, nil, err
	}

	if value, err = bigInt2NodeVal8(big.NewInt(0).SetUint64(gasLimit)); err != nil {
		return nil, nil, err
	}
	return key, value, nil
}

func generateBlockNumber(smt *smt.SMT, blockNumber uint64) (key *utils.NodeKey, value *utils.NodeValue8, err error) {
	if key, err = KeyBlockHeaderParams(big.NewInt(IndexBlockHeaderParamNumber)); err != nil {
		return nil, nil, err
	}

	if value, err = bigInt2NodeVal8(big.NewInt(0).SetUint64(blockNumber)); err != nil {
		return nil, nil, err
	}
	return key, value, nil
}

func generateTimestamp(smt *smt.SMT, timestamp uint64) (key *utils.NodeKey, value *utils.NodeValue8, err error) {
	if key, err = KeyBlockHeaderParams(big.NewInt(IndexBlockHeaderParamTimestamp)); err != nil {
		return nil, nil, err
	}

	if value, err = bigInt2NodeVal8(big.NewInt(0).SetUint64(timestamp)); err != nil {
		return nil, nil, err
	}

	return key, value, nil
}

func generateGer(smt *smt.SMT, ger *common.Hash) (key *utils.NodeKey, value *utils.NodeValue8, err error) {
	if key, err = KeyBlockHeaderParams(big.NewInt(IndexBlockHeaderParamGer)); err != nil {
		return nil, nil, err
	}

	if value, err = bigInt2NodeVal8(ger.Big()); err != nil {
		return nil, nil, err
	}

	return key, value, nil
}

func generateL1BlockHash(smt *smt.SMT, blockHash *common.Hash) (key *utils.NodeKey, value *utils.NodeValue8, err error) {
	if key, err = KeyBlockHeaderParams(big.NewInt(IndexBlockHeaderParamBlockHashL1)); err != nil {
		return nil, nil, err
	}

	if value, err = bigInt2NodeVal8(blockHash.Big()); err != nil {
		return nil, nil, err
	}

	return key, value, nil
}

func bigInt2NodeVal8(val *big.Int) (*utils.NodeValue8, error) {
	x := utils.ScalarToArrayBig(val)
	v, err := utils.NodeValue8FromBigIntArray(x)
	if err != nil {
		return nil, err
	}

	return v, nil
}

func generateL2TxHash(txIndex *big.Int, l2TxHash *big.Int) (key *utils.NodeKey, value *utils.NodeValue8, err error) {
	if key, err = KeyTxHash(txIndex); err != nil {
		return nil, nil, err
	}
	if value, err = bigInt2NodeVal8(l2TxHash); err != nil {
		return nil, nil, err
	}

	return key, value, nil
}

func generateTxStatus(txIndex *big.Int, status *big.Int) (key *utils.NodeKey, value *utils.NodeValue8, err error) {
	if key, err = KeyTxStatus(txIndex); err != nil {
		return nil, nil, err
	}
	if value, err = bigInt2NodeVal8(status); err != nil {
		return nil, nil, err
	}

	return key, value, nil
}

func generateCumulativeGasUsed(txIndex, cumulativeGasUsed *big.Int) (key *utils.NodeKey, value *utils.NodeValue8, err error) {
	if key, err = KeyCumulativeGasUsed(txIndex); err != nil {
		return nil, nil, err
	}
	if value, err = bigInt2NodeVal8(cumulativeGasUsed); err != nil {
		return nil, nil, err
	}
	return key, value, nil
}

func generateTxLog(txIndex *big.Int, logIndex *big.Int, log *big.Int) (key *utils.NodeKey, value *utils.NodeValue8, err error) {
	if key, err = KeyTxLogs(txIndex, logIndex); err != nil {
		return nil, nil, err
	}
	if value, err = bigInt2NodeVal8(log); err != nil {
		return nil, nil, err
	}

	return key, value, nil
}

func generateTxEffectivePercentage(txIndex, effectivePercentage *big.Int) (key *utils.NodeKey, value *utils.NodeValue8, err error) {
	if key, err = KeyEffectivePercentage(txIndex); err != nil {
		return nil, nil, err
	}
	if value, err = bigInt2NodeVal8(effectivePercentage); err != nil {
		return nil, nil, err
	}

	return key, value, nil
}

func generateBlockGasUsed(gasUsed uint64) (key *utils.NodeKey, value *utils.NodeValue8, err error) {
	if key, err = KeyBlockHeaderParams(big.NewInt(IndexBlockHeaderParamGasUsed)); err != nil {
		return nil, nil, err
	}
	gasUsedBig := big.NewInt(0).SetUint64(gasUsed)
	if value, err = bigInt2NodeVal8(gasUsedBig); err != nil {
		return nil, nil, err
	}

	return key, value, nil
}

func (b *BlockInfoTree) GenerateBlockTxKeysVals(
	l2TxHash *common.Hash,
	txIndex int,
	receipt *ethTypes.Receipt,
	logIndex int64,
	cumulativeGasUsed uint64,
	effectivePercentage uint8,
) ([]*utils.NodeKey, []*utils.NodeValue8, error) {
	var keys []*utils.NodeKey
	var vals []*utils.NodeValue8
	txIndexBig := big.NewInt(int64(txIndex))

	key, val, err := generateL2TxHash(txIndexBig, l2TxHash.Big())
	if err != nil {
		return nil, nil, err
	}
	keys = append(keys, key)
	vals = append(vals, val)

	bigStatus := big.NewInt(0).SetUint64(receipt.Status)
	key, val, err = generateTxStatus(txIndexBig, bigStatus)
	if err != nil {
		return nil, nil, err
	}
	keys = append(keys, key)
	vals = append(vals, val)

	bigCumulativeGasUsed := big.NewInt(0).SetUint64(cumulativeGasUsed)
	key, val, err = generateCumulativeGasUsed(txIndexBig, bigCumulativeGasUsed)
	if err != nil {
		return nil, nil, err
	}
	keys = append(keys, key)
	vals = append(vals, val)

	log.Trace("info-tree-tx-inner",
		"tx-index", txIndex,
		"log-index", logIndex,
		"cumulativeGasUsed", cumulativeGasUsed,
		"effective-percentage", effectivePercentage,
		"receipt-status", receipt.Status,
	)

	// now encode the logs
	for _, rLog := range receipt.Logs {
		reducedTopics := ""
		for _, topic := range rLog.Topics {
			reducedTopics += fmt.Sprintf("%x", topic)
		}

		logToEncode := fmt.Sprintf("0x%x%s", rLog.Data, reducedTopics)

		hash, err := utils.HashContractBytecode(logToEncode)
		if err != nil {
			return nil, nil, err
		}

		logEncodedBig := utils.ConvertHexToBigInt(hash)
		key, val, err = generateTxLog(txIndexBig, big.NewInt(logIndex), logEncodedBig)
		if err != nil {
			return nil, nil, err
		}
		keys = append(keys, key)
		vals = append(vals, val)

		log.Trace("info-tree-tx-receipt-log",
			"topics", reducedTopics,
			"to-encode", logToEncode,
			"log-index", logIndex,
		)

		// increment log index
		logIndex += 1
	}

	// setTxEffectivePercentage
	bigEffectivePercentage := big.NewInt(0).SetUint64(uint64(effectivePercentage))
	key, val, err = generateTxEffectivePercentage(txIndexBig, bigEffectivePercentage)
	if err != nil {
		return nil, nil, err
	}
	keys = append(keys, key)
	vals = append(vals, val)

	return keys, vals, nil
}
