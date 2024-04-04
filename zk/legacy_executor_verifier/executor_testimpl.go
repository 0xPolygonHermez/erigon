package legacy_executor_verifier

import (
	"context"
	"fmt"
	"github.com/gateway-fm/cdk-erigon-lib/common"
	"github.com/ledgerwatch/erigon/zk/legacy_executor_verifier/proto/github.com/0xPolygonHermez/zkevm-node/state/runtime/executor"
	"github.com/ledgerwatch/log/v3"
	"google.golang.org/grpc"
	"time"
	"bytes"
)

func (e *Executor) VerifyTest(p *Payload, request *VerifierRequest) (bool, *executor.ProcessBatchResponseV2, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	log.Debug("Sending request to grpc server", "grpcUrl", e.grpcUrl)

	size := 1024 * 1024 * 256 // 256mb maximum size - hack for now until trimmed witness is proved off
	resp, err := e.client.ProcessStatelessBatchV2(ctx, &executor.ProcessStatelessBatchRequestV2{
		Witness:           p.Witness,
		DataStream:        p.DataStream,
		Coinbase:          p.Coinbase,
		OldAccInputHash:   p.OldAccInputHash,
		L1InfoRoot:        p.L1InfoRoot,
		TimestampLimit:    p.TimestampLimit,
		ForcedBlockhashL1: p.ForcedBlockhashL1,
		ContextId:         p.ContextId,
		//TraceConfig: &executor.TraceConfigV2{
		//	DisableStorage:            0,
		//	DisableStack:              0,
		//	EnableMemory:              0,
		//	EnableReturnData:          0,
		//	TxHashToGenerateFullTrace: nil,
		//},
	}, grpc.MaxCallSendMsgSize(size), grpc.MaxCallRecvMsgSize(size))
	if err != nil {
		return false, nil, fmt.Errorf("failed to process stateless batch: %w", err)
	}

	counters := fmt.Sprintf("[SHA: %v]", resp.CntSha256Hashes)
	counters += fmt.Sprintf("[A: %v]", resp.CntArithmetics)
	counters += fmt.Sprintf("[B: %v]", resp.CntBinaries)
	counters += fmt.Sprintf("[K: %v]", resp.CntKeccakHashes)
	counters += fmt.Sprintf("[M: %v]", resp.CntMemAligns)
	counters += fmt.Sprintf("[P: %v]", resp.CntPoseidonHashes)
	counters += fmt.Sprintf("[S: %v]", resp.CntSteps)
	counters += fmt.Sprintf("[D: %v]", resp.CntPoseidonPaddings)
	log.Info("executor result", "batch", request.BatchNumber, "counters", counters, "root", common.BytesToHash(resp.NewStateRoot), "our-root", request.StateRoot)
	log.Debug("Received response from executor", "grpcUrl", e.grpcUrl, "response", resp)

	respCheck, err := responseCheckTest(resp, request.StateRoot)

	return respCheck, resp, err
}

func responseCheckTest(resp *executor.ProcessBatchResponseV2, erigonStateRoot common.Hash) (bool, error) {
	if resp == nil {
		return false, fmt.Errorf("nil response")
	}
	if resp.Error != executor.ExecutorError_EXECUTOR_ERROR_UNSPECIFIED &&
		resp.Error != executor.ExecutorError_EXECUTOR_ERROR_NO_ERROR {
		return false, fmt.Errorf("error in response: %s", resp.Error)
	}

	if !bytes.Equal(resp.NewStateRoot, erigonStateRoot.Bytes()) {
		return false, fmt.Errorf("erigon state root mismatch: expected %s, got %s", erigonStateRoot, common.BytesToHash(resp.NewStateRoot))
	}

	return true, nil
}
