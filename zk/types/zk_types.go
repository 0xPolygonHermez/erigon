package types

import types "github.com/ledgerwatch/erigon-lib/common"

type L1BatchInfo struct {
	BatchNo   uint64
	L1BlockNo uint64
	L1TxHash  types.Hash
	StateRoot types.Hash
}
