package smt

import (
	"fmt"
	"math/big"
	"testing"
)

// Test vectors from JS implementation: https://github.com/0xPolygonHermez/zkevm-testvectors/blob/main/merkle-tree/smt-raw.json
// Test 'want' values come from running the JS tests with debugger attached

func TestSMT_SingleInsert(t *testing.T) {
	scenarios := []struct {
		name     string
		oldRoot  *big.Int
		k        *big.Int
		v        *big.Int
		expected string
	}{
		{
			name:     "TestSMT_Insert_0Key_0Value",
			oldRoot:  big.NewInt(0),
			k:        big.NewInt(0),
			v:        big.NewInt(0),
			expected: "0x0",
		},
		{
			name:     "TestSMT_Insert_0Key_1Value",
			oldRoot:  big.NewInt(0),
			k:        big.NewInt(0),
			v:        big.NewInt(1),
			expected: "0x42bb2f66296df03552203ae337815976ca9c1bf52cc1bdd59399ede8fea8a822",
		},
		{
			name:     "TestSMT_Insert1Key_XValue",
			oldRoot:  big.NewInt(0),
			k:        big.NewInt(1),
			v:        new(big.Int).SetUint64(1),
			expected: "0xb26e0de762d186d2efc35d9ff4388def6c96ec15f942d83d779141386fe1d2e1",
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			s := NewSMT(nil)
			newRoot, err := s.InsertBI(scenario.oldRoot, scenario.k, scenario.v)
			if err != nil {
				t.Errorf("Insert failed: %v", err)
			}
			hex := fmt.Sprintf("0x%0x", newRoot.NewRoot)
			printNode(newRoot)
			if hex != scenario.expected {
				t.Errorf("root hash is not as expected, got %v wanted %v", hex, scenario.expected)
			}
		})
	}
}

func TestSMT_MultipleInsert(t *testing.T) {
	s := NewSMT(nil)
	testCases := []struct {
		root  *big.Int
		key   *big.Int
		value *big.Int
		want  string
		mode  string
	}{
		{
			big.NewInt(0),
			big.NewInt(1),
			big.NewInt(1),
			"0xb26e0de762d186d2efc35d9ff4388def6c96ec15f942d83d779141386fe1d2e1",
			"insertNotFound",
		},
		{
			nil,
			big.NewInt(2),
			big.NewInt(2),
			"0xa399847134a9987c648deabc85a7310fbe854315cbeb6dc3a7efa1a4fa2a2c86",
			"insertFound",
		},
		{
			nil,
			big.NewInt(3),
			big.NewInt(3),
			"0xb5a4b1b7a8c3a7c11becc339bbd7f639229cd14f14f76ee3a0e9170074399da4",
			"insertFound",
		},
	}

	var root *big.Int
	for i, testCase := range testCases {
		if i > 0 {
			testCase.root = root
		}
		r, err := s.InsertBI(testCase.root, testCase.key, testCase.value)
		if err != nil {
			t.Errorf("Test case %d: Insert failed: %v", i, err)
			continue
		}

		got := toHex(r.NewRoot)
		if got != testCase.want {
			t.Errorf("Test case %d: Root hash is not as expected, got %v, want %v", i, got, testCase.want)
		}
		if testCase.mode != r.Mode {
			t.Errorf("Test case %d: Mode is not as expected, got %v, want %v", i, r.Mode, testCase.mode)
		}

		root = r.NewRoot
	}
}

func TestSMT_UpdateElement1(t *testing.T) {
	s := NewSMT(nil)
	testCases := []struct {
		root  *big.Int
		key   *big.Int
		value *big.Int
		want  string
		mode  string
	}{
		{
			big.NewInt(0),
			big.NewInt(1),
			big.NewInt(2),
			"0x7212762089bfe2505ebbd8f1696acb835ecaf394d0f8d191e4c026dab9ddcfa5",
			"insertNotFound",
		},
		{big.NewInt(0),
			big.NewInt(1),
			big.NewInt(3),
			"0x0f740b94e3935291daf0998666160414f14a93bb7be05ad56df4df21ff817c1d",
			"update",
		},
		{big.NewInt(0),
			big.NewInt(1),
			big.NewInt(2),
			"0x7212762089bfe2505ebbd8f1696acb835ecaf394d0f8d191e4c026dab9ddcfa5",
			"update",
		},
	}

	// set up the first root as that from the first testCase
	r := &SMTResponse{
		NewRoot: testCases[0].root,
	}
	var err error

	for i, testCase := range testCases {
		r, err = s.InsertBI(r.NewRoot, testCase.key, testCase.value)
		if err != nil {
			t.Errorf("Test case %d: Insert failed: %v", i, err)
			continue
		}

		got := toHex(r.NewRoot)
		if got != testCase.want {
			t.Errorf("Test case %d: Root hash is not as expected, got %v, want %v", i, got, testCase.want)
		}
		if testCase.mode != r.Mode {
			t.Errorf("Test case %d: Mode is not as expected, got %v, want %v", i, r.Mode, testCase.mode)
		}
	}
}

func TestSMT_AddSharedElement2(t *testing.T) {
	s := NewSMT(nil)

	r1, err := s.InsertBI(big.NewInt(0), big.NewInt(8), big.NewInt(2))
	if err != nil {
		t.Errorf("Insert failed: %v", err)
	}
	printNode(r1)
	r2, err := s.InsertBI(r1.NewRoot, big.NewInt(9), big.NewInt(3))
	if err != nil {
		t.Errorf("Insert failed: %v", err)
	}
	printNode(r2)
	r3, err := s.InsertBI(r2.NewRoot, big.NewInt(8), big.NewInt(0))
	if err != nil {
		t.Errorf("Insert failed: %v", err)
	}
	printNode(r3)
	r4, err := s.InsertBI(r3.NewRoot, big.NewInt(9), big.NewInt(0))
	if err != nil {
		t.Errorf("Insert failed: %v", err)
	}
	printNode(r4)

}

func printNode(n *SMTResponse) {
	fmt.Printf(fmt.Sprintf("Root: %s Mode: %s\n", toHex(n.NewRoot), n.Mode))
}

func toHex(i *big.Int) string {
	return fmt.Sprintf("0x%064x", i)
}
