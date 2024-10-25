package smt

import (
	"bytes"
	"context"
	"math/big"

	"github.com/holiman/uint256"
	libcommon "github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon/core/types/accounts"
	"github.com/ledgerwatch/erigon/smt/pkg/utils"
	"github.com/ledgerwatch/erigon/zkevm/log"
)

// ReadAccountData reads account data from the SMT
func (s *SMT) ReadAccountData(address libcommon.Address) (*accounts.Account, error) {
	account := accounts.Account{}

	balance, err := s.GetAccountBalance(address)
	if err != nil {
		return nil, err
	}
	account.Balance = *balance

	nonce, err := s.GetAccountNonce(address)
	if err != nil {
		return nil, err
	}
	account.Nonce = nonce.Uint64()

	codeHash, err := s.GetAccountCodeHash(address)
	if err != nil {
		return nil, err
	}
	account.CodeHash = codeHash

	account.Root = libcommon.Hash{}

	return &account, nil
}

// ReadAccountStorage reads account storage from the SMT (not implemented for SMT)
func (s *SMT) ReadAccountStorage(address libcommon.Address, incarnation uint64, key *libcommon.Hash) ([]byte, error) {
	value, err := s.getValue(0, address, key)
	if err != nil {
		return []byte{}, err
	}

	return value, nil
}

// ReadAccountCode reads account code from the SMT
func (s *SMT) ReadAccountCode(address libcommon.Address, incarnation uint64, codeHash libcommon.Hash) ([]byte, error) {
	code, err := s.Db.GetCode(codeHash.Bytes())
	if err != nil {
		return []byte{}, err
	}

	return code, nil
}

// ReadAccountCodeSize reads account code size from the SMT
func (s *SMT) ReadAccountCodeSize(address libcommon.Address, incarnation uint64, codeHash libcommon.Hash) (int, error) {
	valueInBytes, err := s.getValue(utils.SC_LENGTH, address, nil)
	if err != nil {
		return 0, err
	}

	sizeBig := big.NewInt(0).SetBytes(valueInBytes)

	return int(sizeBig.Int64()), nil
}

// ReadAccountIncarnation reads account incarnation from the SMT (not implemented for SMT)
func (s *SMT) ReadAccountIncarnation(address libcommon.Address) (uint64, error) {
	return 0, nil
}

// GetAccountBalance returns the balance of an account from the SMT
func (s *SMT) GetAccountBalance(address libcommon.Address) (*uint256.Int, error) {
	balance := uint256.NewInt(0)

	valueInBytes, err := s.getValue(utils.KEY_BALANCE, address, nil)
	if err != nil {
		log.Error("error getting balance", "error", err)
		return nil, err
	}
	balance.SetBytes(valueInBytes)

	return balance, nil
}

// GetAccountNonce returns the nonce of an account from the SMT
func (s *SMT) GetAccountNonce(address libcommon.Address) (*uint256.Int, error) {
	nonce := uint256.NewInt(0)

	valueInBytes, err := s.getValue(utils.KEY_NONCE, address, nil)
	if err != nil {
		log.Error("error getting nonce", "error", err)
		return nil, err
	}
	nonce.SetBytes(valueInBytes)

	return nonce, nil
}

// GetAccountCodeHash returns the code hash of an account from the SMT
func (s *SMT) GetAccountCodeHash(address libcommon.Address) (libcommon.Hash, error) {
	codeHash := libcommon.Hash{}

	valueInBytes, err := s.getValue(utils.SC_CODE, address, nil)
	if err != nil {
		log.Error("error getting codehash", "error", err)
		return libcommon.Hash{}, err
	}
	codeHash.SetBytes(valueInBytes)

	return codeHash, nil
}

// getValue returns the value of a key from SMT by traversing the SMT
func (s *SMT) getValue(key int, address libcommon.Address, storageKey *libcommon.Hash) ([]byte, error) {
	var kn utils.NodeKey

	if storageKey == nil {
		kn = utils.Key(address.String(), key)
	} else {
		a := utils.ConvertHexToBigInt(address.String())
		add := utils.ScalarToArrayBig(a)

		kn = utils.KeyContractStorage(add, storageKey.String())
	}

	return s.getValueInBytes(kn)
}

// getValueInBytes returns the value of a key from SMT in bytes by traversing the SMT
func (s *SMT) getValueInBytes(nodeKey utils.NodeKey) ([]byte, error) {
	value := []byte{}

	keyPath := nodeKey.GetPath()

	keyPathBytes := make([]byte, 0)
	for _, k := range keyPath {
		keyPathBytes = append(keyPathBytes, byte(k))
	}

	action := func(prefix []byte, k utils.NodeKey, v utils.NodeValue12) (bool, error) {
		if !bytes.HasPrefix(keyPathBytes, prefix) {
			return false, nil
		}

		if v.IsFinalNode() {
			valHash := v.Get4to8()
			v, err := s.Db.Get(*valHash)
			if err != nil {
				return false, err
			}
			vInBytes := utils.ArrayBigToScalar(utils.BigIntArrayFromNodeValue8(v.GetNodeValue8())).Bytes()

			value = vInBytes
			return false, nil
		}

		return true, nil
	}

	root, err := s.Db.GetLastRoot()
	if err != nil {
		return nil, err
	}

	err = s.Traverse(context.Background(), root, action)
	if err != nil {
		return nil, err
	}

	return value, nil
}
