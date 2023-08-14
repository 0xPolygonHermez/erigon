package smt

import (
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
	"testing"

	"context"
	"github.com/ledgerwatch/erigon-lib/kv/mdbx"
	db2 "github.com/ledgerwatch/erigon/smt/pkg/db"
)

func TestGenesisMdbx(t *testing.T) {
	runGenesisTestMdbx(t, "testdata/mainnet-genesis.json")
}

func TestIncrementalSMT(t *testing.T) {
	// run one of the existing tests

	// add some data to the SMT

	// verify the root updated correctly

	// the best way to verify the root is to run the in mem db, and then the MDBX db and compare the roots when adding the same data
}

func BenchmarkGenesisMdbx(b *testing.B) {
	for i := 0; i < b.N; i++ {
		runGenesisTestMdbx(b, "testdata/mainnet-genesis.json")
	}
}

func runGenesisTestMdbx(tb testing.TB, filename string) {

	data, err := os.ReadFile(filename)
	if err != nil {
		tb.Fatal("Failed to open file: ", err)
	}

	var genesis Genesis
	err = json.Unmarshal(data, &genesis)
	if err != nil {
		tb.Fatal("Failed to parse json: ", err)
	}

	dbi, err := mdbx.NewTemporaryMdbx()
	tx, err := dbi.BeginRw(context.Background())
	if err != nil {
		tb.Fatal("Failed to open db: ", err)
	}
	sdb := db2.NewEriDb(tx)

	smt := NewSMT(sdb)

	for _, addr := range genesis.Genesis {
		fmt.Println(addr.ContractName)
		bal, _ := new(big.Int).SetString(addr.Balance, 10)
		non, _ := new(big.Int).SetString(addr.Nonce, 10)
		// add balance and nonce
		_, _ = smt.SetAccountState(addr.Address, bal, non)
		// add bytecode if defined
		if addr.Bytecode != "" {
			_ = smt.SetContractBytecode(addr.Address, addr.Bytecode)
		}
		// add storage if defined
		if len(addr.Storage) > 0 {
			_, _ = smt.SetContractStorage(addr.Address, addr.Storage)
		}
	}

	base := 10
	root := genesis.Root
	if strings.HasPrefix(root, "0x") {
		root = root[2:]
		base = 16
	}

	smt.CheckOrphanedNodes(context.Background())

	//smt.PrintTree()

	expected, _ := new(big.Int).SetString(root, base)
	fmt.Println("Expected root: ", genesis.Root)
	fmt.Println("Actual root: ", fmt.Sprintf("0x%x", smt.LastRoot()))

	if expected.Cmp(smt.LastRoot()) != 0 {
		tb.Errorf("Expected root does not match. Got 0x%x, want %s", smt.LastRoot(), root)
	}
}

func Test_Data_Mdbx(t *testing.T) {
	runTestVectorsMdbx(t, "testdata/data.json")
	runTestVectorsMdbx(t, "testdata/data2.json")
}

func runTestVectorsMdbx(t *testing.T, filename string) {
	// load test data from disk
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal("Failed to open file: ", err)
	}

	var testCases []TestCase
	err = json.Unmarshal(data, &testCases)
	if err != nil {
		t.Fatal("Failed to parse json: ", err)
	}

	for k, tc := range testCases {
		t.Run(strconv.Itoa(k), func(t *testing.T) {
			smt := NewSMT(nil)
			for _, addr := range tc.Addresses {

				bal, _ := new(big.Int).SetString(addr.Balance, 10)
				non, _ := new(big.Int).SetString(addr.Nonce, 10)
				// add balance and nonce
				_, _ = smt.SetAccountState(addr.Address, bal, non)
				// add bytecode if defined
				if addr.Bytecode != "" {
					_ = smt.SetContractBytecode(addr.Address, addr.Bytecode)
				}
				// add storage if defined
				if len(addr.Storage) > 0 {
					_, _ = smt.SetContractStorage(addr.Address, addr.Storage)
				}
			}

			base := 10
			if strings.HasPrefix(tc.ExpectedRoot, "0x") {
				tc.ExpectedRoot = tc.ExpectedRoot[2:]
				base = 16
			}

			expected, _ := new(big.Int).SetString(tc.ExpectedRoot, base)

			if expected.Cmp(smt.LastRoot()) != 0 {
				t.Errorf("Expected root does not match. Got 0x%x, want 0x%x", smt.LastRoot(), expected)
			}
		})
	}
}
