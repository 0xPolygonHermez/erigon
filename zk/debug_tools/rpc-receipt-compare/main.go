package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"

	libcommon "github.com/gateway-fm/cdk-erigon-lib/common"
	"github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/ethclient"
	"github.com/ledgerwatch/erigon/zk/debug_tools"
	"github.com/ledgerwatch/log/v3"
)

type Log struct {
	Address          string   `json:"address"`
	Topics           []string `json:"topics"`
	Data             string   `json:"data"`
	BlockNumber      string   `json:"blockNumber"`
	TransactionHash  string   `json:"transactionHash"`
	TransactionIndex string   `json:"transactionIndex"`
	BlockHash        string   `json:"blockHash"`
	LogIndex         string   `json:"logIndex"`
	Removed          bool     `json:"removed"`
}

type Receipt struct {
	Root              string `json:"root"`
	CumulativeGasUsed string `json:"cumulativeGasUsed"`
	LogsBloom         string `json:"logsBloom"`
	Logs              []Log  `json:"logs"`
	Status            string `json:"status"`
	TransactionHash   string `json:"transactionHash"`
	TransactionIndex  string `json:"transactionIndex"`
	BlockHash         string `json:"blockHash"`
	BlockNumber       string `json:"blockNumber"`
	GasUsed           string `json:"gasUsed"`
	From              string `json:"from"`
	To                string `json:"to"`
	ContractAddress   string `json:"contractAddress"`
	Type              string `json:"type"`
	EffectiveGasPrice string `json:"effectiveGasPrice"`
}

type Response struct {
	JSONRPC string  `json:"jsonrpc"`
	ID      int     `json:"id"`
	Result  Receipt `json:"result"`
}

type RequestData struct {
	Method  string   `json:"method"`
	Params  []string `json:"params"`
	ID      int      `json:"id"`
	Jsonrpc string   `json:"jsonrpc"`
}

func CalcReceiptHash() {
	var b types.Bloom
	b.SetBytes([]byte{2})

	addr := libcommon.HexToAddress("0xd6dda5aa7749142b7fda3fe4662c9f346101b8a6")

	log1 := &types.Log{
		Address: addr,
		Topics:  []libcommon.Hash{libcommon.HexToHash("0x1")},
		Data:    []byte{1},
		TxHash:  libcommon.HexToHash("0x1"),

		TxIndex:     2,
		BlockNumber: 2,
		BlockHash:   libcommon.HexToHash("0x2"),
		Index:       2,
		Removed:     true,
	}

	receipt := &types.Receipt{
		Type:              1,
		CumulativeGasUsed: 1,
		PostState:         []byte{0, 0, 0, 0},
		Status:            1,
		Bloom:             b,
		Logs:              []*types.Log{log1},
	}
	var receipts types.Receipts
	receipts = append(receipts, receipt)
	receiptSha := types.DeriveSha(receipts)
	log.Warn("receipt PostState", "PostState", receipt.PostState)
	log.Warn("Receipt hash", "receiptSha", receiptSha)
}

func main() {
	ctx := context.Background()
	rpcConfig, err := debug_tools.GetConf()
	if err != nil {
		log.Error("RPGCOnfig", "err", err)
		return
	}

	//CalcReceiptHash()
	//return
	log.Warn("Starting receipt comparison", "blockNumber", rpcConfig.Block)
	defer log.Warn("Check finished.")

	rpcClientRemote, err := ethclient.Dial(rpcConfig.Url)
	if err != nil {
		log.Error("rpcClientRemote.Dial", "err", err)
	}
	rpcClientLocal, err := ethclient.Dial(rpcConfig.LocalUrl)
	if err != nil {
		log.Error("rpcClientLocal.Dial", "err", err)
	}

	blockNum := big.NewInt(rpcConfig.Block)
	// get local block
	blockLocal, err := rpcClientLocal.BlockByNumber(ctx, blockNum)
	if err != nil {
		log.Error("rpcClientRemote.BlockByNumber", "err", err)
	}

	// get remote block
	blockRemote, err := rpcClientRemote.BlockByNumber(ctx, blockNum)
	if err != nil {
		log.Error("rpcClientLocal.BlockByNumber", "err", err)
	}

	if !compareBlock(blockLocal, blockRemote) {
		log.Error("Block mismatch")
	}

	// compare block tx hashes
	txHashesLocal := make([]string, len(blockLocal.Transactions()))
	for i, tx := range blockLocal.Transactions() {
		txHashesLocal[i] = tx.Hash().String()
	}

	txHashesRemote := make([]string, len(blockRemote.Transactions()))
	for i, tx := range blockRemote.Transactions() {
		txHashesRemote[i] = tx.Hash().String()
	}

	// just print errorand continue
	if len(txHashesLocal) != len(txHashesRemote) {
		log.Error("txHashesLocal != txHashesRemote", "txHashesLocal", txHashesLocal, "txHashesRemote", txHashesRemote)
	}

	if len(txHashesLocal) == 0 {
		log.Error("Block has no txs to compare")
		return
	}

	// use the txs on local node since we might be limiting them for debugging purposes \
	// and those are the ones we want to check
	for _, txHash := range txHashesLocal {
		log.Warn("Comparing tx", "txHash", txHash)

		localReceipt, err := getTxReceipt(rpcConfig.LocalUrl, txHash)
		if err != nil {
			log.Error("Getting localReceipt failed:", "err", err)
			continue
		}

		remoteReceipt, err := getTxReceipt(rpcConfig.Url, txHash)
		if err != nil {
			log.Error("Getting remoteReceipt failed:", "err", err)
			continue
		}

		if !compareReceipt(localReceipt, remoteReceipt) {
			log.Error("receipts don't match", "txHash", txHash)
			local, _ := json.MarshalIndent(localReceipt, "", "  ")
			remote, _ := json.MarshalIndent(remoteReceipt, "", "  ")
			writeToFile(fmt.Sprintf("local-hash-%s", txHash), string(local))
			writeToFile(fmt.Sprintf("remote-hash-%s", txHash), string(remote))
		}
	}
}

func writeToFile(filename, content string) error {
	log.Warn("Writing to file", "filename", filename)
	file, err := os.Create(filename)
	if err != nil {
		log.Error("Failed to create file", "filename", filename, "err", err)
		return err
	}
	defer file.Close()

	_, err = file.WriteString(content)
	if err != nil {
		log.Error("Failed to write to file", "filename", filename, "err", err)
		return err
	}

	return nil
}

func getTxReceipt(url string, txHash string) (*Receipt, error) {
	payloadbytecode := RequestData{
		Method:  "eth_getTransactionReceipt",
		Params:  []string{txHash},
		ID:      1,
		Jsonrpc: "2.0",
	}

	jsonPayload, err := json.Marshal(payloadbytecode)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, err
	}
	// req.SetBasicAuth(cfg.Username, cfg.Pass)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to get rpc: %v", resp.Body)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var httpResp Response
	json.Unmarshal(body, &httpResp)

	return &httpResp.Result, nil
}

func compareReceipt(localReceipt, remoteReceipt *Receipt) bool {
	receiptMatches := true
	if len(localReceipt.Logs) != len(remoteReceipt.Logs) {
		return false
	}
	logs1 := localReceipt.Logs
	logs2 := remoteReceipt.Logs

	if localReceipt.Root != remoteReceipt.Root {
		//log.Error("Root mismatch", "Local", localReceipt.Root, "Remote", remoteReceipt.Root)
		//receiptMatches = false
	}
	if localReceipt.CumulativeGasUsed != remoteReceipt.CumulativeGasUsed {
		log.Error("CumulativeGasUsed mismatch", "Local", localReceipt.CumulativeGasUsed, "Remote", remoteReceipt.CumulativeGasUsed)
		receiptMatches = false
	}
	if localReceipt.LogsBloom != remoteReceipt.LogsBloom {
		log.Error("LogsBloom mismatch", "Local", localReceipt.LogsBloom, "Remote", remoteReceipt.LogsBloom)
		receiptMatches = false
	}
	for j := range logs1 {
		if logs1[j].LogIndex != logs2[j].LogIndex {
			log.Error("LogIndex mismatch", "Local", logs1[j].LogIndex, "Remote", logs2[j].LogIndex)
			receiptMatches = false
		}
		if logs1[j].Address != logs2[j].Address {
			log.Error("Address mismatch", "Local", logs1[j].Address, "Remote", logs2[j].Address)
			receiptMatches = false
		}
		top1 := logs1[j].Topics
		top2 := logs2[j].Topics
		for k := range top1 {
			if top1[k] != top2[k] {
				log.Error("Topics mismatch", "Local", top1[k], "Remote", top2[k])
				receiptMatches = false
			}
		}

		if logs1[j].Data != logs2[j].Data {
			log.Error("Data mismatch", "Local", logs1[j].Data, "Remote", logs2[j].Data)
			receiptMatches = false
		}
		if logs1[j].BlockNumber != logs2[j].BlockNumber {
			log.Error("BlockNumber mismatch", "Local", logs1[j].BlockNumber, "Remote", logs2[j].BlockNumber)
			receiptMatches = false
		}
		if logs1[j].TransactionHash != logs2[j].TransactionHash {
			log.Error("TransactionHash mismatch", "Local", logs1[j].TransactionHash, "Remote", logs2[j].TransactionHash)
			receiptMatches = false
		}
		if logs1[j].TransactionIndex != logs2[j].TransactionIndex {
			log.Error("TransactionIndex mismatch", "Local", logs1[j].TransactionIndex, "Remote", logs2[j].TransactionIndex)
			receiptMatches = false
		}

		if logs1[j].Removed != logs2[j].Removed {
			log.Error("Removed mismatch", "Local", logs1[j].Removed, "Remote", logs2[j].Removed)
			receiptMatches = false
		}
	}
	if localReceipt.Status != remoteReceipt.Status {
		log.Error("Status mismatch", "Local", localReceipt.Status, "Remote", remoteReceipt.Status)
		receiptMatches = false
	}
	if localReceipt.TransactionHash != remoteReceipt.TransactionHash {
		log.Error("TransactionHash mismatch", "Local", localReceipt.TransactionHash, "Remote", remoteReceipt.TransactionHash)
		receiptMatches = false
	}
	if localReceipt.TransactionIndex != remoteReceipt.TransactionIndex {
		log.Error("TransactionIndex mismatch", "Local", localReceipt.TransactionIndex, "Remote", remoteReceipt.TransactionIndex)
		receiptMatches = false
	}
	if localReceipt.BlockNumber != remoteReceipt.BlockNumber {
		log.Error("BlockNumber mismatch", "Local", localReceipt.BlockNumber, "Remote", remoteReceipt.BlockNumber)
		receiptMatches = false
	}
	if localReceipt.GasUsed != remoteReceipt.GasUsed {
		log.Error("GasUsed mismatch", "Local", localReceipt.GasUsed, "Remote", remoteReceipt.GasUsed)
		receiptMatches = false
	}
	if localReceipt.From != remoteReceipt.From {
		log.Error("From mismatch", "Local", localReceipt.From, "Remote", remoteReceipt.From)
		receiptMatches = false
	}
	if localReceipt.To != remoteReceipt.To {
		log.Error("To mismatch", "Local", localReceipt.To, "Remote", remoteReceipt.To)
		receiptMatches = false
	}
	if localReceipt.ContractAddress != remoteReceipt.ContractAddress {
		log.Error("ContractAddress mismatch", "Local", localReceipt.ContractAddress, "Remote", remoteReceipt.ContractAddress)
		receiptMatches = false
	}
	if localReceipt.Type != remoteReceipt.Type {
		log.Error("Type mismatch", "Local", localReceipt.Type, "Remote", remoteReceipt.Type)
		receiptMatches = false
	}

	if localReceipt.EffectiveGasPrice != remoteReceipt.EffectiveGasPrice {
		log.Error("EffectiveGasPrice mismatch", "Local", localReceipt.EffectiveGasPrice, "Remote", remoteReceipt.EffectiveGasPrice)
		receiptMatches = false
	}

	return receiptMatches
}

func compareBlock(localBlock, remoteBlock *types.Block) bool {
	blockMatches := true
	if localBlock.Hash() != remoteBlock.Hash() {
		blockMatches = false
		log.Error("Block hash mismatch", "Local", localBlock.Hash(), "Remote", remoteBlock.Hash())
	}
	localHeader := localBlock.Header()
	remoteHeader := remoteBlock.Header()
	if localHeader.ParentHash != remoteHeader.ParentHash {
		blockMatches = false
		log.Error("ParentHash mismatch", "Local", localHeader.ParentHash, "Remote", remoteHeader.ParentHash)
	}
	if localHeader.UncleHash != remoteHeader.UncleHash {
		blockMatches = false
		log.Error("UncleHash mismatch", "Local", localHeader.UncleHash, "Remote", remoteHeader.UncleHash)
	}
	if localHeader.Root != remoteHeader.Root {
		blockMatches = false
		log.Error("Root mismatch", "Local", localHeader.Root, "Remote", remoteHeader.Root)
	}
	if localHeader.TxHash != remoteHeader.TxHash {
		blockMatches = false
		log.Error("TxHash mismatch", "Local", localHeader.TxHash, "Remote", remoteHeader.TxHash)
	}
	if localHeader.ReceiptHash != remoteHeader.ReceiptHash {
		blockMatches = false
		log.Error("ReceiptHash mismatch", "Local", localHeader.ReceiptHash, "Remote", remoteHeader.ReceiptHash)
	}
	if localHeader.Bloom != remoteHeader.Bloom {
		blockMatches = false
		log.Error("Bloom mismatch", "Local", localHeader.Bloom, "Remote", remoteHeader.Bloom)
	}
	if localHeader.Difficulty.Cmp(remoteHeader.Difficulty) != 0 {
		blockMatches = false
		log.Error("Difficulty mismatch", "Local", localHeader.Difficulty, "Remote", remoteHeader.Difficulty)
	}
	if localHeader.Number.Cmp(remoteHeader.Number) != 0 {
		blockMatches = false
		log.Error("Number mismatch", "Local", localHeader.Number, "Remote", remoteHeader.Number)
	}
	if localHeader.GasLimit != remoteHeader.GasLimit {
		blockMatches = false
		log.Error("GasLimit mismatch", "Local", localHeader.GasLimit, "Remote", remoteHeader.GasLimit)
	}
	if localHeader.GasUsed != remoteHeader.GasUsed {
		blockMatches = false
		log.Error("GasUsed mismatch", "Local", localHeader.GasUsed, "Remote", remoteHeader.GasUsed)
	}
	if localHeader.Time != remoteHeader.Time {
		blockMatches = false
		log.Error("Time mismatch", "Local", localHeader.Time, "Remote", remoteHeader.Time)
	}
	if !bytes.Equal(localHeader.Extra, remoteHeader.Extra) {
		blockMatches = false
		log.Error("Extra mismatch", "Local", localHeader.Extra, "Remote", remoteHeader.Extra)
	}

	if localHeader.MixDigest != remoteHeader.MixDigest {
		blockMatches = false
		log.Error("MixDigest mismatch", "Local", localHeader.MixDigest, "Remote", remoteHeader.MixDigest)
	}

	if localHeader.Nonce != remoteHeader.Nonce {
		blockMatches = false
		log.Error("Nonce mismatch", "Local", localHeader.Nonce, "Remote", remoteHeader.Nonce)
	}
	if localHeader.BaseFee != remoteHeader.BaseFee {
		blockMatches = false
		log.Error("BaseFee mismatch", "Local", localHeader.BaseFee, "Remote", remoteHeader.BaseFee)
	}

	return blockMatches
}
