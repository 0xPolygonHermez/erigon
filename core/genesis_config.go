package core

import (
	"github.com/ledgerwatch/erigon-lib/chain"
	"github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon/core/types"

	"github.com/ledgerwatch/erigon/params"
)

func ConfigOrDefault(g *types.Genesis, genesisHash common.Hash) *chain.Config {
	if g != nil {
		return g.Config
	}

	config := params.ChainConfigByGenesisHash(genesisHash)
	if config != nil {
		return config
	} else {
		return params.AllProtocolChanges
	}
}
