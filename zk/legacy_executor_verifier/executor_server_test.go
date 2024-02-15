package legacy_executor_verifier

import (
	"github.com/ledgerwatch/erigon-lib/common"
	"testing"
	"time"
)

func TestExecutor_Verify_Debug(t *testing.T) {
	scenarios := map[string]struct {
		executorUrl       string
		gprcConnTimeout   time.Duration
		payload           *Payload
		expectedStateRoot *common.Hash
	}{
		"Scenario1": {
			executorUrl:     "51.210.116.237:50071", // real test executor URL
			gprcConnTimeout: 5 * time.Second,
			payload:         &Payload{
				// todo: make a payload
			},
			expectedStateRoot: &common.Hash{},
		},
	}

	for name, scenario := range scenarios {
		t.Run(name, func(t *testing.T) {
			executor, err := NewExecutor(scenario.executorUrl, scenario.gprcConnTimeout)
			if err != nil {
				t.Fatalf("Failed to create executor: %v", err)
			}
			defer executor.Close()

			verified, err := executor.Verify(scenario.payload, scenario.expectedStateRoot)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !verified {
				t.Errorf("Expected true result, got %v", verified)
			}
		})
	}
}

func TestExecutor_SlowExecutor_Timeout(t *testing.T) {
	// this test exists only to make sure timeout is implemented, not to test the actual timeout duration
	executor, err := NewExecutor("http://localhost:8080", 100*time.Millisecond) // use non-responding url
	if err == nil {
		t.Fatalf("Expected error, got nil")
	}
	if err.Error() != "failed to dial grpc: context deadline exceeded" {
		t.Errorf("Unexpected error: %v", err)
	}

	executor.Close()
}
