package smt

import (
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/holiman/uint256"
	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon/core/state"
	"github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/core/types/accounts"
	"github.com/ledgerwatch/erigon/smt/pkg/utils"
	"github.com/ledgerwatch/erigon/turbo/trie"
)

var _ state.StateWriter = (*SMT)(nil)

func (s *SMT) UpdateAccountData(address common.Address, _ *accounts.Account, account *accounts.Account) error {
	return s.SetAccountStorage(address, account)
}

func (s *SMT) UpdateAccountCode(address common.Address, _ uint64, _ common.Hash, code []byte) error {
	if len(code) == 0 {
		return nil
	}

	return s.SetContractBytecode(address.Hex(), fmt.Sprintf("0x%s", hex.EncodeToString(code)))
}

func (s *SMT) DeleteAccount(address common.Address, original *accounts.Account) error {
	return errors.New("DeleteAccount is not implemented for the SMT")
}

func (s *SMT) WriteAccountStorage(address common.Address, _ uint64, key *common.Hash, _ *uint256.Int, value *uint256.Int) error {
	if key == nil || value == nil {
		return nil
	}

	storageKeyString := fmt.Sprintf("0x%s", hex.EncodeToString(key.Bytes()))
	storageMap := map[string]string{storageKeyString: value.Hex()}
	_, err := s.SetContractStorage(address.Hex(), storageMap, nil)

	return err
}

func (s *SMT) CreateContract(address common.Address) error {
	return s.SetAccountStorage(address, nil)
}

// ApplyTraces applies a map of traces on the given SMT and returns new instance of SMT, without altering the original one.
func (s *SMT) ApplyTraces(traces map[common.Address]*types.TxnTrace, rd trie.RetainDecider) (*SMT, error) {
	// result, err := s.Copy(context.Background(), rd)
	// if err != nil {
	// 	return nil, err
	// }

	result := s

	for addr, trace := range traces {
		if trace.SelfDestructed != nil && *trace.SelfDestructed {
			nodeVal, err := s.getValue(utils.KEY_NONCE, addr, nil)
			if err != nil {
				return nil, err
			}

			if len(nodeVal) > 0 {
				return nil, fmt.Errorf("account %s is annotated to be self-destructed, but it already exists in the SMT", addr)
			}

			continue
		}

		addrString := addr.Hex()

		// Set account balance and nonce
		if trace.Balance != nil {
			balanceBig := trace.Balance.ToBig()

			if _, err := result.SetAccountBalance(addrString, balanceBig); err != nil {
				return nil, err
			}
		}

		if trace.Nonce != nil {
			nonceBig := trace.Nonce.ToBig()

			if _, err := result.SetAccountNonce(addrString, nonceBig); err != nil {
				return nil, err
			}
		}

		// Set account storage map
		storageMap := make(map[string]string)
		for hash, storageSlot := range trace.StorageWritten {
			storageKey := fmt.Sprintf("0x%s", hex.EncodeToString(hash[:]))
			storageMap[storageKey] = storageSlot.Hex()
		}

		if len(storageMap) > 0 {
			if _, err := result.SetContractStorage(addrString, storageMap, nil); err != nil {
				return nil, err
			}
		}

		// Set account bytecode
		if trace.CodeUsage != nil {
			if trace.CodeUsage.Write != nil {
				if err := result.SetContractBytecode(addrString, hex.EncodeToString(trace.CodeUsage.Write)); err != nil {
					return nil, err
				}
			}
		}
	}

	return result, nil
}
