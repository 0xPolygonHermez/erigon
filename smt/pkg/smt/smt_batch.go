package smt

import (
	"sort"

	"github.com/ledgerwatch/erigon/smt/pkg/utils"
)

type dataHolder struct {
	nodeKey       *utils.NodeKey
	nodeValue     *utils.NodeValue8
	nodeValueHash *[4]uint64
}

func (s *SMT) insertBatch(nodeKeys []*utils.NodeKey, nodeValues []*utils.NodeValue8, nodeValuesHashes []*[4]uint64, oldRoot *utils.NodeKey) (*SMTResponse, error) {
	s.clearUpMutex.Lock()
	defer s.clearUpMutex.Unlock()

	size := len(nodeKeys)
	dataHolders := make([]*dataHolder, size)
	for i := 0; i < size; i++ {
		var nodeValue *utils.NodeValue8
		var nodeValueHash *[4]uint64

		if nodeValues != nil {
			nodeValue = nodeValues[i]
		}
		if nodeValuesHashes != nil {
			nodeValueHash = nodeValuesHashes[i]
		}
		dataHolders[i] = &dataHolder{
			nodeKey:       nodeKeys[i],
			nodeValue:     nodeValue,
			nodeValueHash: nodeValueHash,
		}
	}

	sortNodeKeysBitwiseAsc(dataHolders)

	for _, dataHolder := range dataHolders {
		_ = dataHolder.nodeKey.GetPath()

		// find the node

		// insert, update, delete

		// calculate left part of the tree and free its memory usage
	}

	smtResponse := &SMTResponse{
		Mode: "not run",
	}

	return smtResponse, nil
}

func sortNodeKeysBitwiseAsc(dataHolders []*dataHolder) {
	sort.Slice(dataHolders, func(i, j int) bool {
		aTmp := dataHolders[i].nodeKey
		bTmp := dataHolders[j].nodeKey

		for l := 0; l < 64; l++ {
			for n := 0; n < 4; n++ {
				aBit := aTmp[n] & 1
				bBit := bTmp[n] & 1

				aTmp[n] >>= 1 // Right shift the current part
				bTmp[n] >>= 1 // Right shift the current part

				if aBit != bBit {
					return aBit < bBit
				}
			}
		}

		return true
	})
}

/*
NodeKey -> Value
NodeKey { Hash(rKey, Hash(Value)) } -> rKey, Hash(Value), 1
NodeKey { Hash(LeftNodeKey, RightNodeKey) } -> LeftNodeKey, RightNodeKey, 0
*/
