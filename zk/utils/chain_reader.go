package utils

import (
	"context"
	libcommon "github.com/ledgerwatch/erigon-lib/common"
	"github.com/ledgerwatch/erigon-lib/kv"
	"github.com/ledgerwatch/erigon/chain"
	"github.com/ledgerwatch/erigon/core/rawdb"
	"github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/turbo/services"
	"github.com/ledgerwatch/log/v3"
	"math/big"
)

type ZkChainReader struct {
	config      *chain.Config
	tx          kv.Getter
	blockReader services.FullBlockReader
}

func NewZkChainReader(config *chain.Config, tx kv.Getter, blockReader services.FullBlockReader) *ZkChainReader {
	return &ZkChainReader{config, tx, blockReader}
}

func (cr ZkChainReader) Config() *chain.Config        { return cr.config }
func (cr ZkChainReader) CurrentHeader() *types.Header { panic("") }
func (cr ZkChainReader) GetHeader(hash libcommon.Hash, number uint64) *types.Header {
	if cr.blockReader != nil {
		h, _ := cr.blockReader.Header(context.Background(), cr.tx, hash, number)
		return h
	}
	return rawdb.ReadHeader(cr.tx, hash, number)
}
func (cr ZkChainReader) GetHeaderByNumber(number uint64) *types.Header {
	if cr.blockReader != nil {
		h, _ := cr.blockReader.HeaderByNumber(context.Background(), cr.tx, number)
		return h
	}
	return rawdb.ReadHeaderByNumber(cr.tx, number)

}
func (cr ZkChainReader) GetHeaderByHash(hash libcommon.Hash) *types.Header {
	if cr.blockReader != nil {
		number := rawdb.ReadHeaderNumber(cr.tx, hash)
		if number == nil {
			return nil
		}
		return cr.GetHeader(hash, *number)
	}
	h, _ := rawdb.ReadHeaderByHash(cr.tx, hash)
	return h
}
func (cr ZkChainReader) GetTd(hash libcommon.Hash, number uint64) *big.Int {
	td, err := rawdb.ReadTd(cr.tx, hash, number)
	if err != nil {
		log.Error("ReadTd failed", "err", err)
		return nil
	}
	return td
}
