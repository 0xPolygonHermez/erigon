package smt

import (
	"fmt"

	"github.com/ledgerwatch/erigon/smt/pkg/utils"
)

// sorts the keys and builds a binary tree from left
// this makes it so the left part of a node can be deleted once it's right part is inserted
// this is because the left part is at its final spot
// when deleting nodes, go down to the leaf and create and save hashes in the SMT
func (s *SMT) GenerateFromKVBulk(kvMap map[utils.NodeKey]utils.NodeValue8) ([4]uint64, error) {
	s.clearUpMutex.Lock()
	defer s.clearUpMutex.Unlock()

	// get nodeKeys and sort them bitwise
	nodeKeys := []utils.NodeKey{}

	for k := range kvMap {
		v := kvMap[k]
		if v.IsZero() {
			continue
		}
		nodeKeys = append(nodeKeys, k)
	}
	//TODO: can sort without converting
	utils.SortNodeKeysBitwiseAsc(nodeKeys)

	rootNode := SmtNode{
		leftHash: [4]uint64{},
		node0:    nil,
		node1:    nil,
	}

	for _, k := range nodeKeys {
		// split the key
		keys := k.GetPath()
		// find last node
		siblings, level := rootNode.findLastNode(keys)

		//if last found node is leaf
		//1. create nodes till the different bit
		//2. insert both leafs there
		//3. save left leaf with hash
		if len(siblings) > 0 && siblings[level].isLeaf() {
			//take the leaf to insert it later where it needs to be
			leaf0 := siblings[len(siblings)-1]
			bit := keys[level]

			//add node, this should always occur in this scenario

			///take the node above the leaf, so we can set its left/right and continue the tree
			var upperNode *SmtNode
			if level == 0 {
				upperNode = &rootNode
			} else {
				upperNode = siblings[len(siblings)-2]
			}

			///set the new node depending on the bit and insert it into siblings
			///now it is the last sibling
			newNode := &SmtNode{}
			if bit == 0 {
				upperNode.node0 = newNode
			} else {
				upperNode.node1 = newNode
			}
			siblings = append(siblings, newNode)
			level++

			//create nodes till the different bit
			level2 := 0
			for leaf0.rKey[level2] == keys[level+level2] {
				newNode := SmtNode{}
				if keys[level+level2] == 0 {
					siblings[len(siblings)-1].node0 = &newNode
				} else {
					siblings[len(siblings)-1].node1 = &newNode
				}
				siblings = append(siblings, &newNode)
				level2++
			}

			//sanity check - new leaf should be on the right side
			//otherwise something went wrong
			if leaf0.rKey[level2] != 0 || keys[level2+level] != 1 {
				return [4]uint64{}, fmt.Errorf(
					"leaf insert error. new leaf should be on the right of the old, oldLeaf: %v, newLeaf: %v",
					append(keys[:level+1], leaf0.rKey[level2:]...),
					keys,
				)
			}

			//insert both leaf nodes in last node
			// r key is reduced by how many nodes were added
			leaf0.rKey = leaf0.rKey[level2+1:]
			siblings[len(siblings)-1].node0 = leaf0
			siblings[len(siblings)-1].node1 = &SmtNode{
				rKey: keys[level+level2+1:],
			}

			//hash, save and delete left leaf
			pathToDeleteFrom := make([]int, level+level2+1)
			copy(pathToDeleteFrom, keys[:level+level2])
			pathToDeleteFrom[level+level2] = 0
			_, leftHash, err := siblings[len(siblings)-1].node0.deleteTree(pathToDeleteFrom, s, kvMap)
			if err != nil {
				return [4]uint64{}, err
			}

			siblings[len(siblings)-1].leftHash = leftHash
			siblings[len(siblings)-1].node0 = nil
		} else
		// if it is not leaf
		// insert the new leaf on the right side
		// save left side
		{
			var upperNode *SmtNode
			//upper node is root node
			if len(siblings) == 0 {
				upperNode = &rootNode
			} else {
				//root is not counted as level, so inserting under it will always be zero
				//in other cases increment level, so it corresponds to the new step down
				level++
				upperNode = siblings[len(siblings)-1]
			}

			newNode := &SmtNode{
				rKey: keys[level+1:],
			}

			// this is case for 1 leaf inserted to the left of the root node
			if len(siblings) == 0 && keys[0] == 0 {
				if upperNode.node0 != nil {
					return [4]uint64{}, fmt.Errorf("tried to override left node")
				}
				upperNode.node0 = newNode
			} else {
				//sanity check
				//found node should be on the left side
				//the new leaf should be on the right side
				//otherwise something went wrong
				if upperNode.node1 != nil || keys[level] != 1 {
					return [4]uint64{}, fmt.Errorf(
						"leaf insert error. new should be on the right of the found node, foundNode: %v, newLeafKey: %v",
						upperNode.node1,
						keys,
					)
				}

				upperNode.node1 = newNode

				//hash, save and delete left leaf
				if upperNode.node0 != nil {
					pathToDeleteFrom := make([]int, level+1)
					copy(pathToDeleteFrom, keys[:level])
					pathToDeleteFrom[level] = 0
					_, leftHash, err := upperNode.node0.deleteTree(pathToDeleteFrom, s, kvMap)
					if err != nil {
						return [4]uint64{}, err
					}
					upperNode.leftHash = leftHash
					upperNode.node0 = nil
				}
			}
		}
	}

	//special case where no values were inserted
	if rootNode.isLeaf() {
		return [4]uint64{}, nil
	}

	//if the root node has only one branch, that branch should become the root node
	var pathToDeleteFrom []int
	if rootNode.node1 == nil {
		rootNode = *rootNode.node0
		pathToDeleteFrom = append(pathToDeleteFrom, 0)
	} else if rootNode.node0 == nil && utils.IsArrayUint64Empty(rootNode.leftHash[:]) {
		rootNode = *rootNode.node1
		pathToDeleteFrom = append(pathToDeleteFrom, 1)
	}

	//if the branch is a leaf, the rkey is the whole key
	if rootNode.isLeaf() {
		newRkey := []int{pathToDeleteFrom[0]}
		pathToDeleteFrom = []int{}
		newRkey = append(newRkey, rootNode.rKey...)
		rootNode.rKey = newRkey
	}

	_, finalRoot, err := rootNode.deleteTree(pathToDeleteFrom, s, kvMap)
	if err != nil {
		return [4]uint64{}, err
	}

	if err := s.setLastRoot(finalRoot); err != nil {
		return [4]uint64{}, err
	}

	return finalRoot, nil
}

type SmtNode struct {
	rKey     []int
	leftHash [4]uint64
	node0    *SmtNode
	node1    *SmtNode
}

func (n *SmtNode) isLeaf() bool {
	return n.node0 == nil && n.node1 == nil && utils.IsArrayUint64Empty(n.leftHash[:])
}

// go down the tree and return last matching node and it's level
// returns level 0 and empty siblings if the last node is the root node
func (n *SmtNode) findLastNode(keys []int) ([]*SmtNode, int) {
	var siblings []*SmtNode
	level := 0
	currentNode := n

	for {
		bit := keys[level]
		if bit == 0 {
			currentNode = currentNode.node0
		} else {
			currentNode = currentNode.node1
		}

		if currentNode == nil {
			if level > 0 {
				level--
			}
			break
		}

		siblings = append(siblings, currentNode)
		if currentNode.isLeaf() {
			break
		}

		level++

	}

	return siblings, level
}

func (n *SmtNode) deleteTree(keyPath []int, s *SMT, kvMap map[utils.NodeKey]utils.NodeValue8) ([]utils.NodeKey, [4]uint64, error) {
	deletedKeys := []utils.NodeKey{}

	if n.isLeaf() {
		fullKey := append(keyPath, n.rKey...)
		k, err := utils.NodeKeyFromPath(fullKey)
		if err != nil {
			return nil, [4]uint64{}, err
		}
		v := kvMap[k]

		deletedKeys = append(deletedKeys, k)
		delete(kvMap, k)

		newKey := utils.RemoveKeyBits(k, len(keyPath))
		//hash and save leaf
		newLeafHash, err := s.createNewLeaf(k, newKey, v)
		if err != nil {
			return nil, [4]uint64{}, err
		}
		return deletedKeys, newLeafHash, nil
	}

	var totalHash utils.NodeValue8

	if n.node0 != nil {
		if !utils.IsArrayUint64Empty(n.leftHash[:]) {
			return nil, [4]uint64{}, fmt.Errorf("node has previously deleted left part")
		}
		localKeyPath := append(keyPath, 0)
		keysFromBelow, leftHash, err := n.node0.deleteTree(localKeyPath, s, kvMap)
		if err != nil {
			return nil, [4]uint64{}, err
		}

		n.leftHash = leftHash
		deletedKeys = append(deletedKeys, keysFromBelow...)
		n.node0 = nil
	}

	if n.node1 != nil {
		localKeyPath := append(keyPath, 1)
		keysFromBelow, rightHash, err := n.node1.deleteTree(localKeyPath, s, kvMap)
		if err != nil {
			return nil, [4]uint64{}, err
		}
		totalHash.SetHalfValue(rightHash, 1)

		deletedKeys = append(deletedKeys, keysFromBelow...)
		n.node1 = nil
	}

	totalHash.SetHalfValue(n.leftHash, 0)

	newRoot, err := s.hashcalcAndSave(totalHash.ToUintArray(), utils.BranchCapacity)
	if err != nil {
		return nil, [4]uint64{}, err
	}

	return deletedKeys, newRoot, nil
}

func (s *SMT) createNewLeaf(k, rkey utils.NodeKey, v utils.NodeValue8) ([4]uint64, error) {
	//hash and save leaf
	newValH, err := s.hashcalcAndSave(v.ToUintArray(), utils.BranchCapacity)
	if err != nil {
		return [4]uint64{}, err
	}

	newLeafHash, err := s.hashcalcAndSave(utils.ConcatArrays4(rkey, newValH), utils.LeafCapacity)
	if err != nil {
		return [4]uint64{}, err
	}

	return newLeafHash, nil
}
