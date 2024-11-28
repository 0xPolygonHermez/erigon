package smt

import (
	"context"
	"fmt"
	"math/big"
	"reflect"
	"strings"

	libcommon "github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon/smt/pkg/utils"
	"github.com/ledgerwatch/erigon/turbo/trie"
	"github.com/status-im/keycard-go/hexutils"
)

// BuildWitness creates a witness from the SMT
func BuildWitness(s *SMT, rd trie.RetainDecider, ctx context.Context) (*trie.Witness, error) {
	operands := make([]trie.WitnessOperator, 0)

	root, err := s.Db.GetLastRoot()
	if err != nil {
		return nil, err
	}

	action := func(prefix []byte, k utils.NodeKey, v utils.NodeValue12) (bool, error) {
		if rd != nil {
			/*
				This function is invoked for every node in the tree. We must decide whether to retain it or not in the witness.
				Retaining means adding node's data. Not retaining means adding only its hash.
				If a path (or rather a prefix of a path) must be retained then rd.Retain(prefix of a path) returns true. Otherwise it returns false.
				If a node (leaf or not) must be retained then rd.Retain returns true for all of the prefixes of its path => rd.Retains returns true for all node's ancestors all the way up to the root. (1)
				Therefore if a leaf must be retained then rd.Retain will return true not only for leaf's path but it will return true for all leaf's ancestors' paths.
				If a node must be retained it could be either because of a modification or because of a deletion.
				In case of modification it is enough to retain only the node but in case of deletion the witness must includes the node's sibling.
				Because of (1) if a node must be retained then its parent must be retained too => we're safe to decide whether a node must be retained or not by using its parent's parent.
				Using a parent's path will ensure that if a node is included in the witness then its sibling will also be included, because they have the same parent.
				As a result, if node must be included in the witness the code will always include its sibling.
				Using this approach we prepare the witness like we're going to deleting all node, because we actually do not have information whether a node is modified or deleted.
				This algorithm adds a little bit more nodes to the witness but it ensures that all requiring nodes are included.
			*/

			var retain bool

			prefixLen := len(prefix)
			if prefixLen > 0 {
				retain = rd.Retain(prefix[:prefixLen-1])
			} else {
				retain = rd.Retain(prefix)
			}

			if !retain {
				h := libcommon.BigToHash(k.ToBigInt())
				hNode := trie.OperatorHash{Hash: h}
				operands = append(operands, &hNode)
				return false, nil
			}
		}

		if v.IsFinalNode() {
			actualK, err := s.Db.GetHashKey(k)
			if err != nil {
				return false, err
			}

			keySource, err := s.Db.GetKeySource(actualK)
			if err != nil {
				return false, err
			}

			t, addr, storage, err := utils.DecodeKeySource(keySource)
			if err != nil {
				return false, err
			}

			valHash := v.Get4to8()
			v, err := s.Db.Get(*valHash)
			if err != nil {
				return false, err
			}

			vInBytes := utils.ArrayBigToScalar(utils.BigIntArrayFromNodeValue8(v.GetNodeValue8())).Bytes()
			if t == utils.SC_CODE {
				code, err := s.Db.GetCode(vInBytes)
				if err != nil {
					return false, err
				}

				operands = append(operands, &trie.OperatorCode{Code: code})
			}

			// fmt.Printf("Node hash: %s, Node type: %d, address %x, storage %x, value %x\n", utils.ConvertBigIntToHex(k.ToBigInt()), t, addr, storage, utils.ArrayBigToScalar(value8).Bytes())
			operands = append(operands, &trie.OperatorSMTLeafValue{
				NodeType:   uint8(t),
				Address:    addr.Bytes(),
				StorageKey: storage.Bytes(),
				Value:      vInBytes,
			})
			return false, nil
		}

		var mask uint32
		if !v.Get0to4().IsZero() {
			mask |= 1
		}

		if !v.Get4to8().IsZero() {
			mask |= 2
		}

		operands = append(operands, &trie.OperatorBranch{
			Mask: mask,
		})

		return true, nil
	}

	err = s.Traverse(ctx, root, action)

	return trie.NewWitness(operands), err
}

// BuildSMTFromWitness builds SMT from witness
func BuildSMTFromWitness(w *trie.Witness) (*SMT, error) {
	// using memdb
	s := NewSMT(nil, false)

	balanceMap := make(map[string]*big.Int)
	nonceMap := make(map[string]*big.Int)
	contractMap := make(map[string]string)
	storageMap := make(map[string]map[string]string)

	path := make([]int, 0)

	firstNode := true
	nodeChildCountMap := make(map[string]uint32)
	nodesBranchValueMap := make(map[string]uint32)

	type nodeHash struct {
		path []int
		hash libcommon.Hash
	}

	nodeHashes := make([]nodeHash, 0)

	for i, operator := range w.Operators {
		switch op := operator.(type) {
		case *trie.OperatorSMTLeafValue:
			valScaler := new(big.Int).SetBytes(op.Value)
			hexAddr := libcommon.BytesToAddress(op.Address).String()

			switch op.NodeType {
			case utils.KEY_BALANCE:
				balanceMap[hexAddr] = valScaler

			case utils.KEY_NONCE:
				nonceMap[hexAddr] = valScaler

			case utils.SC_STORAGE:
				if _, ok := storageMap[hexAddr]; !ok {
					storageMap[hexAddr] = make(map[string]string)
				}

				stKey := hexutils.BytesToHex(op.StorageKey)
				stKey = fmt.Sprintf("0x%s", stKey)

				storageMap[hexAddr][stKey] = valScaler.String()
			}

			path = path[:len(path)-1]
			nodePathAsString := intArrayToString(path)
			nodeChildCountMap[nodePathAsString] += 1

			for len(path) != 0 && nodeChildCountMap[nodePathAsString] == nodesBranchValueMap[nodePathAsString] {
				path = path[:len(path)-1]
				nodePathAsString = intArrayToString(path)
			}

			if nodeChildCountMap[nodePathAsString] < nodesBranchValueMap[nodePathAsString] {
				path = append(path, 1)
			}

		case *trie.OperatorCode:
			smtLeafValueOp, ok := w.Operators[i+1].(*trie.OperatorSMTLeafValue)
			if !ok {
				return nil, fmt.Errorf("expected %T, but found %T witness operator", (*trie.OperatorSMTLeafValue)(nil), reflect.TypeOf(smtLeafValueOp))
			}

			hexAddr := libcommon.BytesToAddress(smtLeafValueOp.Address).String()
			code := hexutils.BytesToHex(op.Code)
			if len(code) > 0 {
				if err := s.Db.AddCode(op.Code); err != nil {
					return nil, err
				}
				code = fmt.Sprintf("0x%s", code)
			}

			contractMap[hexAddr] = code

		case *trie.OperatorBranch:
			if firstNode {
				firstNode = false
			} else {
				nodeChildCountMap[intArrayToString(path[:len(path)-1])] += 1
			}

			switch op.Mask {
			case 1:
				nodesBranchValueMap[intArrayToString(path)] = 1
				path = append(path, 0)
			case 2:
				nodesBranchValueMap[intArrayToString(path)] = 1
				path = append(path, 1)
			case 3:
				nodesBranchValueMap[intArrayToString(path)] = 2
				path = append(path, 0)
			}

		case *trie.OperatorHash:
			pathCopy := make([]int, len(path))
			copy(pathCopy, path)
			nodeHashes = append(nodeHashes, nodeHash{path: pathCopy, hash: op.Hash})

			path = path[:len(path)-1]
			nodeChildCountMap[intArrayToString(path)] += 1

			for len(path) != 0 && nodeChildCountMap[intArrayToString(path)] == nodesBranchValueMap[intArrayToString(path)] {
				path = path[:len(path)-1]
			}
			if nodeChildCountMap[intArrayToString(path)] < nodesBranchValueMap[intArrayToString(path)] {
				path = append(path, 1)
			}

		default:
			// Unsupported operator type
			return nil, fmt.Errorf("unsupported operator type: %T", op)
		}
	}

	for _, nodeHash := range nodeHashes {
		_, err := s.InsertHashNode(nodeHash.path, nodeHash.hash.Big())
		if err != nil {
			return nil, err
		}

		_, err = s.Db.GetLastRoot()
		if err != nil {
			return nil, err
		}
	}

	for addr, balance := range balanceMap {
		_, err := s.SetAccountBalance(addr, balance)
		if err != nil {
			return nil, err
		}
	}

	for addr, nonce := range nonceMap {
		_, err := s.SetAccountNonce(addr, nonce)
		if err != nil {
			return nil, err
		}
	}

	for addr, code := range contractMap {
		err := s.SetContractBytecode(addr, code)
		if err != nil {
			return nil, err
		}
	}

	for addr, storage := range storageMap {
		_, err := s.SetContractStorage(addr, storage, nil)
		if err != nil {
			return nil, err
		}
	}

	return s, nil
}

func intArrayToString(a []int) string {
	var s strings.Builder
	for _, v := range a {
		s.WriteString(fmt.Sprintf("%d", v))
	}
	return s.String()
}
