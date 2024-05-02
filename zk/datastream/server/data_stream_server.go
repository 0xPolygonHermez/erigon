package server

import (
	"bytes"
	"encoding/binary"

	"github.com/0xPolygonHermez/zkevm-data-streamer/datastreamer"
	libcommon "github.com/gateway-fm/cdk-erigon-lib/common"
	eritypes "github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/zk/datastream/types"
	"github.com/ledgerwatch/erigon/zk/hermez_db"
	"github.com/ledgerwatch/erigon/zk/datastream/proto/github.com/0xPolygonHermez/zkevm-node/state/datastream"
)

type BookmarkType byte

var BlockBookmarkType BookmarkType = 0
var BatchBookmarkType BookmarkType = 1

type OperationMode int

const (
	StandardOperationMode OperationMode = iota
	ExecutorOperationMode
)

type DataStreamServer struct {
	stream  *datastreamer.StreamServer
	chainId uint64
	mode    OperationMode
}

type DataStreamEntry interface {
	EntryType() types.EntryType
	Bytes(bigEndian bool) []byte
}

type DataStreamEntryProto interface {
	Marshal() ([]byte, error)
	Type() types.EntryType
}

func NewDataStreamServer(stream *datastreamer.StreamServer, chainId uint64, mode OperationMode) *DataStreamServer {
	return &DataStreamServer{
		stream:  stream,
		chainId: chainId,
		mode:    mode,
	}
}

func (srv *DataStreamServer) CommitEntriesToStreamProto(entries []DataStreamEntryProto) error {
	for _, entry := range entries {
		entryType := entry.Type()

		em, err := entry.Marshal()
		if err != nil {
			return err
		}

		if entryType == types.BookmarkEntryType {
			_, err = srv.stream.AddStreamBookmark(em)
			if err != nil {
				return err
			}
		} else {
			_, err = srv.stream.AddStreamEntry(datastreamer.EntryType(entryType), em)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (srv *DataStreamServer) CreateBatchBookmarkEntryProto(batchNo uint64) *types.BookmarkProto {
	return &types.BookmarkProto{
		BookMark: &datastream.BookMark{
			Type:  datastream.BookmarkType_BOOKMARK_TYPE_BATCH,
			Value: batchNo,
		},
	}
}

func (srv *DataStreamServer) CreateL2BlockBookmarkEntryProto(blockNo uint64) *types.BookmarkProto {
	return &types.BookmarkProto{
		BookMark: &datastream.BookMark{
			Type:  datastream.BookmarkType_BOOKMARK_TYPE_L2_BLOCK,
			Value: blockNo,
		},
	}
}

func (srv *DataStreamServer) CreateL2BlockProto(
	block *eritypes.Block,
	batchNumber uint64,
	ger libcommon.Hash,
	deltaTimestamp uint32,
	l1InfoIndex uint32,
	l1BlockHash libcommon.Hash,
	minTimestamp uint64,
) *types.L2BlockProto {
	return &types.L2BlockProto{
		L2Block: &datastream.L2Block{
			Number:          block.NumberU64(),
			BatchNumber:     batchNumber,
			Timestamp:       block.Time(),
			DeltaTimestamp:  deltaTimestamp,
			MinTimestamp:    minTimestamp,
			L1Blockhash:     l1BlockHash.Bytes(),
			L1InfotreeIndex: l1InfoIndex,
			Hash:            block.Hash().Bytes(),
			StateRoot:       block.Root().Bytes(),
			GlobalExitRoot:  ger.Bytes(),
			Coinbase:        block.Coinbase().Bytes(),
		},
	}
}

func (srv *DataStreamServer) CreateTransactionProto(
	effectiveGasPricePercentage uint8,
	stateRoot libcommon.Hash,
	tx eritypes.Transaction,
) (*types.TxProto, error) {
	buf := make([]byte, 0)
	writer := bytes.NewBuffer(buf)
	err := tx.EncodeRLP(writer)
	if err != nil {
		return nil, err
	}

	encoded := writer.Bytes()

	return &types.TxProto{
		Transaction: &datastream.Transaction{
			EffectiveGasPricePercentage: uint32(effectiveGasPricePercentage),
			IsValid:                     true, // TODO: SEQ: we don't store this value anywhere currently as a sync node
			ImStateRoot:                 stateRoot.Bytes(),
			Encoded:                     encoded,
		},
	}, nil
}

func (srv *DataStreamServer) CreateBatchStartProto(batchNo, chainId, forkId uint64) (*types.BatchStartProto, error) {
	return &types.BatchStartProto{
		BatchStart: &datastream.BatchStart{
			Number:  batchNo,
			ForkId:  forkId,
			ChainId: chainId,
		},
	}, nil
}

func (srv *DataStreamServer) CreateBatchEndProto(localExitRoot, stateRoot libcommon.Hash) (*types.BatchEndProto, error) {
	return &types.BatchEndProto{
		BatchEnd: &datastream.BatchEnd{
			LocalExitRoot: localExitRoot.Bytes(),
			StateRoot:     stateRoot.Bytes(),
		},
	}, nil
}

func (srv *DataStreamServer) CreateStreamEntriesProto(
	block *eritypes.Block,
	reader *hermez_db.HermezDbReader,
	lastBlock *eritypes.Block,
	batchNumber uint64,
	lastBatchNumber uint64,
	l1InfoTreeMinTimestamps map[uint64]uint64,
	batchEnd bool,
) (*[]DataStreamEntryProto, error) {
	blockNum := block.NumberU64()

	entryCount := 2                         // l2 block bookmark + l2 block
	entryCount += len(block.Transactions()) // transactions

	if lastBatchNumber != batchNumber {
		entryCount++ // batch bookmark
	}

	if batchEnd {
		entryCount++ // batch
	}

	entries := make([]DataStreamEntryProto, entryCount)
	index := 0

	// BATCH BOOKMARK
	if batchNumber != lastBatchNumber {
		batchBookmark := srv.CreateBatchBookmarkEntryProto(batchNumber)
		entries[index] = batchBookmark
		index++
	}

	deltaTimestamp := block.Time() - lastBlock.Time()

	// BATCH START
	fork, err := reader.GetForkId(batchNumber)
	if err != nil {
		return nil, err
	}

	batch, err := srv.CreateBatchStartProto(batchNumber, srv.chainId, fork)
	if err != nil {
		return nil, err
	}
	entries[index] = batch

	// L2 BLOCK BOOKMARK
	l2blockBookmark := srv.CreateL2BlockBookmarkEntryProto(blockNum)
	entries[index] = l2blockBookmark
	index++

	ger, err := reader.GetBlockGlobalExitRoot(blockNum)
	if err != nil {
		return nil, err
	}
	l1BlockHash, err := reader.GetBlockL1BlockHash(blockNum)
	if err != nil {
		return nil, err
	}

	l1InfoIndex, err := reader.GetBlockL1InfoTreeIndex(blockNum)
	if err != nil {
		return nil, err
	}

	if l1InfoIndex > 0 {
		// get the l1 info data, so we can add the min timestamp to the map
		l1Info, err := reader.GetL1InfoTreeUpdate(l1InfoIndex)
		if err != nil {
			return nil, err
		}
		if l1Info != nil {
			l1InfoTreeMinTimestamps[l1InfoIndex] = l1Info.Timestamp
		}
	}

	// L2 BLOCK
	l2Block := srv.CreateL2BlockProto(block, batchNumber, ger, uint32(deltaTimestamp), uint32(l1InfoIndex), l1BlockHash, l1InfoTreeMinTimestamps[l1InfoIndex])
	entries[index] = l2Block
	index++

	for _, tx := range block.Transactions() {
		effectiveGasPricePercentage, err := reader.GetEffectiveGasPricePercentage(tx.Hash())
		if err != nil {
			return nil, err
		}
		intermediateRoot, err := reader.GetIntermediateTxStateRoot(block.NumberU64(), tx.Hash())
		if err != nil {
			return nil, err
		}

		// TRANSACTION
		transaction, err := srv.CreateTransactionProto(effectiveGasPricePercentage, intermediateRoot, tx)
		entries[index] = transaction
		index++
	}

	// BATCH
	if batchEnd {
		// TODO: batch end create: block.Root(), block.Root()
	}

	return &entries, nil
}

func (srv *DataStreamServer) CreateAndBuildStreamEntryBytesProto(
	block *eritypes.Block,
	reader *hermez_db.HermezDbReader,
	lastBlock *eritypes.Block,
	batchNumber uint64,
	lastBatchNumber uint64,
	l1InfoTreeMinTimestamps map[uint64]uint64,
	batchEnd bool,
) ([]byte, error) {
	entries, err := srv.CreateStreamEntriesProto(block, reader, lastBlock, batchNumber, lastBatchNumber, l1InfoTreeMinTimestamps, batchEnd)
	if err != nil {
		return nil, err
	}

	var result []byte
	for _, entry := range *entries {
		b, err := encodeEntryToBytesProto(entry)
		if err != nil {
			return nil, err
		}
		result = append(result, b...)
	}

	return result, nil
}

const (
	PACKET_TYPE_DATA = 2
	// NOOP_ENTRY_NUMBER is used because we don't care about the entry number when feeding an atrificial
	// stream to the executor, if this ever changes then we'll need to populate an actual number
	NOOP_ENTRY_NUMBER = 0
)

func encodeEntryToBytesProto(entry DataStreamEntryProto) ([]byte, error) {
	data, err := entry.Marshal()
	if err != nil {
		return nil, err
	}
	var totalLength = 1 + 4 + 4 + 8 + uint32(len(data))
	buf := make([]byte, 1)
	buf[0] = PACKET_TYPE_DATA
	buf = binary.BigEndian.AppendUint32(buf, totalLength)
	buf = binary.BigEndian.AppendUint32(buf, uint32(entry.Type()))
	buf = binary.BigEndian.AppendUint64(buf, uint64(NOOP_ENTRY_NUMBER))
	buf = append(buf, data...)
	return buf, nil
}
