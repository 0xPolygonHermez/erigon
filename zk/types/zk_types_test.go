package types

import (
	"encoding/hex"
	"encoding/json"
	"testing"

	libcommon "github.com/ledgerwatch/erigon-lib/common"
	"github.com/stretchr/testify/require"
)

func Test_L1InfoTreeMarshallUnmarshall(t *testing.T) {
	ger := libcommon.HexToHash("0x1")
	mainnet := libcommon.HexToHash("0x2")
	rollup := libcommon.HexToHash("0x3")
	parent := libcommon.HexToHash("0x4")
	timestamp := uint64(1000)
	in := L1InfoTreeUpdate{
		Index:           1,
		GER:             ger,
		MainnetExitRoot: mainnet,
		RollupExitRoot:  rollup,
		ParentHash:      parent,
		Timestamp:       timestamp,
	}
	marshalled := in.Marshall()

	result := L1InfoTreeUpdate{}
	result.Unmarshall(marshalled)

	require.Equal(t, in, result)
}

func Test_L1InjectedBatchMarshallUnmarshall(t *testing.T) {
	input := &L1InjectedBatch{
		L1BlockNumber:      1,
		Timestamp:          1000,
		L1BlockHash:        libcommon.HexToHash("0x1"),
		L1ParentHash:       libcommon.HexToHash("0x2"),
		LastGlobalExitRoot: libcommon.HexToHash("0x3"),
		Sequencer:          libcommon.HexToAddress("0x4"),
		Transaction:        []byte{100},
	}

	marshalled := input.Marshall()

	result := &L1InjectedBatch{}
	err := result.Unmarshall(marshalled)
	if err != nil {
		t.Fatal(err)
	}

	require.Equal(t, input, result)
}

func Test_L1InjectedBatch_UnmarshalJSON(t *testing.T) {
	txData, err := hex.DecodeString("f9010380808401c9c38094af97e3fe01decff90f26d266668be9f49d8df0d880b8e4f811bff7000000000000000000000000000000000000000000000000000000000000000100000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000a40d5f56745a118d0906a34e69aec8c0db1cb8fa000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000c0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000005ca1ab1e0000000000000000000000000000000000000000000000000000000005ca1ab1e1bff")
	require.NoError(t, err)

	l1BlockHash, err := hex.DecodeString("37dc5f24c65fe9d013953b9b13075293c1dc8a55bc6c0d2116cc4047f90c2068")
	require.NoError(t, err)

	l1ParentHash, err := hex.DecodeString("ec78665685fe91d9445f5f366b7f4473b6aa6a2b7a54cbbf52b52e6db9f1b317")
	require.NoError(t, err)

	ger, err := hex.DecodeString("ad3228b676f7d3cd4284a5443f17f1962b36e491b30a40b2405849e597ba5fb5")
	require.NoError(t, err)

	expectedInjectedBatch := &L1InjectedBatch{
		L1BlockNumber:      68,
		Timestamp:          1728653072,
		L1BlockHash:        libcommon.BytesToHash(l1BlockHash),
		L1ParentHash:       libcommon.BytesToHash(l1ParentHash),
		Sequencer:          libcommon.HexToAddress("0xf39Fd6e51aad88F6F4ce6aB8827279cffFb92266"),
		LastGlobalExitRoot: libcommon.BytesToHash(ger),
		Transaction:        txData,
	}

	injectedBatchDataJSON, err := json.Marshal(expectedInjectedBatch)
	require.NoError(t, err)

	injectedBatch := &L1InjectedBatch{}
	err = injectedBatch.UnmarshalJSON(injectedBatchDataJSON)
	require.NoError(t, err)
	require.Equal(t, expectedInjectedBatch, injectedBatch)
}
