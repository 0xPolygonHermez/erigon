package smt

import (
	"fmt"
	"sync"

	"github.com/dgravesa/go-parallel/parallel"
	"github.com/ledgerwatch/erigon/smt/pkg/utils"
)

func (s *SMT) InsertBatch(nodeKeys []*utils.NodeKey, nodeValues []*utils.NodeValue8, nodeValuesHashes []*[4]uint64, rootNodeKey *utils.NodeKey) (*SMTResponse, error) {
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

	err = calculateRootNodeKeyIfNil(s, &rootNodeKey)
	if err != nil {
		return nil, err
	}

	for i := 0; i < size; i++ {
		insertingNodeKey := nodeKeys[i]
		insertingNodeValue := nodeValues[i]
		insertingNodeValueHash := nodeValuesHashes[i]

		insertingNodePathLevel, insertingNodePath, insertingPointerToSmtBatchNode, err := findInsertingPoint(s, insertingNodeKey, rootNodeKey, &smtBatchNodeRoot)
		if err != nil {
			return nil, err
		}

		// special case if root does not exists yet
		if (*insertingPointerToSmtBatchNode) == nil {
			if !insertingNodeValue.IsZero() {
				*insertingPointerToSmtBatchNode = newSmtBatchNodeLeaf(insertingNodeKey, (*utils.NodeKey)(insertingNodeValueHash), nil)
			} else {
				// nothing to delete
			}
			continue
		}

		insertingRemainingKey := utils.RemoveKeyBits(*insertingNodeKey, insertingNodePathLevel)
		if !insertingNodeValue.IsZero() {
			if !((*insertingPointerToSmtBatchNode).isLeaf()) {
				if insertingPointerToSmtBatchNode, err = (*insertingPointerToSmtBatchNode).createALeafInEmptyDirection(insertingNodePath, insertingNodePathLevel, insertingNodeKey); err != nil {
					return nil, err
				}
				insertingRemainingKey = *((*insertingPointerToSmtBatchNode).nodeLeftKeyOrRemainingKey)
				insertingNodePathLevel++
			}

			if !((*insertingPointerToSmtBatchNode).nodeLeftKeyOrRemainingKey.IsEqualTo(insertingRemainingKey)) {
				currentTreeNodePath := utils.JoinKey(insertingNodePath[:insertingNodePathLevel], *(*insertingPointerToSmtBatchNode).nodeLeftKeyOrRemainingKey).GetPath()
				for insertingNodePath[insertingNodePathLevel] == currentTreeNodePath[insertingNodePathLevel] {
					insertingPointerToSmtBatchNode = (*insertingPointerToSmtBatchNode).moveLeafByAddingALeafInDirection(insertingNodePath, insertingNodePathLevel)
					insertingNodePathLevel++
				}

				(*insertingPointerToSmtBatchNode).moveLeafByAddingALeafInDirection(currentTreeNodePath, insertingNodePathLevel)

				if insertingPointerToSmtBatchNode, err = (*insertingPointerToSmtBatchNode).createALeafInEmptyDirection(insertingNodePath, insertingNodePathLevel, insertingNodeKey); err != nil {
					return nil, err
				}
				insertingRemainingKey = *((*insertingPointerToSmtBatchNode).nodeLeftKeyOrRemainingKey)
				insertingNodePathLevel++
			}

			(*insertingPointerToSmtBatchNode).nodeRightKeyOrValueHash = (*utils.NodeKey)(insertingNodeValueHash)
		} else {
			//TODO: implement delete
		}
	}

	if smtBatchNodeRoot == nil {
		rootNodeKey = &utils.NodeKey{0, 0, 0, 0}
	} else {
		calculateAndSaveHashesDfs(s, smtBatchNodeRoot)
		rootNodeKey = (*utils.NodeKey)(smtBatchNodeRoot.hash)
	}
	if err := s.setLastRoot(*rootNodeKey); err != nil {
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

	return &SMTResponse{
		Mode:          "batch insert",
		NewRootScalar: rootNodeKey,
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

func calculateNodeValueHashesIfMissing(s *SMT, nodeValues []*utils.NodeValue8, nodeValuesHashes *[]*[4]uint64) error {
	var globalError error
	size := len(nodeValues)
	cpuNum := parallel.DefaultNumGoroutines()

	if size > (cpuNum << 2) {
		var wg sync.WaitGroup
		itemsPerCpu := (size + cpuNum - 1) / cpuNum

		wg.Add(cpuNum)

		for cpuIndex := 0; cpuIndex < cpuNum; cpuIndex++ {
			go func(cpuIndexInThread int) {
				defer wg.Done()
				startIndex := cpuIndexInThread * itemsPerCpu
				endIndex := startIndex + itemsPerCpu
				if endIndex > size {
					endIndex = size
				}
				err := calculateNodeValueHashesIfMissingInInterval(s, nodeValues, nodeValuesHashes, startIndex, endIndex)
				if err != nil {
					globalError = err
				}
			}(cpuIndex)
		}

		wg.Wait()
	} else {
		globalError = calculateNodeValueHashesIfMissingInInterval(s, nodeValues, nodeValuesHashes, 0, len(nodeValues))
	}

	return globalError
}

func calculateNodeValueHashesIfMissingInInterval(s *SMT, nodeValues []*utils.NodeValue8, nodeValuesHashes *[]*[4]uint64, startIndex, endIndex int) error {
	for i := startIndex; i < endIndex; i++ {
		if (*nodeValuesHashes)[i] != nil {
			continue
		}

		nodeValueHashObj, err := s.hashcalc(nodeValues[i].ToUintArray(), utils.BranchCapacity)
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

func findInsertingPoint(s *SMT, insertingNodeKey, insertingPointerNodeKey *utils.NodeKey, insertingPointerToSmtBatchNode **smtBatchNode) (int, []int, **smtBatchNode, error) {
	var insertingNodePathLevel int = -1

	var nextInsertingPointerNodeKey *utils.NodeKey
	var nextInsertingPointerToSmtBatchNode **smtBatchNode

	var insertingPointerToSmtBatchNodeParent *smtBatchNode

	insertingNodePath := insertingNodeKey.GetPath()

	for {
		if (*insertingPointerToSmtBatchNode) == nil { // update in-memory structure from db
			if !insertingPointerNodeKey.IsZero() {
				dbNodeValue, err := s.Db.Get(*insertingPointerNodeKey)
				if err != nil {
					return -2, []int{}, insertingPointerToSmtBatchNode, err
				}

				nodeLeftKeyOrRemainingKey := utils.NodeKeyFromBigIntArray(dbNodeValue[0:4])
				nodeRightKeyOrValueHash := utils.NodeKeyFromBigIntArray(dbNodeValue[4:8])
				*insertingPointerToSmtBatchNode = &smtBatchNode{
					parentNode:                insertingPointerToSmtBatchNodeParent,
					nodeLeftKeyOrRemainingKey: &nodeLeftKeyOrRemainingKey,
					nodeRightKeyOrValueHash:   &nodeRightKeyOrValueHash,
					leaf:                      dbNodeValue.IsFinalNode(),
				}
			} else {
				if insertingNodePathLevel != -1 {
					return -2, []int{}, insertingPointerToSmtBatchNode, fmt.Errorf("nodekey is zero at non-root level")
				}
			}
		}

		if (*insertingPointerToSmtBatchNode) == nil {
			if insertingNodePathLevel != -1 {
				return -2, []int{}, insertingPointerToSmtBatchNode, fmt.Errorf("working smt pointer is nil at non-root level")
			}
			break
		}

		insertingNodePathLevel++

		if (*insertingPointerToSmtBatchNode).isLeaf() {
			break
		}

		insertDirection := insertingNodePath[insertingNodePathLevel]
		nextInsertingPointerNodeKey = (*insertingPointerToSmtBatchNode).getNextNodeKeyInDirection(insertDirection)
		nextInsertingPointerToSmtBatchNode = (*insertingPointerToSmtBatchNode).getChildInDirection(insertDirection)
		if nextInsertingPointerNodeKey.IsZero() && (*nextInsertingPointerToSmtBatchNode) == nil {
			break
		}

		insertingPointerNodeKey = nextInsertingPointerNodeKey
		insertingPointerToSmtBatchNodeParent = *insertingPointerToSmtBatchNode
		insertingPointerToSmtBatchNode = nextInsertingPointerToSmtBatchNode
	}

	return insertingNodePathLevel, insertingNodePath, insertingPointerToSmtBatchNode, nil
}

func calculateAndSaveHashesDfs(s *SMT, smtBatchNode *smtBatchNode) error {
	if smtBatchNode.isLeaf() {
		hashObj, err := s.hashcalcAndSave(utils.ConcatArrays4(*smtBatchNode.nodeLeftKeyOrRemainingKey, *smtBatchNode.nodeRightKeyOrValueHash), utils.LeafCapacity)
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
		totalHash.SetHalfValue(*smtBatchNode.nodeLeftKeyOrRemainingKey, 0)
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
	nodeLeftKeyOrRemainingKey *utils.NodeKey
	nodeRightKeyOrValueHash   *utils.NodeKey
	leaf                      bool
	parentNode                *smtBatchNode // if nil => this is the root node
	leftNode                  *smtBatchNode
	rightNode                 *smtBatchNode
	hash                      *[4]uint64
}

func newSmtBatchNodeLeaf(nodeLeftKeyOrRemainingKey, nodeRightKeyOrValueHash *utils.NodeKey, parentNode *smtBatchNode) *smtBatchNode {
	return &smtBatchNode{
		nodeLeftKeyOrRemainingKey: nodeLeftKeyOrRemainingKey,
		nodeRightKeyOrValueHash:   nodeRightKeyOrValueHash,
		leaf:                      true,
		parentNode:                parentNode,
		leftNode:                  nil,
		rightNode:                 nil,
		hash:                      nil,
	}
}

func (sbn *smtBatchNode) isLeaf() bool {
	return sbn.leaf
}

func (sbn *smtBatchNode) getNextNodeKeyInDirection(direction int) *utils.NodeKey {
	if direction == 0 {
		return sbn.nodeLeftKeyOrRemainingKey
	} else {
		return sbn.nodeRightKeyOrValueHash
	}
}

func (sbn *smtBatchNode) getChildInDirection(direction int) **smtBatchNode {
	if direction == 0 {
		return &sbn.leftNode
	} else {
		return &sbn.rightNode
	}
}

func (sbn *smtBatchNode) createALeafInEmptyDirection(insertingNodePath []int, insertingNodePathLevel int, insertingNodeKey *utils.NodeKey) (**smtBatchNode, error) {
	direction := insertingNodePath[insertingNodePathLevel]
	childPointer := sbn.getChildInDirection(direction)
	if (*childPointer) != nil {
		return nil, fmt.Errorf("branch has already been taken")
	}
	remainingKey := utils.RemoveKeyBits(*insertingNodeKey, insertingNodePathLevel+1)
	*childPointer = newSmtBatchNodeLeaf(&remainingKey, nil, sbn)
	return childPointer, nil
}

func (sbn *smtBatchNode) moveLeafByAddingALeafInDirection(insertingNodeKey []int, insertingNodeKeyLevel int) **smtBatchNode {
	direction := insertingNodeKey[insertingNodeKeyLevel]
	insertingNodeKeyUpToLevel := insertingNodeKey[:insertingNodeKeyLevel]

	childPointer := sbn.getChildInDirection(direction)

	nodeKey := utils.JoinKey(insertingNodeKeyUpToLevel, *sbn.nodeLeftKeyOrRemainingKey)
	remainingKey := utils.RemoveKeyBits(*nodeKey, len(insertingNodeKeyUpToLevel)+1)

	*childPointer = newSmtBatchNodeLeaf(&remainingKey, sbn.nodeRightKeyOrValueHash, sbn)
	sbn.nodeLeftKeyOrRemainingKey = &utils.NodeKey{0, 0, 0, 0}
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
		remainingKey := sbn.nodeLeftKeyOrRemainingKey
		leafValueHash := sbn.nodeRightKeyOrValueHash
		totalKey := utils.JoinKey(path, *remainingKey)
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
