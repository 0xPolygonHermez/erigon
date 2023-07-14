package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"net/http"
	"os"
	"strings"
)

type HTTPResponse struct {
	Result string `json:"result"`
}

type RequestData struct {
	Method  string   `json:"method"`
	Params  []string `json:"params"`
	ID      int      `json:"id"`
	Jsonrpc string   `json:"jsonrpc"`
}

type AccountData struct {
	Nonce   *big.Int          `json:"nonce"`
	Balance *big.Int          `json:"balance"`
	Storage map[string]string `json:"Storage"`
}

func toHex(i int64) string {
	return fmt.Sprintf("0x%x", i)
}

func toInt(s string) int64 {
	i, _ := new(big.Int).SetString(s, 10)
	return i.Int64()
}

var block = toHex(5382)

func main() {
	// read block number from a command-line flag if present
	if len(os.Args) > 1 {
		// concert to int
		block = toHex(toInt(os.Args[1]))
	}

	// read from stdin until end of file
	jsonData, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		fmt.Println("Error reading from Stdin:", err)
		return
	}

	data := make(map[string]AccountData)

	err = json.Unmarshal([]byte(jsonData), &data)
	if err != nil {
		fmt.Println("Error parsing JSON data:", err)
		return
	}

	url := "https://zkevm-rpc.com/"
	for accountHash, accountData := range data {
		fmt.Println(accountHash)

		// check nonce
		payloadNonce := RequestData{
			Method:  "eth_getTransactionCount",
			Params:  []string{accountHash, block},
			ID:      1,
			Jsonrpc: "2.0",
		}
		compareValues("Nonce", accountData.Nonce, url, payloadNonce)

		// check balance
		payloadBalance := RequestData{
			Method:  "eth_getBalance",
			Params:  []string{accountHash, block},
			ID:      1,
			Jsonrpc: "2.0",
		}
		compareValues("Balance", accountData.Balance, url, payloadBalance)

		// check storage
		for storageLocation, localValue := range accountData.Storage {
			payloadStorage := RequestData{
				Method:  "eth_getStorageAt",
				Params:  []string{accountHash, storageLocation, block},
				ID:      1,
				Jsonrpc: "2.0",
			}
			compareValuesString(storageLocation, localValue, url, payloadStorage)
		}
	}
}

func compareValuesString(key string, localValue string, url string, payload RequestData) {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		fmt.Println("Error preparing request:", err)
		return
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		fmt.Println("Error sending request:", err)
		return
	}

	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	var httpResp HTTPResponse
	json.Unmarshal(body, &httpResp)
	remoteValueDecode := httpResp.Result

	localValueStr := strings.TrimPrefix(localValue, "0x")
	remoteValueStr := strings.TrimPrefix(remoteValueDecode, "0x")

	localValueInt, _ := new(big.Int).SetString(localValueStr, 16)
	remoteValueInt, _ := new(big.Int).SetString(remoteValueStr, 16)

	if localValueInt == nil {
		fmt.Println("Local value not found for", key)
		return
	}

	if remoteValueInt == nil {
		fmt.Println("Remote value not found for", key)
		fmt.Println(localValue)
		return
	}

	fmt.Printf("\t %s : 0x%064x : %s\n", key, localValueInt, remoteValueDecode)
	if localValueInt.Cmp(remoteValueInt) != 0 {
		fmt.Printf("Mismatch detected for %s. Local: %s, Remote: %s\n", key, localValue, remoteValueStr)
	}
}

func compareValues(key string, localValue *big.Int, url string, payload RequestData) {
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		fmt.Println("Error preparing request:", err)
		return
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		fmt.Println("Error sending request:", err)
		return
	}

	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	var httpResp HTTPResponse
	json.Unmarshal(body, &httpResp)
	remoteValueDecode := httpResp.Result

	remoteValueStr := strings.TrimPrefix(remoteValueDecode, "0x")

	remoteValueInt := new(big.Int)
	remoteValueInt.SetString(remoteValueStr, 16)

	if remoteValueInt == nil {
		fmt.Println("Remote value not found for", key)
		fmt.Println(localValue)
		return
	}

	localValueHex := localValue.Text(16)

	fmt.Printf("\t %s : 0x%s : %s\n", key, localValueHex, remoteValueDecode)
	if localValue.Cmp(remoteValueInt) != 0 {
		fmt.Printf(" \tMismatch detected for %s. Local: 0x%s, Remote: 0x%s\n", key, localValueHex, remoteValueStr)
	}
}
