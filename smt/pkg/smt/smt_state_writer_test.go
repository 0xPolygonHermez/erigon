package smt

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	libcommon "github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/kv/memdb"
	"github.com/ledgerwatch/erigon/core/state"
	"github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/turbo/trie"
	"github.com/stretchr/testify/require"
)

func TestSMTApplyTraces(t *testing.T) {
	type testData struct {
		IsFullWitness bool                                  `json:"isFullWitness"`
		Witness       string                                `json:"witness"`
		StateRoot     libcommon.Hash                        `json:"stateRoot"`
		Traces        map[libcommon.Address]*types.TxnTrace `json:"traces,omitempty"`
	}

	cases := []struct {
		name string
		file string
	}{
		{
			name: "Single EOA tx full witness",
			file: "./testdata/zerotraces/full-witness-single-eoa-tx.json",
		},
		{
			name: "Empty block full witness",
			file: "./testdata/zerotraces/full-witness-empty-block.json",
		},
		// {
		// 	name: "SC deployment full witness",
		// 	file: "./testdata/zerotraces/full-witness-contract-deployment.json",
		// },
		// {
		// 	name: "SC interaction full witness",
		// 	file: "./testdata/zerotraces/full-witness-contract-interaction.json",
		// },
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rawTestData, err := os.ReadFile(c.file)
			require.NoError(t, err)

			var td testData
			err = json.Unmarshal(rawTestData, &td)
			require.NoError(t, err)

			decodedWitnessRaw, err := hex.DecodeString(strings.TrimPrefix(td.Witness, "0x"))
			require.NoError(t, err)

			witness, err := trie.NewWitnessFromReader(bytes.NewReader(decodedWitnessRaw), false)
			require.NoError(t, err)

			smt, err := BuildSMTFromWitness(witness)
			require.NoError(t, err)

			var rd trie.RetainDecider
			if td.IsFullWitness {
				rd = &trie.AlwaysTrueRetainDecider{}
			} else {
				_, tx := memdb.NewTestTx(t)
				tds := state.NewTrieDbState(td.StateRoot, tx, 0, state.NewPlainStateReader(tx))
				tds.StartNewBuffer()

				rd, err = tds.ResolveSMTRetainList(nil)
				require.NoError(t, err)
			}

			newSMT, err := smt.ApplyTraces(td.Traces, rd)
			require.NoError(t, err)

			require.Equal(t, td.StateRoot, libcommon.BigToHash(newSMT.LastRoot()))
		})
	}
}
