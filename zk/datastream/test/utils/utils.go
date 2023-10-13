package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type RequestData struct {
	Method  string   `json:"method"`
	Params  []string `json:"params"`
	ID      int      `json:"id"`
	Jsonrpc string   `json:"jsonrpc"`
}

type HTTPResponse struct {
	JsonRpc string `json:"jsonrpc"`
	Id      int    `json:"id"`
	Result  Result `json:"result"`
}

type Result struct {
	Number       string   `json:"number"`
	StateRoot    string   `json:"stateRoot"`
	Timestamp    string   `json:"timestamp"`
	Transactions []string `json:"transactions"`
}

func GetBlockByHash(blockHash string) (Result, error) {
	payloadbytecode := RequestData{
		Method:  "eth_getBlockByHash",
		Params:  []string{blockHash},
		ID:      1,
		Jsonrpc: "2.0",
	}

	jsonPayload, err := json.Marshal(payloadbytecode)
	if err != nil {
		return Result{}, err
	}

	req, err := http.NewRequest("POST", "https://rpc.internal.zkevm-test.net", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return Result{}, err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, err
	}

	if resp.StatusCode != 200 {
		return Result{}, fmt.Errorf(string(body))
	}

	var httpResp HTTPResponse
	err = json.Unmarshal(body, &httpResp)
	if err != nil {
		return Result{}, err
	}
	result := httpResp.Result

	return result, nil
}
