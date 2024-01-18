package smt

import "github.com/ledgerwatch/erigon/smt/pkg/utils"

func (s *SMT) insertBatch(k []*utils.NodeKey, v []*utils.NodeValue8, newValH []*[4]uint64, oldRoot utils.NodeKey) (*SMTResponse, error) {
	smtResponse := &SMTResponse{
		Mode: "not run",
	}

	return smtResponse, nil
}

/*
NodeKey -> Value
NodeKey { Hash(rKey, Hash(Value)) } -> rKey, Hash(Value), 1
NodeKey { Hash(LeftNodeKey, RightNodeKey) } -> LeftNodeKey, RightNodeKey, 0
*/
