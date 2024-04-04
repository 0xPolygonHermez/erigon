package legacy_executor_verifier

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"os"
	"github.com/ledgerwatch/erigon/zk/legacy_executor_verifier/proto/github.com/0xPolygonHermez/zkevm-node/state/runtime/executor"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestVerifyBatches(t *testing.T) {
	dumpDir := filepath.Join("verifier_dumps")
	payloadFiles, err := os.ReadDir(dumpDir)
	if err != nil {
		t.Fatalf("Failed to read dump directory: %v", err)
	}

	sort.Slice(payloadFiles, func(i, j int) bool {
		batchNumI := extractBatchNumber(payloadFiles[i].Name())
		batchNumJ := extractBatchNumber(payloadFiles[j].Name())
		return batchNumI < batchNumJ
	})

	executorUrls := []string{
		"34.175.105.40:50071",
		"34.175.237.160:50071",
		"34.175.42.120:50071",
		"34.175.236.212:50071",
		"34.175.232.61:50071",
		"34.175.167.26:50071",
		"51.210.116.237:50071",
		"34.175.214.161:50071",
		//"34.175.73.226:50071",
	}
	executors := make([]*Executor, len(executorUrls))
	for i, url := range executorUrls {
		executors[i], err = NewExecutor(url, 5000*time.Millisecond, false)
		if err != nil {
			t.Fatalf("Failed to create executor %s: %v,", url, err)
		}
	}

	for _, fileInfo := range payloadFiles {
		if !strings.Contains(fileInfo.Name(), "-payload.json") {
			continue
		}

		payloadPath := filepath.Join(dumpDir, fileInfo.Name())
		payload, request, err := loadPayloadandRequest(payloadPath)
		if err != nil {
			t.Errorf("Error loading payload and request from %s: %v", payloadPath, err)
			continue
		}

		uniqueResponses := make(map[string][]string)
		responseExamples := make([]*executor.ProcessBatchResponseV2, 0)

		for _, e := range executors {
			success, resp, err := e.VerifyTest(payload, request)
			if err != nil {
				t.Errorf("%s: Error verifying payload %s: %v", e.grpcUrl, payloadPath, err)
			}
			if !success {
				t.Errorf("%s: Payload %s verification failed", e.grpcUrl, payloadPath)
			}

			matched := false
			for _, exampleResp := range responseExamples {
				if responsesAreEqual(t, exampleResp, resp) {
					responseJson, _ := json.MarshalIndent(resp, "", "  ")
					jsonResponse := string(responseJson)
					uniqueResponses[jsonResponse] = append(uniqueResponses[jsonResponse], e.grpcUrl)
					matched = true
					break
				}
			}

			if !matched {
				responseExamples = append(responseExamples, resp)
				responseJson, _ := json.MarshalIndent(resp, "", "  ")
				jsonResponse := string(responseJson)
				uniqueResponses[jsonResponse] = []string{e.grpcUrl}
			}
		}

		outDir := fmt.Sprintf("%s/outputs-batch-%d-%d", dumpDir, extractBatchNumber(fileInfo.Name()), time.Now().UnixMilli())
		if err := os.MkdirAll(outDir, 0755); err != nil {
			t.Errorf("Failed to create output directory: %v", err)
		}

		count := 0
		for responseJson, urls := range uniqueResponses {
			if len(urls) > 1 {
				t.Logf("Executors with matching responses: %v\n", urls)
			} else {
				count++
				mismatchFileName := fmt.Sprintf("%s/batch_%d_server_%s_mismatch-%d.json", outDir, extractBatchNumber(fileInfo.Name()), urls[0], count)
				if err := os.WriteFile(mismatchFileName, []byte(responseJson), 0644); err != nil {
					t.Errorf("Failed to write mismatched response to file: %v", err)
				}
				t.Errorf("Mismatched response written to %s for executor(s): %v", mismatchFileName, urls)
			}
		}
	}
}

func extractBatchNumber(filename string) int {
	parts := strings.Split(filename, "-")
	for _, part := range parts {
		if strings.HasPrefix(part, "batch_") {
			batchStr := strings.TrimPrefix(part, "batch_")
			batchNum, err := strconv.Atoi(batchStr)
			if err == nil {
				return batchNum
			}
			break
		}
	}
	return 0
}

func loadPayloadandRequest(filePath string) (*Payload, *VerifierRequest, error) {
	payloadData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read file: %w", err)
	}
	var payload Payload
	if err := json.Unmarshal(payloadData, &payload); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	requestData, err := os.ReadFile(strings.Replace(filePath, "-payload.json", "-request.json", 1))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read file: %w", err)
	}

	var request VerifierRequest
	if err := json.Unmarshal(requestData, &request); err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal request: %w", err)
	}

	return &payload, &request, nil
}

func responsesAreEqual(t *testing.T, resp1, resp2 *executor.ProcessBatchResponseV2) bool {
	resp1Copy := *resp1
	resp2Copy := *resp2

	resp1Copy.FlushId, resp2Copy.FlushId = 0, 0
	resp1Copy.ProverId, resp2Copy.ProverId = "", ""
	resp1Copy.CntPoseidonHashes, resp2Copy.CntPoseidonHashes = 0, 0
	resp1Copy.StoredFlushId, resp2Copy.StoredFlushId = 0, 0

	o := cmpopts.IgnoreUnexported(
		executor.ProcessBatchResponseV2{},
		executor.ProcessBlockResponseV2{},
		executor.ProcessTransactionResponseV2{},
		executor.InfoReadWriteV2{},
	)
	diff := cmp.Diff(resp1Copy, resp2Copy, o)
	if diff != "" {
		t.Errorf("Objects differ: %v", diff)
		return false
	}

	return true
}
