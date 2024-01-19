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

	var err error

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
		// this can happen in parallel
		if nodeValueHash == nil {
			nodeValueHashObj, err := s.hashcalc(nodeValue.ToUintArray(), utils.BranchCapacity)
			if err != nil {
				return nil, err
			}
			nodeValueHash = &nodeValueHashObj
		}
		dataHolders[i] = &dataHolder{
			nodeKey:       nodeKeys[i],
			nodeValue:     nodeValue,
			nodeValueHash: nodeValueHash,
		}
	}

	sortDataHoldersBitwiseAsc(dataHolders)

	if oldRoot == nil {
		oldRootObj, err := s.getLastRoot()
		if err != nil {
			return nil, err
		}
		oldRoot = &oldRootObj
	}
	var finalRoot utils.NodeKey = *oldRoot

	for _, dataHolder := range dataHolders {
		var usedKey []int
		var level int
		var foundKey *utils.NodeKey
		var foundRKey utils.NodeKey
		var foundOldValHash utils.NodeKey

		siblings := map[int]*utils.NodeValue12{}
		refNodeKey := oldRoot

		nodeKey := dataHolder.nodeKey
		nodeValue := dataHolder.nodeValue
		nodeValueHash := dataHolder.nodeValueHash
		nodePath := dataHolder.nodeKey.GetPath()

		for !refNodeKey.IsZero() && foundKey == nil {
			sl, err := s.Db.Get(*refNodeKey)
			if err != nil {
				return nil, err
			}
			siblings[level] = &sl
			if siblings[level].IsFinalNode() {
				foundRKey = utils.NodeKeyFromBigIntArray(siblings[level][0:4])
				foundOldValHash = utils.NodeKeyFromBigIntArray(siblings[level][4:8])
				foundKey = utils.JoinKey(usedKey, foundRKey)
				if err != nil {
					return nil, err
				}
			} else {
				refNodeKeyObj := utils.NodeKeyFromBigIntArray(siblings[level][nodePath[level]*4 : nodePath[level]*4+4])
				refNodeKey = &refNodeKeyObj
				usedKey = append(usedKey, nodePath[level])
				level++
			}
		}

		level--
		if len(usedKey) != 0 {
			usedKey = usedKey[:len(usedKey)-1]
		}

		if !nodeValue.IsZero() { // we have a value - so we're updating or inserting
			if foundKey != nil {
				if foundKey.IsEqualTo(*nodeKey) {
					_, err = s.hashSave(nodeValue.ToUintArray(), utils.BranchCapacity, *nodeValueHash)
					if err != nil {
						return nil, err
					}
					newLeafHash, err := s.hashcalcAndSave(utils.ConcatArrays4(foundRKey, *nodeValueHash), utils.LeafCapacity)
					if err != nil {
						return nil, err
					}
					s.Db.InsertHashKey(newLeafHash, *nodeKey)
				} else {
					level2 := level + 1
					foundKeys := foundKey.GetPath()

					for {
						if level2 >= len(nodePath) || level2 >= len(foundKeys) {
							break
						}

						if nodePath[level2] != foundKeys[level2] {
							break
						}

						level2++
					}

					oldKey := utils.RemoveKeyBits(*foundKey, level2+1)
					oldLeafHash, err := s.hashcalcAndSave(utils.ConcatArrays4(oldKey, foundOldValHash), utils.LeafCapacity)
					s.Db.InsertHashKey(oldLeafHash, *foundKey)
					if err != nil {
						return nil, err
					}

					newKey := utils.RemoveKeyBits(*nodeKey, level2+1)

					_, err = s.hashSave(nodeValue.ToUintArray(), utils.BranchCapacity, *nodeValueHash)
					if err != nil {
						return nil, err
					}
					newLeafHash, err := s.hashcalcAndSave(utils.ConcatArrays4(newKey, *nodeValueHash), utils.LeafCapacity)
					if err != nil {
						return nil, err
					}
					s.Db.InsertHashKey(newLeafHash, *nodeKey)

					var node [8]uint64
					for i := 0; i < 8; i++ {
						node[i] = 0
					}

					for j := 0; j < 4; j++ {
						node[nodePath[level2]*4+j] = newLeafHash[j]
						node[foundKeys[level2]*4+j] = oldLeafHash[j]
					}

					r2, err := s.hashcalcAndSave(node, utils.BranchCapacity)
					if err != nil {
						return nil, err
					}
					level2 -= 1

					for level2 != level {
						for i := 0; i < 8; i++ {
							node[i] = 0
						}

						for j := 0; j < 4; j++ {
							node[nodePath[level2]*4+j] = r2[j]
						}

						r2, err = s.hashcalcAndSave(node, utils.BranchCapacity)
						if err != nil {
							return nil, err
						}
						level2 -= 1
					}
				}
			} else {
				newKey := utils.RemoveKeyBits(*nodeKey, level+1)

				_, err = s.hashSave(nodeValue.ToUintArray(), utils.BranchCapacity, *nodeValueHash)
				if err != nil {
					return nil, err
				}
				newLeafHash, err := s.hashcalcAndSave(utils.ConcatArrays4(newKey, *nodeValueHash), utils.LeafCapacity)
				if err != nil {
					return nil, err
				}
				s.Db.InsertHashKey(newLeafHash, *nodeKey)
			}
		} else if foundKey != nil && foundKey.IsEqualTo(*nodeKey) { // we don't have a value so we're deleting
			if level >= 0 {
				// for j := 0; j < 4; j++ {
				// 	siblings[level][keys[level]*4+j] = big.NewInt(0)
				// }

				uKey, err := siblings[level].IsUniqueSibling()
				if err != nil {
					return nil, err
				}

				if uKey >= 0 {
					dk := utils.NodeKeyFromBigIntArray(siblings[level][uKey*4 : uKey*4+4])
					sl, err := s.Db.Get(dk)
					if err != nil {
						return nil, err
					}
					siblings[level+1] = &sl

					if siblings[level+1].IsFinalNode() {
						valH := siblings[level+1].Get4to8()
						rKey := siblings[level+1].Get0to4()

						insKey := utils.JoinKey(append(usedKey, uKey), *rKey)

						for uKey >= 0 && level >= 0 {
							level -= 1
							if level >= 0 {
								uKey, err = siblings[level].IsUniqueSibling()
								if err != nil {
									return nil, err
								}
							}
						}

						oldKey := utils.RemoveKeyBits(*insKey, level+1)
						oldLeafHash, err := s.hashcalcAndSave(utils.ConcatArrays4(oldKey, *valH), utils.LeafCapacity)
						s.Db.InsertHashKey(oldLeafHash, *insKey)
						if err != nil {
							return nil, err
						}
					} else {
						// DELETE NOT FOUND
					}
				} else {
					// DELETE NOT FOUND
				}
			} else {
				// DELETE LAST
				finalRoot = utils.NodeKey{0, 0, 0, 0}
			}
		} else { // we're going zero to zero - do nothing
		}

		// calculate left part of the tree and free its memory usage
	}

	smtResponse := &SMTResponse{
		Mode:          "not run",
		NewRootScalar: &finalRoot,
	}

	return smtResponse, nil
}

func sortDataHoldersBitwiseAsc(dataHolders []*dataHolder) {
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
