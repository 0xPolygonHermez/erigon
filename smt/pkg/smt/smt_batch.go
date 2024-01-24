package smt

import (
	"fmt"

	"github.com/ledgerwatch/erigon/smt/pkg/utils"
)

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

	err = calculateRootNodeKeyIfNil(s, &workingRootNodeKey)
	if err != nil {
		return nil, err
	}

	for i := 0; i < size; i++ {
		var nodePathLevel int = -1

		var nextWorkingNodeKey *utils.NodeKey
		var nextWorkingSmtBatchNode **smtBatchNode

		var workingSmtBatchNodeParent *smtBatchNode
		workingSmtBatchNode := &smtBatchNodeRoot
		workingNodeKey := workingRootNodeKey
		workingRKey := *nodeKeys[i]

		nodePath := nodeKeys[i].GetPath()

		for {
			if (*workingSmtBatchNode) == nil { // update in-memory structure from db
				if !workingNodeKey.IsZero() {
					dbNodeValue, err := s.Db.Get(*workingNodeKey)
					if err != nil {
						return nil, err
					}

					nodeLeftKeyOrRkey := utils.NodeKeyFromBigIntArray(dbNodeValue[0:4])
					nodeRightKeyOrValueHash := utils.NodeKeyFromBigIntArray(dbNodeValue[4:8])
					*workingSmtBatchNode = &smtBatchNode{
						parentNode:              workingSmtBatchNodeParent,
						nodeLeftKeyOrRkey:       &nodeLeftKeyOrRkey,
						nodeRightKeyOrValueHash: &nodeRightKeyOrValueHash,
						leaf:                    dbNodeValue.IsFinalNode(),
					}
				} else {
					if nodePathLevel != -1 {
						return nil, fmt.Errorf("nodekey is zero at non-root level")
					}
				}
			}

			if (*workingSmtBatchNode) == nil {
				if nodePathLevel != -1 {
					return nil, fmt.Errorf("working smt pointer is nil at non-root level")
				}
				break
			}

			nodePathLevel++

			if (*workingSmtBatchNode).isLeaf() {
				break
			}

			direction := nodePath[nodePathLevel]
			nextWorkingNodeKey = (*workingSmtBatchNode).getNextNodeKey(direction)
			nextWorkingSmtBatchNode = (*workingSmtBatchNode).getChildByDirection(direction)
			if nextWorkingNodeKey.IsZero() && (*nextWorkingSmtBatchNode) == nil {
				break
			}

			workingNodeKey = nextWorkingNodeKey
			workingSmtBatchNodeParent = *workingSmtBatchNode
			workingSmtBatchNode = nextWorkingSmtBatchNode
		}

		nodeValue := nodeValues[i]
		nodeValueHash := nodeValuesHashes[i]

		// special case if root does not exists yet
		if (*workingSmtBatchNode) == nil {
			if !nodeValue.IsZero() {
				*workingSmtBatchNode = newSmtBatchNodeLeaf(&workingRKey, (*utils.NodeKey)(nodeValueHash), nil)
			} else {
				// nothing to delete
			}

			// dumpBatchTreeFromMemory(smtBatchNodeRoot, 0, make([]int, 0), 12)
			// fmt.Println()
			continue
		}

		workingRKey = utils.RemoveKeyBits(*nodeKeys[i], nodePathLevel)
		if !nodeValue.IsZero() {
			if !((*workingSmtBatchNode).isLeaf()) {
				nodePathDirection := nodePath[nodePathLevel]
				nextWorkingSmtBatchNode = (*workingSmtBatchNode).getChildByDirection(nodePathDirection)
				if (*nextWorkingSmtBatchNode) != nil {
					return nil, fmt.Errorf("branch has already been taken")
				}

				nodePathLevel++
				workingRKey = utils.RemoveKeyBits(*nodeKeys[i], nodePathLevel)
				*nextWorkingSmtBatchNode = newSmtBatchNodeLeaf(&workingRKey, nil, *workingSmtBatchNode)
				workingSmtBatchNode = nextWorkingSmtBatchNode
			}

			if !((*workingSmtBatchNode).isLeaf()) {
				return nil, fmt.Errorf("has a branch on a place where leaf is expected")
			}

			if !((*workingSmtBatchNode).nodeLeftKeyOrRkey.IsEqualTo(workingRKey)) {
				workingNodePath := utils.JoinKey(nodePath[:nodePathLevel], *(*workingSmtBatchNode).nodeLeftKeyOrRkey).GetPath()
				for nodePath[nodePathLevel] == workingNodePath[nodePathLevel] {
					workingNodePathDirection := workingNodePath[nodePathLevel]
					workingSmtBatchNode = (*workingSmtBatchNode).moveLeafByAddingALeafInDirection(workingNodePathDirection, nodePath[:nodePathLevel])

					nodePathLevel++
					workingRKey = utils.RemoveKeyBits(*nodeKeys[i], nodePathLevel)
				}

				workingNodePathDirection := workingNodePath[nodePathLevel]
				(*workingSmtBatchNode).moveLeafByAddingALeafInDirection(workingNodePathDirection, nodePath[:nodePathLevel])

				nodePathDirection := nodePath[nodePathLevel]
				nextWorkingSmtBatchNode = (*workingSmtBatchNode).getChildByDirection(nodePathDirection)
				nodePathLevel++
				workingRKey = utils.RemoveKeyBits(*nodeKeys[i], nodePathLevel)
				*nextWorkingSmtBatchNode = newSmtBatchNodeLeaf(&workingRKey, nil, *workingSmtBatchNode)
				workingSmtBatchNode = nextWorkingSmtBatchNode
			}

			(*workingSmtBatchNode).nodeRightKeyOrValueHash = (*utils.NodeKey)(nodeValueHash)
		}

		// dumpBatchTreeFromMemory(smtBatchNodeRoot, 0, make([]int, 0), 12)
		// fmt.Println()
	}

	if smtBatchNodeRoot == nil {
		workingRootNodeKey = &utils.NodeKey{0, 0, 0, 0}
	} else {
		calculateAndSaveHashesDfs(s, smtBatchNodeRoot)
		workingRootNodeKey = (*utils.NodeKey)(smtBatchNodeRoot.hash)
	}
	if err := s.setLastRoot(*workingRootNodeKey); err != nil {
		return nil, err
	}

	for i, nodeValue := range nodeValues {
		_, err = s.hashSave(nodeValue.ToUintArray(), utils.BranchCapacity, *nodeValuesHashes[i])
		if err != nil {
			return nil, err
		}
	}

	// dumpBatchTreeFromMemory(smtBatchNodeRoot, 0, make([]int, 0), 12)
	// fmt.Println()
	// dumpTree(s, (*workingRootNodeKey), 0, []int{}, 12)

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

// TODO: implement threads
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

func calculateRootNodeKeyIfNil(s *SMT, root **utils.NodeKey) error {
	if (*root) == nil {
		oldRootObj, err := s.getLastRoot()
		if err != nil {
			return err
		}
		(*root) = &oldRootObj
	}
	return nil
}

func calculateAndSaveHashesDfs(s *SMT, smtBatchNode *smtBatchNode) error {
	if smtBatchNode.isLeaf() {
		hashObj, err := s.hashcalcAndSave(utils.ConcatArrays4(*smtBatchNode.nodeLeftKeyOrRkey, *smtBatchNode.nodeRightKeyOrValueHash), utils.LeafCapacity)
		if err != nil {
			return err
		}
		smtBatchNode.hash = &hashObj
		return nil
	}

	var totalHash utils.NodeValue8

	if smtBatchNode.leftNode != nil {
		calculateAndSaveHashesDfs(s, smtBatchNode.leftNode)
		totalHash.SetHalfValue(*smtBatchNode.leftNode.hash, 0)
	} else {
		totalHash.SetHalfValue(*smtBatchNode.nodeLeftKeyOrRkey, 0)
	}

	if smtBatchNode.rightNode != nil {
		calculateAndSaveHashesDfs(s, smtBatchNode.rightNode)
		totalHash.SetHalfValue(*smtBatchNode.rightNode.hash, 1)
	} else {
		totalHash.SetHalfValue(*smtBatchNode.nodeRightKeyOrValueHash, 1)
	}

	hashObj, err := s.hashcalcAndSave(totalHash.ToUintArray(), utils.BranchCapacity)
	if err != nil {
		return err
	}
	smtBatchNode.hash = &hashObj
	return nil
}

type smtBatchNode struct {
	nodeLeftKeyOrRkey       *utils.NodeKey
	nodeRightKeyOrValueHash *utils.NodeKey
	leaf                    bool
	parentNode              *smtBatchNode // if nil => this is the root node
	leftNode                *smtBatchNode
	rightNode               *smtBatchNode
	hash                    *[4]uint64
}

func newSmtBatchNodeLeaf(nodeLeftKeyOrRkey, nodeRightKeyOrValueHash *utils.NodeKey, parentNode *smtBatchNode) *smtBatchNode {
	return &smtBatchNode{
		nodeLeftKeyOrRkey:       nodeLeftKeyOrRkey,
		nodeRightKeyOrValueHash: nodeRightKeyOrValueHash,
		leaf:                    true,
		parentNode:              parentNode,
		leftNode:                nil,
		rightNode:               nil,
		hash:                    nil,
	}
}

func (sbn *smtBatchNode) isLeaf() bool {
	return sbn.leaf
}

func (sbn *smtBatchNode) getNextNodeKey(direction int) *utils.NodeKey {
	if direction == 0 {
		return sbn.nodeLeftKeyOrRkey
	} else {
		return sbn.nodeRightKeyOrValueHash
	}
}

func (sbn *smtBatchNode) getChildByDirection(direction int) **smtBatchNode {
	if direction == 0 {
		return &sbn.leftNode
	} else {
		return &sbn.rightNode
	}
}

func (sbn *smtBatchNode) moveLeafByAddingALeafInDirection(direction int, nodePath []int) **smtBatchNode {
	childPointer := sbn.getChildByDirection(direction)

	nodeKey := utils.JoinKey(nodePath, *sbn.nodeLeftKeyOrRkey)
	workingSmtBatchNodeRkey := utils.RemoveKeyBits(*nodeKey, len(nodePath)+1)

	*childPointer = newSmtBatchNodeLeaf(&workingSmtBatchNodeRkey, sbn.nodeRightKeyOrValueHash, sbn)
	sbn.nodeLeftKeyOrRkey = &utils.NodeKey{0, 0, 0, 0}
	sbn.nodeRightKeyOrValueHash = &utils.NodeKey{0, 0, 0, 0}
	sbn.leaf = false

	return childPointer
}

func dumpBatchTreeFromMemory(sbn *smtBatchNode, level int, path []int, printDepth int) {
	if sbn == nil {
		if level == 0 {
			fmt.Printf("Empty tree\n")
		}
		return
	}

	if !sbn.isLeaf() {
		dumpBatchTreeFromMemory(sbn.rightNode, level+1, append(path, 1), printDepth)
	}

	if sbn.isLeaf() {
		rKey := sbn.nodeLeftKeyOrRkey
		leafValueHash := sbn.nodeRightKeyOrValueHash
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
		dumpBatchTreeFromMemory(sbn.leftNode, level+1, append(path, 0), printDepth)
	}
}
