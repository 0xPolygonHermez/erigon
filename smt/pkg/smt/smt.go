package smt

import (
	"math/big"

	"encoding/json"
	"fmt"
	"github.com/ledgerwatch/erigon/smt/pkg/db"
	"github.com/ledgerwatch/erigon/smt/pkg/utils"
	"strings"
)

type DB interface {
	Get(key utils.NodeKey) (utils.NodeValue12, error)
	Insert(key utils.NodeKey, value utils.NodeValue12) error
}

type DebuggableDB interface {
	DB
	PrintDb()
	GetDb() map[string][]string
}

type SMT struct {
	Db DB
}

type SMTResponse struct {
	NewRoot *big.Int
	Mode    string
}

func NewSMT(database DB) *SMT {
	if database == nil {
		database = db.NewMemDb()
	}
	return &SMT{
		Db: database,
	}
}

func (s *SMT) InsertBI(lastRoot *big.Int, key *big.Int, value *big.Int) (*SMTResponse, error) {
	k := utils.ScalarToNodeKey(key)
	v := utils.ScalarToNodeValue8(value)
	return s.Insert(lastRoot, k, v)
}

func (s *SMT) InsertKA(lastRoot *big.Int, key utils.NodeKey, value *big.Int) (*SMTResponse, error) {
	x := utils.ScalarToArrayBig(value)
	v, err := utils.NodeValue8FromBigIntArray(x)
	if err != nil {
		return nil, err
	}

	return s.Insert(lastRoot, key, *v)
}

func (s *SMT) Insert(lastRoot *big.Int, k utils.NodeKey, v utils.NodeValue8) (*SMTResponse, error) {
	oldRoot := utils.ScalarToRoot(lastRoot)
	r := oldRoot
	newRoot := oldRoot

	smtResponse := &SMTResponse{
		NewRoot: big.NewInt(0),
		Mode:    "not run",
	}

	if k.IsZero() && v.IsZero() {
		return smtResponse, nil
	}

	// split the key
	keys := k.GetPath()

	var usedKey []int
	var siblings map[int]*utils.NodeValue12
	var level int
	var foundKey *utils.NodeKey
	var foundVal utils.NodeValue8
	var oldValue utils.NodeValue8
	var foundRKey utils.NodeKey
	var proofHashCounter int
	var foundOldValHash utils.NodeKey
	var insKey *utils.NodeKey
	var insValue utils.NodeValue8
	var isOld0 bool

	siblings = map[int]*utils.NodeValue12{}

	// JS WHILE
	for !r.IsZero() && foundKey == nil {
		sl, err := s.Db.Get(r)
		if err != nil {
			return nil, err
		}
		siblings[level] = &sl
		if siblings[level].IsFinalNode() {
			foundOldValHash = utils.NodeKeyFromBigIntArray(siblings[level][4:8])
			fva, err := s.Db.Get(foundOldValHash)
			if err != nil {
				return nil, err
			}
			foundValA := utils.Value8FromBigIntArray(fva[0:8])
			foundRKey = utils.NodeKeyFromBigIntArray(siblings[level][0:4])
			foundVal = foundValA

			foundKey = utils.JoinKey(usedKey, foundRKey)
			if err != nil {
				return nil, err
			}
		} else {
			r = utils.NodeKeyFromBigIntArray(siblings[level][keys[level]*4 : keys[level]*4+4])
			usedKey = append(usedKey, keys[level])
			level++
		}
	}

	level--
	if len(usedKey) != 0 {
		usedKey = usedKey[:len(usedKey)-1]
	}

	//
	proofHashCounter = 0
	if !oldRoot.IsZero() {
		utils.RemoveOver(siblings, level+1)
		proofHashCounter += len(siblings)
		if foundVal.IsZero() {
			proofHashCounter += 2
		}
	}

	if !v.IsZero() { // we have a value - so we're updating or inserting
		if foundKey != nil {
			if foundKey.IsEqualTo(k) {
				// UPDATE MODE
				smtResponse.Mode = "update"
				oldValue = foundVal

				newValH, err := s.HashSave(v.ToUintArray(), utils.BranchCapacity)
				if err != nil {
					return nil, err
				}
				newLeafHash, err := s.HashSave(utils.ConcatArrays4(foundRKey, newValH), utils.LeafCapacity)
				if err != nil {
					return nil, err
				}
				if level >= 0 {
					for j := 0; j < 4; j++ {
						siblings[level][keys[level]*4+j] = new(big.Int).SetUint64(newLeafHash[j])
					}
				} else {
					newRoot = newLeafHash
				}
			} else {
				smtResponse.Mode = "insertFound"
				// INSERT WITH FOUND KEY
				level2 := level + 1
				foundKeys := foundKey.GetPath()

				for {
					if level2 >= len(keys) || level2 >= len(foundKeys) {
						break
					}

					if keys[level2] != foundKeys[level2] {
						break
					}

					level2++
				}

				oldKey := utils.RemoveKeyBits(*foundKey, level2+1)
				oldLeafHash, err := s.HashSave(utils.ConcatArrays4(oldKey, foundOldValHash), utils.LeafCapacity)
				if err != nil {
					return nil, err
				}

				insKey = foundKey
				insValue = foundVal
				isOld0 = false

				newKey := utils.RemoveKeyBits(k, level2+1)
				newValH, err := s.HashSave(v.ToUintArray(), utils.BranchCapacity)
				if err != nil {
					return nil, err
				}

				newLeafHash, err := s.HashSave(utils.ConcatArrays4(newKey, newValH), utils.LeafCapacity)

				var node [8]uint64
				for i := 0; i < 8; i++ {
					node[i] = 0
				}

				for j := 0; j < 4; j++ {
					node[keys[level2]*4+j] = newLeafHash[j]
					node[foundKeys[level2]*4+j] = oldLeafHash[j]
				}

				r2, err := s.HashSave(node, utils.BranchCapacity)
				if err != nil {
					return nil, err
				}
				proofHashCounter += 4
				level2 -= 1

				for level2 != level {
					for i := 0; i < 8; i++ {
						node[i] = 0
					}

					for j := 0; j < 4; j++ {
						node[keys[level2]*4+j] = r2[j]
					}

					r2, err = s.HashSave(node, utils.BranchCapacity)
					if err != nil {
						return nil, err
					}
					proofHashCounter += 1
					level2 -= 1
				}

				if level >= 0 {
					for j := 0; j < 4; j++ {
						siblings[level][keys[level]*4+j] = new(big.Int).SetUint64(r2[j])
					}
				} else {
					newRoot = r2
				}
			}

		} else {
			// INSERT NOT FOUND
			smtResponse.Mode = "insertNotFound"
			newKey := utils.RemoveKeyBits(k, level+1)

			newValH, err := s.HashSave(v.ToUintArray(), utils.BranchCapacity)
			if err != nil {
				return nil, err
			}

			nk := utils.ConcatArrays4(newKey, newValH)

			newLeafHash, err := s.HashSave(nk, utils.LeafCapacity)
			if err != nil {
				return nil, err
			}

			proofHashCounter += 2

			if level >= 0 {
				for j := 0; j < 4; j++ {
					nlh := big.Int{}
					nlh.SetUint64(newLeafHash[j])
					siblings[level][keys[level]*4+j] = &nlh
				}
			} else {
				newRoot = newLeafHash
			}
		}
	} else if foundKey != nil && foundKey.IsEqualTo(k) { // we don't have a value so we're deleting
		oldValue = foundVal
		if level >= 0 {
			for j := 0; j < 4; j++ {
				siblings[level][keys[level]*4+j] = nil
			}

			uKey, err := siblings[level].IsUniqueSibling()
			if err != nil {
				return nil, err
			}

			if uKey {
				// DELETE FOUND
				smtResponse.Mode = "deleteFound"
				dk := utils.NodeKeyFromBigIntArray(siblings[level][4:8])
				sl, err := s.Db.Get(dk)
				if err != nil {
					return nil, err
				}
				siblings[level+1] = &sl

				if siblings[level+1].IsFinalNode() {
					valH := siblings[level+1].Get4to8()
					preValA, err := s.Db.Get(*valH)
					if err != nil {
						return nil, err
					}
					valA := preValA.Get0to8()
					rKey := siblings[level+1].Get0to4()
					proofHashCounter += 2

					val := utils.ArrayToScalar(valA[:])
					insKey = utils.JoinKey(append(usedKey, 1), *rKey)
					insValue = utils.ScalarToNodeValue8(val)
					isOld0 = false

					for uKey && level >= 0 {
						level -= 1
						if level >= 0 {
							uKey, err = siblings[level].IsUniqueSibling()
							if err != nil {
								return nil, err
							}
						}
					}

					oldKey := utils.RemoveKeyBits(*insKey, level+1)
					oldLeafHash, err := s.HashSave(utils.ConcatArrays4(oldKey, *valH), utils.LeafCapacity)
					if err != nil {
						return nil, err
					}
					proofHashCounter += 1

					if level >= 0 {
						for j := 0; j < 4; j++ {
							siblings[level][keys[level]*4+j] = new(big.Int).SetUint64(oldLeafHash[j])
						}
					} else {
						newRoot = oldLeafHash
					}
				} else {
					// DELETE NOT FOUND
					smtResponse.Mode = "deleteNotFound"
				}
			} else {
				// DELETE NOT FOUND
				smtResponse.Mode = "deleteNotFound"
			}
		} else {
			// DELETE LAST
			smtResponse.Mode = "deleteLast"
			newRoot = utils.NodeKey{0, 0, 0, 0}
		}
	} else { // we're going zero to zero - do nothing
		smtResponse.Mode = "zeroToZero"
		if foundKey != nil {
			insKey = foundKey
			insValue = foundVal
			isOld0 = false
		}
	}

	utils.RemoveOver(siblings, level+1)

	for level >= 0 {
		hashValueIn, err := utils.NodeValue8FromBigIntArray(siblings[level][0:8])
		hashCapIn := utils.NodeKeyFromBigIntArray(siblings[level][8:12])
		newRoot, err = s.HashSave(hashValueIn.ToUintArray(), hashCapIn)
		if err != nil {
			return nil, err
		}
		proofHashCounter += 1
		level -= 1
		if level >= 0 {
			for j := 0; j < 4; j++ {
				nrj := big.Int{}
				nrj.SetUint64(newRoot[j])
				siblings[level][keys[level]*4+j] = &nrj
			}
		}
	}

	_ = r
	_ = oldValue
	_ = insValue
	_ = isOld0

	smtResponse.NewRoot = newRoot.ToBigInt()

	return smtResponse, nil
}

func (s *SMT) HashSave(in [8]uint64, capacity [4]uint64) ([4]uint64, error) {
	h, err := utils.Hash(in, capacity)
	if err != nil {
		return [4]uint64{}, err
	}

	var sl []uint64

	sl = append(sl, in[:]...)
	sl = append(sl, capacity[:]...)

	v := utils.NodeValue12{}
	for i, val := range sl {
		b := new(big.Int)
		v[i] = b.SetUint64(val)
	}

	err = s.Db.Insert(h, v)
	return h, err
}

// Utility functions for debugging

func (s *SMT) PrintDb() {
	if debugDB, ok := s.Db.(DebuggableDB); ok {
		debugDB.PrintDb()
	}
}

func (s *SMT) PrintTree() {
	if debugDB, ok := s.Db.(DebuggableDB); ok {
		data := debugDB.GetDb()
		str, err := json.Marshal(data)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println(string(str))
	}
}

func removeNonValueNodes(data map[string][]string) map[string][]string {
	valueNodes := make(map[string][]string)
	for k, v := range data {
		isValue := true
		for _, val := range v[0:8] {
			// remove hex prefix
			valStr := strings.TrimPrefix(val, "0x")
			if len(valStr)*4 > 32 {
				isValue = false
				break
			}
		}

		if isValue {
			valueNodes[k] = v[0:8]
		}
	}
	return valueNodes
}
