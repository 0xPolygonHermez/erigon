package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
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

var block = "0xDF7"
var url = "https://zkevm-rpc.com/"
var jsonFile = "addrDump.json"

func main() {
	jsonFile, err := os.Open(jsonFile)
	if err != nil {
		fmt.Println(err)
	}
	defer jsonFile.Close()

	data := make(map[string]map[string]string)
	byteValue, _ := ioutil.ReadAll(jsonFile)
	err = json.Unmarshal(byteValue, &data)
	if err != nil {
		fmt.Println("Error parsing JSON data:", err)
		return
	}

	for accountHash, storageMap := range data {
		for key, value := range storageMap {
			compareValuesString(accountHash, key, value)
		}
	}

	fmt.Println("Check finished.")
}

func compareValuesString(accountHash, key, value string) {
	payloadbytecode := RequestData{
		Method:  "eth_getStorageAt",
		Params:  []string{accountHash, key, block},
		ID:      1,
		Jsonrpc: "2.0",
	}

	jsonPayload, err := json.Marshal(payloadbytecode)
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

	localValueStr := strings.TrimPrefix(value, "0x")
	localValueStr = fmt.Sprintf("%064s", localValueStr)
	remoteValueStr := strings.TrimPrefix(remoteValueDecode, "0x")

	// fmt.Println("Checking", accountHash)
	if !strings.EqualFold(localValueStr, remoteValueStr) {
		fmt.Printf("Mismatch detected for %s and key %s. Local: %s, Remote: %s\n", accountHash, key, localValueStr, remoteValueStr)
	}
}
