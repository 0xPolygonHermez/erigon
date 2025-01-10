package chain

import (
	"encoding/json"
	"fmt"
	"github.com/ledgerwatch/erigon-lib/chain"
	"github.com/ledgerwatch/erigon/zk/config"
	"os"
	"path"
)

func UnionChain(ch string) *chain.Config {
	var chainCfg chain.Config
	p := path.Join(config.DynamicChainConfigPath, ch+"-cfg.json")
	if config.ZkConfigPath != "" || len(config.ZkConfigPath) != 0 {
		p = config.ZkConfigPath
	}
	union := config.UnmarshalUnionConfig(p)
	if err := json.Unmarshal(union["chainspec"], &chainCfg); err != nil {
		panic(fmt.Sprintf("could not parse chain for %s: %v", ch, err))
	}
	return &chainCfg
}

func DynamicChain(ch string) *chain.Config {
	filename := path.Join(config.DynamicChainConfigPath, ch+"-chainspec.json")

	f, err := os.Open(filename)
	if err != nil {
		panic(fmt.Sprintf("could not open chainspec for %s: %v", filename, err))
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	spec := &chain.Config{}
	err = decoder.Decode(&spec)
	if err != nil {
		panic(fmt.Sprintf("could not parse chainspec for %s: %v", filename, err))
	}

	chainId := spec.ChainID.Uint64()
	chain.SetDynamicChainDetails(chainId, spec.ChainName)

	return spec
}
