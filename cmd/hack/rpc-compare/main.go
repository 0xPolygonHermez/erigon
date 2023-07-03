package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/big"
	"net/http"
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

var block = "0x3"

func main() {
	// replace this with your state data
	jsonData := `{
		"0x0000000000000000000000000000000000000000": {
			"Storage": {},
			"Balance": 1175842430000000,
			"Nonce": 0
		},
		"0x000000000000000000000000000000005ca1ab1e": {
			"Storage": {
				"0x0000000000000000000000000000000000000000000000000000000000000000": "0x00000000000000000000000000000003",
				"0x7dfe757ecd65cbd7922a9c0161e935dd7fdbcc0e999689c7d31633896b1fc60b": "0xd5c3e264faaa0becc31b75f07ca2b42c374f7ed98a02650797b911eabb697991",
				"0xcc69885fda6bcc1a4ace058b4a62bf5e179ea78fd58a1ccd71c22cc9b688792f": "0x07483d2b55eb8bb44f617a4de963497fab3fafed5c2b49af975fd73246a5df46",
				"0xd9d16d34ffb15ba3a3d852f0d403e2ce1d691fb54de27ac87cd2f993f3ec330f": "0xc9b9d8bf2cb021742be218774289810d2b3aba5fbf4ebb23ce78429301ab6fcd"
			},
			"Balance": 0,
			"Nonce": 0
		},
		"0x0200143fa295ee4dffef22ee2616c2e008d81688": {
			"Storage": {},
			"Balance": 0,
			"Nonce": 1
		},
		"0x0f99738b2fc14d77308337f3e2596b63ae7bcc4a": {
			"Storage": {
				"0x0000000000000000000000000000000000000000000000000000000000000000": "0xbba0935fa93eb23de7990b47f0d96a8f75766d13"
			},
			"Balance": 0,
			"Nonce": 1
		},
		"0x1dba1131000664b884a1ba238464159892252d3a": {
			"Storage": {},
			"Balance": 98824157570000000,
			"Nonce": 3
		},
		"0x2a3dd3eb832af982ec71669e178424b10dca2ede": {
			"Storage": {
				"0x0000000000000000000000000000000000000000000000000000000000000000": "0x00000000000000000000000000000001",
				"0x0000000000000000000000000000000000000000000000000000000000000001": "0x00000000000000000000000000000001",
				"0x0000000000000000000000000000000000000000000000000000000000000068": "0xa40d5f56745a118d0906a34e69aec8c0db1cb8fa0000000100",
				"0x360894a13ba1a3210667c828492db98dca3e2076cc3735a920a3ca505d382bbc": "0x5ac4182a1dd41aeef465e40b82fd326bf66ab82c",
				"0x5843af22e99e7c98370145a5056245c244ce8ee852f4ef5e6d6a8e410a18cf41": "0x00000000000000000000000000000001",
				"0xb53127684a568b3173ae13b9f8a6016e243e63b6e8ee1178d6a717850b5d6103": "0x0f99738b2fc14d77308337f3e2596b63ae7bcc4a"
			},
			"Balance": 199999999900000000000000000,
			"Nonce": 1
		},
		"0x4c1665d6651ecefa59b9b3041951608468b18891": {
			"Storage": {},
			"Balance": 0,
			"Nonce": 8
		},
		"0x5ac4182a1dd41aeef465e40b82fd326bf66ab82c": {
			"Storage": {},
			"Balance": 0,
			"Nonce": 1
		},
		"0x9d90066e7478496e2284e54c3548106bb4f90e50": {
			"Storage": {},
			"Balance": 0,
			"Nonce": 1
		},
		"0xa40d5f56745a118d0906a34e69aec8c0db1cb8fa": {
			"Storage": {
				"0x360894a13ba1a3210667c828492db98dca3e2076cc3735a920a3ca505d382bbc": "0x0200143fa295ee4dffef22ee2616c2e008d81688",
				"0x9e9b340901522f8b68e31e9eae72c8380b83458fdd78f96f12e62a4a5f62cdad": "0x000000000000000000000000641dde27",
				"0xb53127684a568b3173ae13b9f8a6016e243e63b6e8ee1178d6a717850b5d6103": "0x0f99738b2fc14d77308337f3e2596b63ae7bcc4a"
			},
			"Balance": 0,
			"Nonce": 1
		},
		"0xb33dcb71fc49969473ded72bc361f6be8ee149d1": {
			"Storage": {},
			"Balance": 0,
			"Nonce": 1
		},
		"0xbba0935fa93eb23de7990b47f0d96a8f75766d13": {
			"Storage": {
				"0x0000000000000000000000000000000000000000000000000000000000000002": "0x000000000000000000000000000d2f00",
				"0x33d4aa03df3f12c4f615b40676f67fdafecd3edb5a9c0ca2a47a923dae33a023": "0x00000000000000000000000000000001",
				"0x3412d5605ac6cd444957cedb533e5dacad6378b4bc819ebe3652188a665066d6": "0x5f58e3a2316349923ce3780f8d587db2d72378aed66a8261c916544fa6846ca5",
				"0x531a7c25761aa4b0f2310edca9bb25e1e3ceb49ad4b0422aec866b3ce7567c87": "0x00000000000000000000000000000001",
				"0x64494413541ff93b31aa309254e3fed72a7456e9845988b915b4c7a7ceba8814": "0x5f58e3a2316349923ce3780f8d587db2d72378aed66a8261c916544fa6846ca5",
				"0x76616448da8d124a07383c26a6b2433b3259de946aa40f51524ec96ee05e871a": "0x00000000000000000000000000000001",
				"0x9fa2d8034dbcb437bee38d61fbd100910e1342ffc07f128aa1b8e6790b7f3f68": "0x00000000000000000000000000000001",
				"0xc3ad33e20b0c56a223ad5104fff154aa010f8715b9c981fd38fdc60a4d1a52fc": "0x5f58e3a2316349923ce3780f8d587db2d72378aed66a8261c916544fa6846ca5",
				"0xdae2aa361dfd1ca020a396615627d436107c35eff9fe7738a3512819782d706a": "0x5f58e3a2316349923ce3780f8d587db2d72378aed66a8261c916544fa6846ca5",
				"0xedbedc78c4240c7613622a35de050b48bd6c6d9a31b3d485b68fbbed54a4802d": "0x00000000000000000000000000000001"
			},
			"Balance": 0,
			"Nonce": 1
		},
		"0xcb19edde626906eb1ee52357a27f62dd519608c2": {
			"Storage": {
				"0x0000000000000000000000000000000000000000000000000000000000000000": "0x4c1665d6651ecefa59b9b3041951608468b18891"
			},
			"Balance": 0,
			"Nonce": 4
		}
	}
	`

	data := make(map[string]AccountData)

	err := json.Unmarshal([]byte(jsonData), &data)
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
		fmt.Printf(" \tMismatch detected for %s. Local: 0x%s, Remote: %s\n", key, localValueHex, remoteValueStr)
	}
}
