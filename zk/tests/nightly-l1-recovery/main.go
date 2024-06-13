package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/bitly/go-simplejson"
)

const (
	erigonNodeURL = "http://erigon:8545"     // URL for the Erigon node
	otherNodeURL  = "http://other-node:8545" // URL for the other node to compare against
	checkInterval = 30 * time.Second         // Interval to check the block height
	runDuration   = 10 * time.Minute         // Total duration to run the checks
)

func getBlockHeight(url string) (*simplejson.Json, error) {
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	reqBody := `{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}`
	req.Body = http.NoBody
	req.ContentLength = int64(len(reqBody))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	jsonResp, err := simplejson.NewFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	return jsonResp, nil
}

func compareBlockHeights(erigonHeight, otherHeight *simplejson.Json) {
	erigonBlock, _ := erigonHeight.Get("result").String()
	otherBlock, _ := otherHeight.Get("result").String()

	if erigonBlock != otherBlock {
		diff, _ := json.MarshalIndent(map[string]string{
			"erigon": erigonBlock,
			"other":  otherBlock,
		}, "", "  ")
		log.Println("Block height mismatch detected:", string(diff))
	} else {
		log.Println("Block heights are equal:", erigonBlock)
	}
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), runDuration)
	defer cancel()

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Stopping block height checks.")
			return
		case <-ticker.C:
			erigonHeight, err := getBlockHeight(erigonNodeURL)
			if err != nil {
				log.Println("Error fetching block height from Erigon:", err)
				continue
			}

			otherHeight, err := getBlockHeight(otherNodeURL)
			if err != nil {
				log.Println("Error fetching block height from other node:", err)
				continue
			}

			compareBlockHeights(erigonHeight, otherHeight)
		}
	}
}
