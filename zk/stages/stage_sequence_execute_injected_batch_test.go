package stages

import (
	"encoding/json"
	"testing"

	"github.com/ledgerwatch/erigon-lib/chain"
	zktx "github.com/ledgerwatch/erigon/zk/tx"
	zktypes "github.com/ledgerwatch/erigon/zk/types"
	"github.com/stretchr/testify/require"
)

func TestUnmarshalInjectedJSONData(t *testing.T) {
	injectedBatchDataJSON := []byte(`
	{
        "batchL2Data": "0xf9010380808401c9c38094af97e3fe01decff90f26d266668be9f49d8df0d880b8e4f811bff7000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000a40d5f56745a118d0906a34e69aec8c0db1cb8fa000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000c0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000005ca1ab1e0000000000000000000000000000000000000000000000000000000005ca1ab1e1bff",
        "globalExitRoot": "0xad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5",
        "timestamp": 1728653072,
        "sequencer": "0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266",
        "l1BlockNumber": 68,
        "l1BlockHash": "0x37dc5f24c65fe9d013953b9b13075293c1dc8a55bc6c0d2116cc4047f90c2068",
        "l1ParentHash": "0xec78665685fe91d9445f5f366b7f4473b6aa6a2b7a54cbbf52b52e6db9f1b317"
    }`)

	var injectedBatchData zktypes.L1InjectedBatch
	err := json.Unmarshal(injectedBatchDataJSON, &injectedBatchData)
	require.NoError(t, err)

	decodedL2Data, err := zktx.DecodeBatchL2Blocks([]byte(injectedBatchData.Transaction), uint64(chain.ForkID12Banana))
	require.NoError(t, err)
	require.NotNil(t, decodedL2Data)
}
