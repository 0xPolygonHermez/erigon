package config

import (
	"encoding/json"
	"fmt"
	"github.com/ledgerwatch/erigon-lib/chain"
	"github.com/ledgerwatch/erigon/core/types"
	"io"
	"os"
	"path"
	"path/filepath"
)

var ZkConfigPath string
var DynamicChainConfigPath string

type DynamicConfig struct {
	Root       string `json:"root"`
	Timestamp  uint64 `json:"timestamp"`
	GasLimit   uint64 `json:"gasLimit"`
	Difficulty int64  `json:"difficulty"`
}

type ZKConfig struct {
	ChainCfg   *chain.Config
	DynamicCfg DynamicConfig
	AllocCfg   types.GenesisAlloc
}

func NewZKConfig(ch string) *ZKConfig {
	unionPath := path.Join(DynamicChainConfigPath, ch+"-cfg.json")
	if ZkConfigPath != "" || len(ZkConfigPath) != 0 {
		unionPath = ZkConfigPath
	}
	if dynamicUnionConfigExists(unionPath) {
		return zkUnionConfig(ch)
	} else {
		return zkDynamicConfig(ch)
	}
}

func zkUnionConfig(ch string) *ZKConfig {
	return &ZKConfig{
		ChainCfg:   unionChain(ch),
		DynamicCfg: unionDynamic(ch),
		AllocCfg:   unionAlloc(ch),
	}
}

func unionChain(ch string) *chain.Config {
	var chainCfg chain.Config
	p := path.Join(DynamicChainConfigPath, ch+"-cfg.json")
	if ZkConfigPath != "" || len(ZkConfigPath) != 0 {
		p = ZkConfigPath
	}
	union := UnmarshalUnionConfig(p)
	if err := json.Unmarshal(union["chainspec"], &chainCfg); err != nil {
		panic(fmt.Sprintf("could not parse chain for %s: %v", ch, err))
	}
	return &chainCfg
}

func unionDynamic(ch string) DynamicConfig {
	var dyn DynamicConfig
	p := path.Join(DynamicChainConfigPath, ch+"-cfg.json")
	if ZkConfigPath != "" || len(ZkConfigPath) != 0 {
		p = ZkConfigPath
	}
	union := UnmarshalUnionConfig(p)
	if err := json.Unmarshal(union["conf"], &dyn); err != nil {
		panic(fmt.Sprintf("could not parse conf for %s: %v", ch, err))
	}
	return dyn
}

func unionAlloc(ch string) types.GenesisAlloc {
	var alloc types.GenesisAlloc
	p := path.Join(DynamicChainConfigPath, ch+"-cfg.json")
	if ZkConfigPath != "" || len(ZkConfigPath) != 0 {
		p = ZkConfigPath
	}
	union := UnmarshalUnionConfig(p)
	if err := json.Unmarshal(union["allocs"], &alloc); err != nil {
		panic(fmt.Sprintf("could not parse alloc for %s: %v", ch, err))
	}
	return alloc
}

func dynamicUnionConfigExists(filename string) bool {
	fileInfo, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	if err != nil {
		return false
	}
	if fileInfo.IsDir() || filepath.Ext(fileInfo.Name()) != ".json" {
		return false
	}
	return true
}

func UnmarshalUnionConfig(filename string) map[string]json.RawMessage {
	file, err := os.Open(filename)
	if err != nil {
		panic(fmt.Sprintf("could not open union config for %s: %v", filename, err))
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		panic(fmt.Sprintf("could not read union config for %s: %v", filename, err))
	}

	var union map[string]json.RawMessage
	if err = json.Unmarshal(data, &union); err != nil {
		panic(fmt.Sprintf("could not parse union config for %s: %v", filename, err))
	}

	return union
}

func zkDynamicConfig(ch string) *ZKConfig {
	return &ZKConfig{
		ChainCfg:   dynamicChain(ch),
		DynamicCfg: dynamicCfg(ch),
		AllocCfg:   dynamicAlloc(ch),
	}
}

func dynamicChain(ch string) *chain.Config {
	filename := path.Join(DynamicChainConfigPath, ch+"-chainspec.json")

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

func dynamicCfg(ch string) DynamicConfig {
	filename := path.Join(DynamicChainConfigPath, ch+"-conf.json")

	f, err := os.Open(filename)
	if err != nil {
		panic(fmt.Sprintf("could not open timestamp for %s: %v", filename, err))
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	conf := DynamicConfig{}
	err = decoder.Decode(&conf)
	if err != nil {
		panic(fmt.Sprintf("could not parse timestamp for %s: %v", filename, err))
	}

	return conf
}

func dynamicAlloc(ch string) types.GenesisAlloc {
	filename := path.Join(DynamicChainConfigPath, ch+"-allocs.json")

	f, err := os.Open(filename)
	if err != nil {
		panic(fmt.Sprintf("could not open alloc for %s: %v", filename, err))
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	alloc := make(types.GenesisAlloc)
	err = decoder.Decode(&alloc)
	if err != nil {
		panic(fmt.Sprintf("could not parse alloc for %s: %v", filename, err))
	}

	return alloc
}
