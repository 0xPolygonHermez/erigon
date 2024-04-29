package server

import (
	"bytes"
	"encoding/binary"
	"fmt"

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

// replace by EntryType_name mapping in proto
var entryTypeMappings = map[types.EntryType]datastreamer.EntryType{
	types.EntryTypeStartL2Block: datastreamer.EntryType(1),
	types.EntryTypeL2Tx:         datastreamer.EntryType(2),
	types.EntryTypeEndL2Block:   datastreamer.EntryType(3),
	types.EntryTypeGerUpdate:    datastreamer.EntryType(4),
	types.EntryTypeBookmark:     datastreamer.EntryType(176),
}

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
	Type() (datastream.EntryType, datastream.BookmarkType)
}

func NewDataStreamServer(stream *datastreamer.StreamServer, chainId uint64, mode OperationMode) *DataStreamServer {
	return &DataStreamServer{
		stream:  stream,
		chainId: chainId,
		mode:    mode,
	}
}

// Deprecated: use CommitEntriesToStreamProto instead
func (srv *DataStreamServer) CommitEntriesToStream(entries []DataStreamEntry, bigEndian bool) error {
	for _, entry := range entries {
		entryType := entry.EntryType()
		bytes := entry.Bytes(bigEndian)
		if entryType == types.EntryTypeBookmark {
			_, err := srv.stream.AddStreamBookmark(bytes)
			if err != nil {
				return err
			}
		} else {
			mapped, ok := entryTypeMappings[entryType]
			if !ok {
				return fmt.Errorf("unsupported stream entry type: %v", entryType)
			}
			_, err := srv.stream.AddStreamEntry(mapped, bytes)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (srv *DataStreamServer) CommitEntriesToStreamProto(entries []DataStreamEntryProto) error {
	for _, entry := range entries {
		em, err := entry.Marshal()
		if err != nil {
			return err
		}
		entryType, bookmarkType := entry.Type()
		if entryType != datastream.EntryType_ENTRY_TYPE_UNSPECIFIED {
			_, err = srv.stream.AddStreamEntry(datastreamer.EntryType(entryType), em)
		} else if bookmarkType != datastream.BookmarkType_BOOKMARK_TYPE_UNSPECIFIED {
			_, err = srv.stream.AddStreamBookmark(em)
		} else {
			return fmt.Errorf("unsupported stream entry type")
		}
	}
	return nil
}

// Deprecated: use CreateBatchBookmarkEntryProto/CreateL2BlockBookmarkEntryProto instead
func (srv *DataStreamServer) CreateBookmarkEntry(t BookmarkType, marker uint64) *types.Bookmark {
	return &types.Bookmark{Type: byte(t), From: marker}
}

func (srv *DataStreamServer) CreateBatchBookmarkEntryProto(batchNo uint64) *types.BookmarkBatchProto {
	return &types.BookmarkBatchProto{
		BookMark: &datastream.BookMark{
			Type:  datastream.BookmarkType_BOOKMARK_TYPE_BATCH,
			Value: batchNo,
		},
	}
}

func (srv *DataStreamServer) CreateL2BlockBookmarkEntryProto(blockNo uint64) *types.BookmarkL2BlockProto {
	return &types.BookmarkL2BlockProto{
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

// Deprecated: no longer required
func (srv *DataStreamServer) CreateBlockStartEntry(block *eritypes.Block, batchNumber uint64, forkId uint16, ger libcommon.Hash, deltaTimestamp uint32, l1InfoIndex uint32, l1BlockHash libcommon.Hash) *types.StartL2Block {
	return &types.StartL2Block{
		BatchNumber:     batchNumber,
		L2BlockNumber:   block.NumberU64(),
		Timestamp:       int64(block.Time()),
		DeltaTimestamp:  deltaTimestamp,
		L1InfoTreeIndex: l1InfoIndex,
		L1BlockHash:     l1BlockHash,
		GlobalExitRoot:  ger,
		Coinbase:        block.Coinbase(),
		ForkId:          forkId,
		ChainId:         uint32(srv.chainId),
	}
}

// Deprecated: no longer required
func (srv *DataStreamServer) CreateBlockEndEntry(blockNumber uint64, blockHash, stateRoot libcommon.Hash) *types.EndL2Block {
	return &types.EndL2Block{
		L2BlockNumber: blockNumber,
		L2Blockhash:   blockHash,
		StateRoot:     stateRoot,
	}
}

// Deprecated: use CreateTransactionProto instead
func (srv *DataStreamServer) CreateTransactionEntry(
	effectiveGasPricePercentage uint8,
	stateRoot libcommon.Hash,
	fork uint16,
	tx eritypes.Transaction,
) (*types.L2Transaction, error) {
	buf := make([]byte, 0)
	writer := bytes.NewBuffer(buf)
	err := tx.EncodeRLP(writer)
	if err != nil {
		return nil, err
	}

	encoded := writer.Bytes()

	length := len(encoded)

	return &types.L2Transaction{
		EffectiveGasPricePercentage: effectiveGasPricePercentage,
		IsValid:                     1, // TODO: SEQ: we don't store this value anywhere currently as a sync node
		StateRoot:                   stateRoot,
		EncodedLength:               uint32(length),
		Encoded:                     encoded,
	}, nil
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

func (srv *DataStreamServer) CreateBatchProto(batchNo, chainId, forkId uint64, localExitRoot, stateRoot libcommon.Hash) (*types.BatchProto, error) {
	return &types.BatchProto{
		Batch: &datastream.Batch{
			Number:        batchNo,
			LocalExitRoot: localExitRoot.Bytes(),
			StateRoot:     stateRoot.Bytes(),
			ForkId:        forkId,
			ChainId:       chainId,
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
		fork, err := reader.GetForkId(batchNumber)
		if err != nil {
			return nil, err
		}

		batch, err := srv.CreateBatchProto(batchNumber, srv.chainId, fork, block.Root(), block.Root())
		if err != nil {
			return nil, err
		}
		entries[index] = batch
	}

	return &entries, nil
}

// Deprecated: use CreateStreamEntriesProto instead
func (srv *DataStreamServer) CreateStreamEntries(
	block *eritypes.Block,
	reader *hermez_db.HermezDbReader,
	lastBlock *eritypes.Block,
	batchNumber uint64,
	lastBatchNumber uint64,
	gerUpdates *[]types.GerUpdate,
	l1InfoTreeMinTimestamps map[uint64]uint64,
) (*[]DataStreamEntry, error) {
	blockNum := block.NumberU64()

	fork, err := reader.GetForkId(batchNumber)
	if err != nil {
		return nil, err
	}

	// block start + block end + bookmark
	entryCount := 3
	if gerUpdates != nil {
		entryCount += len(*gerUpdates)
	}

	entryCount += len(block.Transactions())

	if lastBatchNumber != batchNumber {
		// for the batch bookmark
		entryCount++
	}

	entries := make([]DataStreamEntry, entryCount)
	index := 0

	//gerUpdates are before the bookmark for this block and are gottne by previous ones bookmark
	if gerUpdates != nil {
		for i := range *gerUpdates {
			entries[index] = &(*gerUpdates)[i]
			index++
		}
	}

	if batchNumber != lastBatchNumber {
		batchStart := srv.CreateBookmarkEntry(BatchBookmarkType, batchNumber)
		entries[index] = batchStart
		index++
	}

	bookmark := srv.CreateBookmarkEntry(BlockBookmarkType, block.NumberU64())
	entries[index] = bookmark
	index++

	deltaTimestamp := block.Time() - lastBlock.Time()

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

	blockStart := srv.CreateBlockStartEntry(block, batchNumber, uint16(fork), ger, uint32(deltaTimestamp), uint32(l1InfoIndex), l1BlockHash)
	entries[index] = blockStart
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
		transaction, err := srv.CreateTransactionEntry(effectiveGasPricePercentage, intermediateRoot, uint16(fork), tx)
		if err != nil {
			return nil, err
		}
		entries[index] = transaction
		index++
	}

	blockEnd := srv.CreateBlockEndEntry(block.NumberU64(), block.Root(), block.Root())
	entries[index] = blockEnd

	return &entries, nil
}

// Deprecated: use CreateAndBuildStreamEntryBytesProto instead
func (srv *DataStreamServer) CreateAndBuildStreamEntryBytes(
	block *eritypes.Block,
	reader *hermez_db.HermezDbReader,
	lastBlock *eritypes.Block,
	batchNumber uint64,
	lastBatchNumber uint64,
	bigEndian bool,
	gerUpdates *[]types.GerUpdate,
	l1InfoTreeMinTimestamps map[uint64]uint64,
) ([]byte, error) {
	entries, err := srv.CreateStreamEntries(block, reader, lastBlock, batchNumber, lastBatchNumber, gerUpdates, l1InfoTreeMinTimestamps)
	if err != nil {
		return nil, err
	}

	var result []byte
	for _, entry := range *entries {
		b := encodeEntryToBytes(entry, bigEndian)
		result = append(result, b...)
	}

	return result, nil
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

func encodeEntryToBytes(entry DataStreamEntry, bigEndian bool) []byte {
	data := entry.Bytes(bigEndian)
	var totalLength = 1 + 4 + 4 + 8 + uint32(len(data))
	buf := make([]byte, 1)
	buf[0] = PACKET_TYPE_DATA
	if bigEndian {
		buf = binary.BigEndian.AppendUint32(buf, totalLength)
		buf = binary.BigEndian.AppendUint32(buf, uint32(entry.EntryType()))
		buf = binary.BigEndian.AppendUint64(buf, uint64(NOOP_ENTRY_NUMBER))
	} else {
		buf = binary.LittleEndian.AppendUint32(buf, totalLength)
		buf = binary.LittleEndian.AppendUint32(buf, uint32(entry.EntryType()))
		buf = binary.LittleEndian.AppendUint64(buf, uint64(NOOP_ENTRY_NUMBER))
	}
	buf = append(buf, data...)
	return buf
}

func encodeEntryToBytesProto(entry DataStreamEntryProto) ([]byte, error) {
	em, err := entry.Marshal()
	if err != nil {
		return nil, err
	}
	entryType, bookmark := entry.Type()
	var totalLength = 1 + 4 + 4 + 8 + uint32(len(em))
	buf := make([]byte, 1)
	buf[0] = PACKET_TYPE_DATA
	buf = binary.BigEndian.AppendUint32(buf, totalLength)
	if bookmark != datastream.BookmarkType_BOOKMARK_TYPE_UNSPECIFIED {
		buf = binary.BigEndian.AppendUint32(buf, uint32(bookmark))
	} else {
		buf = binary.BigEndian.AppendUint32(buf, uint32(entryType))
	}
	buf = binary.BigEndian.AppendUint64(buf, uint64(NOOP_ENTRY_NUMBER))
	buf = append(buf, em...)
	return buf, nil
}
