package smt

import (
	"math/big"
	"strings"

	"github.com/ledgerwatch/erigon/smt/pkg/utils"
)

func (s *SMT) SetAccountState(ethAddr string, balance, nonce *big.Int) (*big.Int, error) {
	keyBalance, err := utils.KeyEthAddrBalance(ethAddr)
	if err != nil {
		return nil, err
	}
	keyNonce, err := utils.KeyEthAddrNonce(ethAddr)
	if err != nil {
		return nil, err
	}

	auxRes, err := s.InsertKA(keyBalance, balance)
	if err != nil {
		return nil, err
	}

	auxRes, err = s.InsertKA(keyNonce, nonce)

	return auxRes.NewRoot, err
}

func (s *SMT) SetContractBytecode(ethAddr string, bytecode string) error {
	keyContractCode, err := utils.KeyContractCode(ethAddr)
	if err != nil {
		return err
	}
	keyContractLength, err := utils.KeyContractLength(ethAddr)
	if err != nil {
		return err
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

	_, err = s.InsertKA(keyContractCode, bi)
	if err != nil {
		return err
	}

	_, err = s.InsertKA(keyContractLength, big.NewInt(int64(bytecodeLength)))
	if err != nil {
		return err
	}

	return nil
}

func (s *SMT) SetContractStorage(ethAddr string, storage map[string]string) (*big.Int, error) {
	i := 0

	var auxRes *SMTResponse

	for k, v := range storage {
		// start timer
		//start := time.Now()

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

		auxRes, err = s.InsertKA(keyStoragePosition, val)
		if err != nil {
			return nil, err
		}

		i++
		//end := time.Now()
		//elapsed := end.Sub(start)
		//fmt.Println("Elapsed time: ", elapsed)
	}

	return auxRes.NewRoot, nil
}
