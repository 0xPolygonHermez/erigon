package main

import (
	"fmt"
	"math/big"

	"github.com/revitteth/smt/v2/pkg/db"
	"github.com/revitteth/smt/v2/pkg/smt"
)

func main() {

	mdb := db.NewMemDb()
	s := smt.NewSMT(mdb)

	r, err := s.InsertBI(big.NewInt(0), big.NewInt(0), big.NewInt(1))
	if err != nil {
		panic(err)
	}

	hex := fmt.Sprintf("0x%0x", r.NewRoot)

	fmt.Printf(hex)
}
