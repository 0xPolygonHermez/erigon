package smt

import (
	"math/big"
	"strings"

	"github.com/ledgerwatch/erigon/smt/pkg/utils"
)

func SetAccountState(ethAddr string, smt *SMT, root, balance, nonce *big.Int) (*big.Int, error) {
	keyBalance, err := utils.KeyEthAddrBalance(ethAddr)
	if err != nil {
		return nil, err
	}
	keyNonce, err := utils.KeyEthAddrNonce(ethAddr)
	if err != nil {
		return nil, err
	}

	auxRes, err := smt.InsertKA(root, keyBalance, balance)
	if err != nil {
		return nil, err
	}

	auxRes, err = smt.InsertKA(auxRes.NewRoot, keyNonce, nonce)

	return auxRes.NewRoot, err
}

func SetContractBytecode(ethAddr string, smt *SMT, root *big.Int, bytecode string) (*big.Int, error) {
	keyContractCode, err := utils.KeyContractCode(ethAddr)
	if err != nil {
		return nil, err
	}
	keyContractLength, err := utils.KeyContractLength(ethAddr)
	if err != nil {
		return nil, err
	}

	hashedBytecode, err := utils.HashContractBytecode(bytecode)

	var parsedBytecode string

	if strings.HasPrefix(bytecode, "0x") {
		parsedBytecode = bytecode[2:]
	} else {
		parsedBytecode = bytecode
	}

	if len(parsedBytecode)%2 != 0 {
		parsedBytecode = "0" + parsedBytecode
	}

	bytecodeLength := len(parsedBytecode) / 2

	bi := utils.ConvertHexToBigInt(hashedBytecode)

	r1, err := smt.InsertKA(root, keyContractCode, bi)
	if err != nil {
		return nil, err
	}

	r2, err := smt.InsertKA(r1.NewRoot, keyContractLength, big.NewInt(int64(bytecodeLength)))
	if err != nil {
		return nil, err
	}

	return r2.NewRoot, nil
}

func SetContractStorage(ethAddr string, smt *SMT, root *big.Int, storage map[string]string) (*big.Int, error) {
	i := 0
	tmpRoot := root

	for k, v := range storage {
		keyStoragePosition, err := utils.KeyContractStorage(ethAddr, k)
		if err != nil {
			return nil, err
		}

		base := 10
		if strings.HasPrefix(v, "0x") {
			v = v[2:]
			base = 16
		}

		val, _ := new(big.Int).SetString(v, base)

		auxRes, err := smt.InsertKA(tmpRoot, keyStoragePosition, val)
		if err != nil {
			return nil, err
		}

		tmpRoot = auxRes.NewRoot
		i++
	}

	return tmpRoot, nil
}
