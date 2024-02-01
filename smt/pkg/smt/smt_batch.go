package smt

import (
	"fmt"
	"sync"

	"github.com/dgravesa/go-parallel/parallel"
	"github.com/ledgerwatch/erigon/smt/pkg/utils"
	"github.com/ledgerwatch/erigon/zk"
)

func (s *SMT) InsertBatch(logPrefix string, nodeKeys []*utils.NodeKey, nodeValues []*utils.NodeValue8, nodeValuesHashes []*[4]uint64, rootNodeKey *utils.NodeKey) (*SMTResponse, error) {
	s.clearUpMutex.Lock()
	defer s.clearUpMutex.Unlock()

	var size int = len(nodeKeys)
	var err error
	var smtBatchNodeRoot *smtBatchNode
	valueHashesToDelete := []*[4]uint64{}

	//start a progress checker
	preprocessStage := uint64(size) / 10
	finalizeStage := preprocessStage
	progressChan, stopProgressPrinter := zk.ProgressPrinterWithoutValues(fmt.Sprintf("[%s] SMT incremental progress", logPrefix), uint64(size)+preprocessStage+finalizeStage)
	defer stopProgressPrinter()

	err = validateDataLengths(nodeKeys, nodeValues, &nodeValuesHashes)
	if err != nil {
		return nil, err
	}

	err = calculateNodeValueHashesIfMissing(s, nodeValues, &nodeValuesHashes)
	if err != nil {
		return nil, err
	}

	progressChan <- uint64(preprocessStage)

	err = calculateRootNodeKeyIfNil(s, &rootNodeKey)
	if err != nil {
		return nil, err
	}

	for i := 0; i < size; i++ {
		progressChan <- preprocessStage + uint64(i)

		insertingNodeKey := nodeKeys[i]
		insertingNodeValue := nodeValues[i]
		insertingNodeValueHash := nodeValuesHashes[i]

		insertingNodePathLevel, insertingNodePath, insertingPointerToSmtBatchNode, err := findInsertingPoint(s, insertingNodeKey, rootNodeKey, &smtBatchNodeRoot, insertingNodeValue.IsZero())
		if err != nil {
			return nil, err
		}

		// special case if root does not exists yet
		if (*insertingPointerToSmtBatchNode) == nil {
			if !insertingNodeValue.IsZero() {
				*insertingPointerToSmtBatchNode = newSmtBatchNodeLeaf(insertingNodeKey, (*utils.NodeKey)(insertingNodeValueHash), nil)
			}
			// else branch would be for deleting a value but the root does not exists => there is nothing to delete
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
					insertingPointerToSmtBatchNode = (*insertingPointerToSmtBatchNode).expandLeafByAddingALeafInDirection(insertingNodePath, insertingNodePathLevel)
					insertingNodePathLevel++
				}

				(*insertingPointerToSmtBatchNode).expandLeafByAddingALeafInDirection(currentTreeNodePath, insertingNodePathLevel)

				if insertingPointerToSmtBatchNode, err = (*insertingPointerToSmtBatchNode).createALeafInEmptyDirection(insertingNodePath, insertingNodePathLevel, insertingNodeKey); err != nil {
					return nil, err
				}
				// there is no need to update insertingRemainingKey because it is not needed anymore therefore its value is incorrect if used after this line
				// insertingRemainingKey = *((*insertingPointerToSmtBatchNode).nodeLeftKeyOrRemainingKey)
				insertingNodePathLevel++
			}

			if (*insertingPointerToSmtBatchNode).nodeRightKeyOrValueHash != nil {
				valueHashesToDelete = append(valueHashesToDelete, (*[4]uint64)((*insertingPointerToSmtBatchNode).nodeRightKeyOrValueHash))
			}
			(*insertingPointerToSmtBatchNode).nodeRightKeyOrValueHash = (*utils.NodeKey)(insertingNodeValueHash)
		} else {
			if (*insertingPointerToSmtBatchNode).nodeLeftKeyOrRemainingKey.IsEqualTo(insertingRemainingKey) {
				valueHashesToDelete = append(valueHashesToDelete, (*[4]uint64)((*insertingPointerToSmtBatchNode).nodeRightKeyOrValueHash))

				parentAfterDelete := &((*insertingPointerToSmtBatchNode).parentNode)
				*insertingPointerToSmtBatchNode = nil
				insertingPointerToSmtBatchNode = parentAfterDelete
				(*insertingPointerToSmtBatchNode).updateKeysAfterDelete()
				insertingNodePathLevel--
				// there is no need to update insertingRemainingKey because it is not needed anymore therefore its value is incorrect if used after this line
				// insertingRemainingKey = utils.RemoveKeyBits(*insertingNodeKey, insertingNodePathLevel)
			}

			for {
				// the root node has been deleted so we can safely break
				if *insertingPointerToSmtBatchNode == nil {
					break
				}

				// a leaf (with mismatching remaining key) => nothing to collapse
				if (*insertingPointerToSmtBatchNode).isLeaf() {
					break
				}

				// does not have a single leaf => nothing to collapse
				theSingleNodeLeaf, theSingleNodeLeafDirection := (*insertingPointerToSmtBatchNode).getTheSingleLeafAndDirectionIfAny()
				if theSingleNodeLeaf == nil {
					break
				}

				insertingPointerToSmtBatchNode = (*insertingPointerToSmtBatchNode).collapseLeafByRemovingTheSingleLeaf(insertingNodePath, insertingNodePathLevel, theSingleNodeLeaf, theSingleNodeLeafDirection)
				insertingNodePathLevel--
			}
		}
	}

	if smtBatchNodeRoot == nil {
		rootNodeKey = &utils.NodeKey{0, 0, 0, 0}
	} else {
		calculateAndSaveHashesDfs(s, smtBatchNodeRoot, make([]int, 256), 0)
		rootNodeKey = (*utils.NodeKey)(smtBatchNodeRoot.hash)
	}
	if err := s.setLastRoot(*rootNodeKey); err != nil {
		return nil, err
	}

	for _, valueHashToDelete := range valueHashesToDelete {
		s.Db.DeleteByNodeKey(*valueHashToDelete)
	}
	for i, nodeValue := range nodeValues {
		if !nodeValue.IsZero() {
			_, err = s.hashSave(nodeValue.ToUintArray(), utils.BranchCapacity, *nodeValuesHashes[i])
			if err != nil {
				return nil, err
			}
		}
	}

	progressChan <- preprocessStage + uint64(size) + finalizeStage

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

func findInsertingPoint(s *SMT, insertingNodeKey, insertingPointerNodeKey *utils.NodeKey, insertingPointerToSmtBatchNode **smtBatchNode, fetchDirectSiblings bool) (int, []int, **smtBatchNode, error) {
	var err error
	var insertingNodePathLevel int = -1

	var nextInsertingPointerNodeKey *utils.NodeKey
	var nextInsertingPointerToSmtBatchNode **smtBatchNode

	var insertingPointerToSmtBatchNodeParent *smtBatchNode

	insertingNodePath := insertingNodeKey.GetPath()

	for {
		if (*insertingPointerToSmtBatchNode) == nil { // update in-memory structure from db
			if !insertingPointerNodeKey.IsZero() {
				*insertingPointerToSmtBatchNode, err = fetchNodeDataFromDb(s, insertingPointerNodeKey, insertingPointerToSmtBatchNodeParent)
				if err != nil {
					return -2, []int{}, insertingPointerToSmtBatchNode, err
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

		if fetchDirectSiblings {
			// load direct siblings of a non-leaf from the DB
			if (*insertingPointerToSmtBatchNode).leftNode == nil {
				(*insertingPointerToSmtBatchNode).leftNode, err = fetchNodeDataFromDb(s, (*insertingPointerToSmtBatchNode).nodeLeftKeyOrRemainingKey, (*insertingPointerToSmtBatchNode))
				if err != nil {
					return -2, []int{}, insertingPointerToSmtBatchNode, err
				}
			}
			if (*insertingPointerToSmtBatchNode).rightNode == nil {
				(*insertingPointerToSmtBatchNode).rightNode, err = fetchNodeDataFromDb(s, (*insertingPointerToSmtBatchNode).nodeRightKeyOrValueHash, (*insertingPointerToSmtBatchNode))
				if err != nil {
					return -2, []int{}, insertingPointerToSmtBatchNode, err
				}
			}
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

func calculateAndSaveHashesDfs(s *SMT, smtBatchNode *smtBatchNode, path []int, level int) error {
	if smtBatchNode.isLeaf() {
		hashObj, err := s.hashcalcAndSave(utils.ConcatArrays4(*smtBatchNode.nodeLeftKeyOrRemainingKey, *smtBatchNode.nodeRightKeyOrValueHash), utils.LeafCapacity)
		if err != nil {
			return err
		}

		nodeKey := utils.JoinKey(path[:level], *smtBatchNode.nodeRightKeyOrValueHash)
		s.Db.InsertHashKey(hashObj, *nodeKey)

		smtBatchNode.hash = &hashObj
		return nil
	}

	var totalHash utils.NodeValue8

	if smtBatchNode.leftNode != nil {
		path[level+1] = 0
		calculateAndSaveHashesDfs(s, smtBatchNode.leftNode, path, level+1)
		totalHash.SetHalfValue(*smtBatchNode.leftNode.hash, 0)
	} else {
		totalHash.SetHalfValue(*smtBatchNode.nodeLeftKeyOrRemainingKey, 0)
	}

	if smtBatchNode.rightNode != nil {
		path[level+1] = 1
		calculateAndSaveHashesDfs(s, smtBatchNode.rightNode, path, level+1)
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

func fetchNodeDataFromDb(s *SMT, nodeKey *utils.NodeKey, parentNode *smtBatchNode) (*smtBatchNode, error) {
	if nodeKey.IsZero() {
		return nil, nil
	}

	dbNodeValue, err := s.Db.Get(*nodeKey)
	if err != nil {
		return nil, err
	}

	nodeLeftKeyOrRemainingKey := utils.NodeKeyFromBigIntArray(dbNodeValue[0:4])
	nodeRightKeyOrValueHash := utils.NodeKeyFromBigIntArray(dbNodeValue[4:8])
	return &smtBatchNode{
		parentNode:                parentNode,
		nodeLeftKeyOrRemainingKey: &nodeLeftKeyOrRemainingKey,
		nodeRightKeyOrValueHash:   &nodeRightKeyOrValueHash,
		leaf:                      dbNodeValue.IsFinalNode(),
	}, nil
}

func (sbn *smtBatchNode) isLeaf() bool {
	return sbn.leaf
}

func (sbn *smtBatchNode) getTheSingleLeafAndDirectionIfAny() (*smtBatchNode, int) {
	if sbn.leftNode != nil && sbn.rightNode == nil && sbn.leftNode.isLeaf() {
		return sbn.leftNode, 0
	}
	if sbn.leftNode == nil && sbn.rightNode != nil && sbn.rightNode.isLeaf() {
		return sbn.rightNode, 1
	}
	return nil, -1
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

func (sbn *smtBatchNode) updateKeysAfterDelete() {
	if sbn.leftNode == nil {
		sbn.nodeLeftKeyOrRemainingKey = &utils.NodeKey{0, 0, 0, 0}
	}
	if sbn.rightNode == nil {
		sbn.nodeRightKeyOrValueHash = &utils.NodeKey{0, 0, 0, 0}
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

func (sbn *smtBatchNode) expandLeafByAddingALeafInDirection(insertingNodeKey []int, insertingNodeKeyLevel int) **smtBatchNode {
	direction := insertingNodeKey[insertingNodeKeyLevel]
	insertingNodeKeyUpToLevel := insertingNodeKey[:insertingNodeKeyLevel]

	childPointer := sbn.getChildInDirection(direction)

	nodeKey := utils.JoinKey(insertingNodeKeyUpToLevel, *sbn.nodeLeftKeyOrRemainingKey)
	remainingKey := utils.RemoveKeyBits(*nodeKey, insertingNodeKeyLevel+1)

	*childPointer = newSmtBatchNodeLeaf(&remainingKey, sbn.nodeRightKeyOrValueHash, sbn)
	sbn.nodeLeftKeyOrRemainingKey = &utils.NodeKey{0, 0, 0, 0}
	sbn.nodeRightKeyOrValueHash = &utils.NodeKey{0, 0, 0, 0}
	sbn.leaf = false

	return childPointer
}

func (sbn *smtBatchNode) collapseLeafByRemovingTheSingleLeaf(insertingNodeKey []int, insertingNodeKeyLevel int, theSingleLeaf *smtBatchNode, theSingleNodeLeafDirection int) **smtBatchNode {
	insertingNodeKeyUpToLevel := insertingNodeKey[:insertingNodeKeyLevel+1]
	insertingNodeKeyUpToLevel[insertingNodeKeyLevel] = theSingleNodeLeafDirection
	nodeKey := utils.JoinKey(insertingNodeKeyUpToLevel, *theSingleLeaf.nodeLeftKeyOrRemainingKey)
	remainingKey := utils.RemoveKeyBits(*nodeKey, insertingNodeKeyLevel)

	sbn.nodeLeftKeyOrRemainingKey = &remainingKey
	sbn.nodeRightKeyOrValueHash = theSingleLeaf.nodeRightKeyOrValueHash
	sbn.leaf = true
	sbn.leftNode = nil
	sbn.rightNode = nil

	return &sbn.parentNode
}
