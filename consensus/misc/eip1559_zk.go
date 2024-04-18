package misc

import (
	"math/big"
	"github.com/ledgerwatch/erigon/chain"
	"github.com/ledgerwatch/erigon/core/types"
)

func CalcBaseFeeZk(config *chain.Config, parent *types.Header) *big.Int {
	if config.SupportZeroGas {
		return big.NewInt(0)
	}

	return CalcBaseFee(config, parent)
}
