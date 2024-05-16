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
	libcommon "github.com/gateway-fm/cdk-erigon-lib/common"
	"github.com/ledgerwatch/erigon/core"
	erigonchain "github.com/gateway-fm/cdk-erigon-lib/chain"
	"github.com/ledgerwatch/erigon/core/rawdb"
	"github.com/gateway-fm/cdk-erigon-lib/kv"
	"github.com/ledgerwatch/erigon/chain"
	"github.com/ledgerwatch/erigon/core/types"
	"context"
	"github.com/ledgerwatch/erigon/zk/witness"
	"github.com/ledgerwatch/erigon/node"
	"github.com/ledgerwatch/erigon/core/vm/stack"
)

func TestVerifyBatchesFromNode(t *testing.T) {

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
		executors[i] = NewExecutor(url, 5000*time.Millisecond, false, 1)
	}
	var legacyExecutors []ILegacyExecutor
	for _, e := range executors {
		legacyExecutors = append(legacyExecutors, e)
	}

	// chainKv
	chainKv, err := node.OpenDatabase(stack.Config(), kv.ChainDB)
	if err != nil {
		return nil, err
	}

	// chain config
	var chainConfig *chain.Config
	var genesis *types.Block
	if err := chainKv.Update(context.Background(), func(tx kv.RwTx) error {
		h, err := rawdb.ReadCanonicalHash(tx, 0)
		if err != nil {
			panic(err)
		}
		genesisSpec := config.Genesis
		if h != (libcommon.Hash{}) { // fallback to db content
			genesisSpec = nil
		}
		var genesisErr error
		chainConfig, genesis, genesisErr = core.WriteGenesisBlock(tx, genesisSpec, config.OverrideShanghaiTime, tmpdir)
		if _, ok := genesisErr.(*erigonchain.ConfigCompatError); genesisErr != nil && !ok {
			return genesisErr
		}

		currentBlock = rawdb.ReadCurrentBlock(tx)
		return nil
	}); err != nil {
		panic(err)
	}

	// chain db

	// witness generator
	witnessGenerator := witness.NewGenerator(
		config.Dirs,
		config.HistoryV3,
		backend.agg,
		backend.blockReader,
		backend.chainConfig,
		backend.engine,
	)

	verifier := NewLegacyExecutorVerifier(
		*cfg.Zk,
		executors,
		backend.chainConfig,
		backend.chainDB,
		witnessGenerator,
		backend.l1Syncer,
		backend.dataStream,
	)

	// load the chainconfig
	// load the zk config

	// we should get a list of executors, and wire up the verifiers, and then generate the payloads from the node to send!
}

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
		executors[i] = NewExecutor(url, 5000*time.Millisecond, false, 1)
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
