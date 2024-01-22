package smt

import (
	"fmt"

	"github.com/ledgerwatch/erigon/smt/pkg/utils"
)

type dataHolder struct {
	nodeKey       *utils.NodeKey
	nodeValue     *utils.NodeValue8
	nodeValueHash *[4]uint64
}

// func (s *SMT) insertBatch(nodeKeys []*utils.NodeKey, nodeValues []*utils.NodeValue8, nodeValuesHashes []*[4]uint64, oldRoot *utils.NodeKey) (*SMTResponse, error) {
// 	s.clearUpMutex.Lock()
// 	defer s.clearUpMutex.Unlock()

// 	var err error

// 	size := len(nodeKeys)
// 	dataHolders := make([]*dataHolder, size)
// 	for i := 0; i < size; i++ {
// 		var nodeValue *utils.NodeValue8
// 		var nodeValueHash *[4]uint64

// 		if nodeValues != nil {
// 			nodeValue = nodeValues[i]
// 		}
// 		if nodeValuesHashes != nil {
// 			nodeValueHash = nodeValuesHashes[i]
// 		}
// 		// this can happen in parallel
// 		if nodeValueHash == nil {
// 			nodeValueHashObj, err := s.hashcalc(nodeValue.ToUintArray(), utils.BranchCapacity)
// 			if err != nil {
// 				return nil, err
// 			}
// 			nodeValueHash = &nodeValueHashObj
// 		}
// 		dataHolders[i] = &dataHolder{
// 			nodeKey:       nodeKeys[i],
// 			nodeValue:     nodeValue,
// 			nodeValueHash: nodeValueHash,
// 		}
// 	}

// 	sortDataHoldersBitwiseAsc(dataHolders)

// 	if oldRoot == nil {
// 		oldRootObj, err := s.getLastRoot()
// 		if err != nil {
// 			return nil, err
// 		}
// 		oldRoot = &oldRootObj
// 	}

// 	// rootBatchNode := &smtBatchNode{}

// 	var finalRoot utils.NodeKey = *oldRoot

// 	for _, dataHolder := range dataHolders {
// 		var usedKey []int
// 		var level int
// 		var foundKey *utils.NodeKey
// 		var foundRKey utils.NodeKey
// 		var foundOldValHash utils.NodeKey

// 		siblings := map[int]*utils.NodeValue12{}
// 		refNodeKey := oldRoot

// 		nodeKey := dataHolder.nodeKey
// 		nodeValue := dataHolder.nodeValue
// 		nodeValueHash := dataHolder.nodeValueHash
// 		nodePath := dataHolder.nodeKey.GetPath()

// 		for !refNodeKey.IsZero() && foundKey == nil {
// 			sl, err := s.Db.Get(*refNodeKey)
// 			if err != nil {
// 				return nil, err
// 			}
// 			siblings[level] = &sl
// 			if siblings[level].IsFinalNode() {
// 				foundRKey = utils.NodeKeyFromBigIntArray(siblings[level][0:4])
// 				foundOldValHash = utils.NodeKeyFromBigIntArray(siblings[level][4:8])
// 				foundKey = utils.JoinKey(usedKey, foundRKey)
// 				if err != nil {
// 					return nil, err
// 				}
// 			} else {
// 				refNodeKeyObj := utils.NodeKeyFromBigIntArray(siblings[level][nodePath[level]*4 : nodePath[level]*4+4])
// 				refNodeKey = &refNodeKeyObj
// 				usedKey = append(usedKey, nodePath[level])
// 				level++
// 			}
// 		}

// 		level--
// 		if len(usedKey) != 0 {
// 			usedKey = usedKey[:len(usedKey)-1]
// 		}

// 		if !nodeValue.IsZero() { // we have a value - so we're updating or inserting
// 			if foundKey != nil {
// 				if foundKey.IsEqualTo(*nodeKey) {
// 					_, err = s.hashSave(nodeValue.ToUintArray(), utils.BranchCapacity, *nodeValueHash)
// 					if err != nil {
// 						return nil, err
// 					}
// 					newLeafHash, err := s.hashcalcAndSave(utils.ConcatArrays4(foundRKey, *nodeValueHash), utils.LeafCapacity)
// 					if err != nil {
// 						return nil, err
// 					}
// 					s.Db.InsertHashKey(newLeafHash, *nodeKey)
// 				} else {
// 					level2 := level + 1
// 					foundKeys := foundKey.GetPath()

// 					for {
// 						if level2 >= len(nodePath) || level2 >= len(foundKeys) {
// 							break
// 						}

// 						if nodePath[level2] != foundKeys[level2] {
// 							break
// 						}

// 						level2++
// 					}

// 					oldKey := utils.RemoveKeyBits(*foundKey, level2+1)
// 					oldLeafHash, err := s.hashcalcAndSave(utils.ConcatArrays4(oldKey, foundOldValHash), utils.LeafCapacity)
// 					s.Db.InsertHashKey(oldLeafHash, *foundKey)
// 					if err != nil {
// 						return nil, err
// 					}

// 					newKey := utils.RemoveKeyBits(*nodeKey, level2+1)

// 					_, err = s.hashSave(nodeValue.ToUintArray(), utils.BranchCapacity, *nodeValueHash)
// 					if err != nil {
// 						return nil, err
// 					}
// 					newLeafHash, err := s.hashcalcAndSave(utils.ConcatArrays4(newKey, *nodeValueHash), utils.LeafCapacity)
// 					if err != nil {
// 						return nil, err
// 					}
// 					s.Db.InsertHashKey(newLeafHash, *nodeKey)

// 					var node [8]uint64
// 					for i := 0; i < 8; i++ {
// 						node[i] = 0
// 					}

// 					for j := 0; j < 4; j++ {
// 						node[nodePath[level2]*4+j] = newLeafHash[j]
// 						node[foundKeys[level2]*4+j] = oldLeafHash[j]
// 					}

// 					r2, err := s.hashcalcAndSave(node, utils.BranchCapacity)
// 					if err != nil {
// 						return nil, err
// 					}
// 					level2 -= 1

// 					for level2 != level {
// 						for i := 0; i < 8; i++ {
// 							node[i] = 0
// 						}

// 						for j := 0; j < 4; j++ {
// 							node[nodePath[level2]*4+j] = r2[j]
// 						}

// 						r2, err = s.hashcalcAndSave(node, utils.BranchCapacity)
// 						if err != nil {
// 							return nil, err
// 						}
// 						level2 -= 1
// 					}
// 				}
// 			} else {
// 				newKey := utils.RemoveKeyBits(*nodeKey, level+1)

// 				_, err = s.hashSave(nodeValue.ToUintArray(), utils.BranchCapacity, *nodeValueHash)
// 				if err != nil {
// 					return nil, err
// 				}
// 				newLeafHash, err := s.hashcalcAndSave(utils.ConcatArrays4(newKey, *nodeValueHash), utils.LeafCapacity)
// 				if err != nil {
// 					return nil, err
// 				}
// 				s.Db.InsertHashKey(newLeafHash, *nodeKey)
// 			}
// 		} else if foundKey != nil && foundKey.IsEqualTo(*nodeKey) { // we don't have a value so we're deleting
// 			if level >= 0 {
// 				// for j := 0; j < 4; j++ {
// 				// 	siblings[level][keys[level]*4+j] = big.NewInt(0)
// 				// }

// 				uKey, err := siblings[level].IsUniqueSibling()
// 				if err != nil {
// 					return nil, err
// 				}

// 				if uKey >= 0 {
// 					dk := utils.NodeKeyFromBigIntArray(siblings[level][uKey*4 : uKey*4+4])
// 					sl, err := s.Db.Get(dk)
// 					if err != nil {
// 						return nil, err
// 					}
// 					siblings[level+1] = &sl

// 					if siblings[level+1].IsFinalNode() {
// 						valH := siblings[level+1].Get4to8()
// 						rKey := siblings[level+1].Get0to4()

// 						insKey := utils.JoinKey(append(usedKey, uKey), *rKey)

// 						for uKey >= 0 && level >= 0 {
// 							level -= 1
// 							if level >= 0 {
// 								uKey, err = siblings[level].IsUniqueSibling()
// 								if err != nil {
// 									return nil, err
// 								}
// 							}
// 						}

// 						oldKey := utils.RemoveKeyBits(*insKey, level+1)
// 						oldLeafHash, err := s.hashcalcAndSave(utils.ConcatArrays4(oldKey, *valH), utils.LeafCapacity)
// 						s.Db.InsertHashKey(oldLeafHash, *insKey)
// 						if err != nil {
// 							return nil, err
// 						}
// 					} else {
// 						// DELETE NOT FOUND
// 					}
// 				} else {
// 					// DELETE NOT FOUND
// 				}
// 			} else {
// 				// DELETE LAST
// 				finalRoot = utils.NodeKey{0, 0, 0, 0}
// 			}
// 		} else { // we're going zero to zero - do nothing
// 		}

// 		// calculate left part of the tree and free its memory usage
// 	}

// 	smtResponse := &SMTResponse{
// 		Mode:          "not run",
// 		NewRootScalar: &finalRoot,
// 	}

// 	return smtResponse, nil
// }

// func sortDataHoldersBitwiseAsc(dataHolders []*dataHolder) {
// 	sort.Slice(dataHolders, func(i, j int) bool {
// 		aTmp := dataHolders[i].nodeKey
// 		bTmp := dataHolders[j].nodeKey

// 		for l := 0; l < 64; l++ {
// 			for n := 0; n < 4; n++ {
// 				aBit := aTmp[n] & 1
// 				bBit := bTmp[n] & 1

// 				aTmp[n] >>= 1 // Right shift the current part
// 				bTmp[n] >>= 1 // Right shift the current part

// 				if aBit != bBit {
// 					return aBit < bBit
// 				}
// 			}
// 		}

// 		return true
// 	})
// }

/*
NodeKey -> Value
NodeKey { Hash(rKey, Hash(Value)) } -> rKey, Hash(Value), 1
NodeKey { Hash(LeftNodeKey, RightNodeKey) } -> LeftNodeKey, RightNodeKey, 0
*/

func (s *SMT) InsertBatch(nodeKeys []*utils.NodeKey, nodeValues []*utils.NodeValue8, nodeValuesHashes []*[4]uint64, workingRootNodeKey *utils.NodeKey) (*SMTResponse, error) {
	s.clearUpMutex.Lock()
	defer s.clearUpMutex.Unlock()

	var size int = len(nodeKeys)
	var err error
	var smtBatchNodeRoot *smtBatchNode = nil

	err = validateDataLengths(nodeKeys, nodeValues, &nodeValuesHashes)
	if err != nil {
		return nil, err
	}

	err = calculateNodeValueHashesIfMissing(s, nodeValues, &nodeValuesHashes)
	if err != nil {
		return nil, err
	}

	workingRootNodeKey, err = getRootNodeKeyIfNil(s, workingRootNodeKey)
	if err != nil {
		return nil, err
	}

	for i := 0; i < size; i++ {
		var level int = -1
		var workingPath []int = make([]int, 256)

		var workingSmtBatchNodeParent *smtBatchNode
		workingSmtBatchNode := &smtBatchNodeRoot
		workingNodeKey := workingRootNodeKey

		nodePath := nodeKeys[i].GetPath()

		for {
			if (*workingSmtBatchNode) == nil { // update in-memory structure from db
				if !workingNodeKey.IsZero() {
					dbNodeValue, err := s.Db.Get(*workingNodeKey)
					if err != nil {
						return nil, err
					}

					*workingSmtBatchNode = &smtBatchNode{
						nodeKey:    workingNodeKey,
						parentNode: workingSmtBatchNodeParent,
						nodeValue:  &dbNodeValue,
					}
				} else {
					if level != -1 {
						return nil, fmt.Errorf("nodekey is zero at non-root level")
					}
				}
			}

			if (*workingSmtBatchNode) == nil {
				break
			}

			level++

			if (*workingSmtBatchNode).isLeaf() {
				break
			}

			direction := nodePath[level]
			workingSmtBatchNodeParent = *workingSmtBatchNode
			workingNodeKey = (*workingSmtBatchNode).getNextNodeKey(direction)
			if direction == 0 {
				workingSmtBatchNode = &((*workingSmtBatchNode).leftNode)
			} else {
				workingSmtBatchNode = &((*workingSmtBatchNode).rightNode)
			}

			workingPath[level] = direction
		}

		// insert, update, delete based on workingSmtBatchNode
	}

	calculateHashesDfs(s, smtBatchNodeRoot)

	dumpBatchTree(smtBatchNodeRoot, 0, make([]int, 0), 8)

	return &SMTResponse{
		Mode:          "batch insert",
		NewRootScalar: workingRootNodeKey,
	}, nil
}

func validateDataLengths(nodeKeys []*utils.NodeKey, nodeValues []*utils.NodeValue8, nodeValuesHashes *[]*[4]uint64) error {
	var size int = len(nodeKeys)

	if len(nodeValues) != size {
		return fmt.Errorf("mismatch nodeValues length, expected %d but got %d", size, len(nodeValues))
	}

	if (*nodeValuesHashes) == nil {
		(*nodeValuesHashes) = make([]*[4]uint64, size)
	}
	if len(*nodeValuesHashes) != size {
		return fmt.Errorf("mismatch nodeValuesHashes length, expected %d but got %d", size, len(*nodeValuesHashes))
	}

	return nil
}

// this can happen in parallel
func calculateNodeValueHashesIfMissing(s *SMT, nodeValues []*utils.NodeValue8, nodeValuesHashes *[]*[4]uint64) error {
	for i, nodeValue := range nodeValues {
		if (*nodeValuesHashes)[i] != nil {
			continue
		}

		nodeValueHashObj, err := s.hashcalc(nodeValue.ToUintArray(), utils.BranchCapacity)
		if err != nil {
			return err
		}

		(*nodeValuesHashes)[i] = &nodeValueHashObj
	}

	return nil
}

// update it to using pointer-to-pointer
func getRootNodeKeyIfNil(s *SMT, root *utils.NodeKey) (*utils.NodeKey, error) {
	if root == nil {
		oldRootObj, err := s.getLastRoot()
		if err != nil {
			return nil, err
		}
		root = &oldRootObj
	}
	return root, nil
}

func calculateHashesDfs(s *SMT, smtBatchNodeRoot *smtBatchNode) error {
	if smtBatchNodeRoot.isLeaf() {
		hashObj := [4]uint64{0, 0, 0, 0}
		smtBatchNodeRoot.hash = &hashObj
		return nil
	}

	var leftHash *[4]uint64
	var rightHash *[4]uint64

	if smtBatchNodeRoot.leftNode == nil {
		// no changes on the left part => get the hash from parent node or db
		leftHashObj := [4]uint64{0, 0, 0, 0}
		leftHash = &leftHashObj
	} else {
		calculateHashesDfs(s, smtBatchNodeRoot.leftNode)
		leftHash = smtBatchNodeRoot.leftNode.hash
	}

	if smtBatchNodeRoot.rightNode == nil {
		// no changes on the right part => get the hash from parent node or db
		rightHashObj := [4]uint64{0, 0, 0, 0}
		rightHash = &rightHashObj
	} else {
		calculateHashesDfs(s, smtBatchNodeRoot.rightNode)
		rightHash = smtBatchNodeRoot.rightNode.hash
	}

	var totalHash utils.NodeValue8
	totalHash.SetHalfValue(*leftHash, 0)
	totalHash.SetHalfValue(*rightHash, 1)

	return nil
}

type smtBatchNode struct {
	nodeKey    *utils.NodeKey // if nil => there is no even root node
	parentNode *smtBatchNode  // if nil => this is the root node
	leftNode   *smtBatchNode
	rightNode  *smtBatchNode
	nodeValue  *utils.NodeValue12
	hash       *[4]uint64
}

func (sbn *smtBatchNode) isRoot() bool {
	return sbn.parentNode == nil
}

func (sbn *smtBatchNode) isLeaf() bool {
	return sbn.nodeValue.IsFinalNode()
}

func (sbn *smtBatchNode) getRKey() *utils.NodeKey {
	nodeKeyObj := utils.NodeKeyFromBigIntArray(sbn.nodeValue[0:4])
	return &nodeKeyObj
}

func (sbn *smtBatchNode) getNextNodeKey(direction int) *utils.NodeKey {
	nextDirectionIndexStart := direction << 2
	nextNodeKeyAsArray := sbn.nodeValue[nextDirectionIndexStart : nextDirectionIndexStart+4]
	nextNodeKey := utils.NodeKeyFromBigIntArray(nextNodeKeyAsArray)
	return &nextNodeKey
}

func dumpBatchTree(sbn *smtBatchNode, level int, path []int, printDepth int) {
	if sbn == nil {
		if level == 0 {
			fmt.Printf("Empty tree\n")
		}
		return
	}

	// nodeValue := sbn.nodeValue
	if !sbn.isLeaf() {
		dumpBatchTree(sbn.rightNode, level+1, append(path, 1), printDepth)
	}

	if sbn.isLeaf() {
		rKey := sbn.getRKey()
		leafValueHash := utils.NodeKeyFromBigIntArray(sbn.nodeValue[4:8])
		totalKey := utils.JoinKey(path, *rKey)
		leafPath := totalKey.GetPath()
		fmt.Printf("|")
		for i := 0; i < level; i++ {
			fmt.Printf("=")
		}
		fmt.Printf("%s", convertPathToBinaryString(path))
		for i := level * 2; i < printDepth; i++ {
			fmt.Printf("-")
		}
		fmt.Printf(" # %s -> %+v", convertPathToBinaryString(leafPath), leafValueHash)
		fmt.Println()
		return
	} else {
		fmt.Printf("|")
		for i := 0; i < level; i++ {
			fmt.Printf("=")
		}
		fmt.Printf("%s", convertPathToBinaryString(path))
		for i := level * 2; i < printDepth; i++ {
			fmt.Printf("-")
		}
		fmt.Println()
	}

	if !sbn.isLeaf() {
		dumpBatchTree(sbn.leftNode, level+1, append(path, 0), printDepth)
	}
}
