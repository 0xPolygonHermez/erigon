package client

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"reflect"

	"github.com/ledgerwatch/erigon/zk/datastream/types"
)

type StreamType uint64
type Command uint64

type EntityDefinition struct {
	Name       string
	StreamType StreamType
	Definition reflect.Type
}

type StreamClient struct {
	server     string // Server address to connect IP:port
	streamType StreamType
	conn       net.Conn
	id         string // Client id

	Header types.HeaderEntry // Header info received (from Header command)

	entriesDefinition map[types.EntryType]EntityDefinition
}

const (
	// StreamTypeSequencer represents a Sequencer stream
	StSequencer StreamType = 1

	// Packet types
	PtPadding = 0
	PtHeader  = 1    // Just for the header page
	PtData    = 2    // Data entry
	PtResult  = 0xff // Not stored/present in file (just for client command result)

)

// Creates a new client fo datastream
// server must be in format "url:port"
func NewClient(server string) StreamClient {
	// Create the client data stream
	c := StreamClient{
		server:     server,
		streamType: StSequencer,
		id:         "",
		entriesDefinition: map[types.EntryType]EntityDefinition{
			types.EntryTypeStartL2Block: {
				Name:       "StartL2Block",
				StreamType: StSequencer,
				Definition: reflect.TypeOf(types.StartL2Block{}),
			},
			types.EntryTypeL2Tx: {
				Name:       "L2Transaction",
				StreamType: StSequencer,
				Definition: reflect.TypeOf(types.L2Transaction{}),
			},
			types.EntryTypeEndL2Block: {
				Name:       "EndL2Block",
				StreamType: StSequencer,
				Definition: reflect.TypeOf(types.EndL2Block{}),
			},
		},
	}

	return c
}

// Opens a TCP connection to the server
func (c *StreamClient) Start() error {
	// Connect to server
	var err error
	c.conn, err = net.Dial("tcp", c.server)
	if err != nil {
		return fmt.Errorf("error connecting to server %s: %v", c.server, err)
	}

	c.id = c.conn.LocalAddr().String()

	return nil
}

func (c *StreamClient) Stop() {
	c.conn.Close()
}

// Command header: Get status
// Returns the current status of the header.
// If started, terminate the connection.
func (c *StreamClient) GetHeader() error {
	if err := c.sendHeaderCmd(); err != nil {
		return fmt.Errorf("%s send header error: %v", c.id, err)
	}

	// Read packet
	packet, err := readBuffer(c.conn, 1)
	if err != nil {
		return fmt.Errorf("%s read buffer error: %v", c.id, err)
	}

	// Check packet type
	if packet[0] != PtResult {
		return fmt.Errorf("%s error expecting result packet type %d and received %d", c.id, PtResult, packet[0])
	}

	// Read server result entry for the command
	r, err := c.readResultEntry(packet)
	if err != nil {
		return fmt.Errorf("%s read result entry error: %v", c.id, err)
	}
	if err := r.GetError(); err != nil {
		return fmt.Errorf("%s got Result error code %d: %v", c.id, r.ErrorNum, err)
	}

	// Read header entry
	h, err := c.readHeaderEntry()
	if err != nil {
		return fmt.Errorf("%s read header entry error: %v", c.id, err)
	}

	c.Header = *h

	return nil
}

// sends start command, reads entries until limit reached and sends end command
func (c *StreamClient) ReadEntries(fromEntry uint64, l2BlocksAmount int) (*[]types.FullL2Block, uint64, error) {
	// send start command
	if err := c.initiateDownload(fromEntry); err != nil {
		return nil, 0, fmt.Errorf("%s initiate download error: %v", c.id, err)
	}

	fullL2Blocks, entriesRead, err := c.readFullL2Blocks(fromEntry, l2BlocksAmount)
	if err != nil {
		return nil, 0, fmt.Errorf("%s read full L2 blocks error: %v", c.id, err)
	}

	return fullL2Blocks, entriesRead, nil
}

// sends start command, reads entries until limit reached and sends end command
// sends the parsed FullL2Blocks with transactions to a channel
func (c *StreamClient) ReadAllEntriesToChannel(l2BlockChan chan types.FullL2Block, fromEntry uint64) (uint64, error) {
	// send start command
	if err := c.initiateDownload(fromEntry); err != nil {
		return 0, fmt.Errorf("%s initiate download error: %v", c.id, err)
	}

	entriesRead, err := c.readAllFullL2BlocksToChannel(0, l2BlockChan)
	if err != nil {
		return 0, fmt.Errorf("%s read full L2 blocks error: %v", c.id, err)
	}

	return entriesRead, nil
}

// runs the prerequisites for entries download
func (c *StreamClient) initiateDownload(fromEntry uint64) error {
	// send start command
	if err := c.sendStartCmd(fromEntry); err != nil {
		return fmt.Errorf("send start command error: %v", err)
	}

	// Read packet
	packet, err := readBuffer(c.conn, 1)
	if err != nil {
		return fmt.Errorf("read buffer error %v", err)
	}

	// Read server result entry for the command
	r, err := c.readResultEntry(packet)
	if err != nil {
		return fmt.Errorf("read result entry error: %v", err)
	}

	if err := r.GetError(); err != nil {
		return fmt.Errorf("got Result error code %d: %v", r.ErrorNum, err)
	}

	return nil
}

// reads all entries from the server and sends them to a channel
// sends the parsed FullL2Blocks with transactions to a channel
func (c *StreamClient) readAllFullL2BlocksToChannel(fromEntry uint64, l2BlockChan chan types.FullL2Block) (uint64, error) {
	entriesRead := uint64(0)
	blocksRead := 0
	for {
		if blocksRead > 1000 || entriesRead+fromEntry >= c.Header.TotalEntries {
			break
		}

		fullBlock, er, err := c.readFullBlock()
		if err != nil {
			return 0, fmt.Errorf("failed to read full block: %v", err)
		}

		blocksRead++
		l2BlockChan <- *fullBlock
		entriesRead += er
	}

	return entriesRead, nil
}

// reads a set amount of l2blocks from the server and returns them
// returns the parsed FullL2Blocks with transactions and the amount of entries read
func (c *StreamClient) readFullL2Blocks(fromEntry uint64, l2BlocksAmount int) (*[]types.FullL2Block, uint64, error) {
	fullL2Blocks := []types.FullL2Block{}
	entriesRead := uint64(0)
	for {
		if len(fullL2Blocks) >= l2BlocksAmount || entriesRead+fromEntry >= c.Header.TotalEntries {
			break
		}
		fullBlock, er, err := c.readFullBlock()
		if err != nil {
			return nil, 0, fmt.Errorf("failed to read full block: %v", err)
		}

		entriesRead += er
		fullL2Blocks = append(fullL2Blocks, *fullBlock)
	}

	return &fullL2Blocks, entriesRead, nil
}

// reads a full block from the server
// returns the parsed FullL2Block and the amount of entries read
func (c *StreamClient) readFullBlock() (*types.FullL2Block, uint64, error) {
	entriesRead := uint64(0)

	// read bookmark
	// TODO: maybe parse it and return it if needed
	bookmarkFile, err := c.readFileEntry()
	if err != nil {
		return nil, 0, fmt.Errorf("read file entry error: %v", err)
	}
	if !bookmarkFile.IsBookmark() {
		return nil, 0, fmt.Errorf("expected to find a bookmark but got: %d", bookmarkFile.EntryType)
	}
	entriesRead++

	// Wait next data entry streamed
	file, err := c.readFileEntry()
	if err != nil {
		return nil, 0, fmt.Errorf("read file entry error: %v", err)
	}
	entriesRead++
	// should start with a StartL2Block entry, followed by
	// txs entries and ending with a block endL2BlockEntry
	var startL2Block *types.StartL2Block
	l2Txs := []types.L2Transaction{}
	var endL2Block *types.EndL2Block

	if file.IsBlockStart() {
		startL2Block, err = types.DecodeStartL2Block(file.Data)
		if err != nil {
			return nil, 0, fmt.Errorf("read start of block error: %v", err)
		}

		for {
			file, err := c.readFileEntry()
			if err != nil {
				return nil, 0, fmt.Errorf("read file entry error: %v", err)
			}

			entriesRead++

			if file.IsTx() {
				l2Tx, err := types.DecodeL2Transaction(file.Data)
				if err != nil {
					return nil, 0, fmt.Errorf("parse l2Transaction error: %v", err)
				}
				l2Txs = append(l2Txs, *l2Tx)
			} else if file.IsBlockEnd() {
				endL2Block, err = types.DecodeEndL2Block(file.Data)
				if err != nil {
					return nil, 0, fmt.Errorf("parse endL2Block error: %v", err)
				}
				if startL2Block.L2BlockNumber != endL2Block.L2BlockNumber {
					return nil, 0, fmt.Errorf("start block block number different than endBlock block number. StartBlock: %d, EndBlock: %d", startL2Block.L2BlockNumber, endL2Block.L2BlockNumber)
				}
				break
			} else {
				return nil, 0, fmt.Errorf("expected EndL2Block or L2Transaction type, got type: %d", file.EntryType)
			}
		}
	} else {
		return nil, 0, fmt.Errorf("expected StartL2Block, but got type: %d", file.EntryType)
	}

	fullL2Block := types.ParseFullL2Block(startL2Block, endL2Block, &l2Txs)

	return fullL2Block, entriesRead, nil
}

// reads file bytes from socket and tries to parse them
// returns the parsed FileEntry
func (c *StreamClient) readFileEntry() (*types.FileEntry, error) {
	// Read packet type
	packet, err := readBuffer(c.conn, 1)
	if err != nil {
		return &types.FileEntry{}, fmt.Errorf("failed to read packet type: %v", err)
	}

	// Check packet type
	if packet[0] == PtResult {
		// Read server result entry for the command
		r, err := c.readResultEntry(packet)
		if err != nil {
			return &types.FileEntry{}, err
		}
		if err := r.GetError(); err != nil {
			return &types.FileEntry{}, fmt.Errorf("got Result error code %d: %v", r.ErrorNum, err)
		}
		return &types.FileEntry{}, nil
	} else if packet[0] != PtData {
		return &types.FileEntry{}, fmt.Errorf("error expecting data packet type %d and received %d", PtData, packet[0])
	}

	// Read the rest of fixed size fields
	buffer, err := readBuffer(c.conn, types.FileEntryMinSize-1)
	if err != nil {
		return &types.FileEntry{}, fmt.Errorf("error reading file bytes: %v", err)
	}
	buffer = append(packet, buffer...)

	// Read variable field (data)
	length := binary.BigEndian.Uint32(buffer[1:5])
	if length < types.FileEntryMinSize {
		return &types.FileEntry{}, errors.New("error reading data entry: wrong data length")
	}

	// Read rest of the file data
	bufferAux, err := readBuffer(c.conn, length-types.FileEntryMinSize)
	if err != nil {
		return &types.FileEntry{}, fmt.Errorf("error reading file data bytes: %v", err)
	}
	buffer = append(buffer, bufferAux...)

	// Decode binary data to data entry struct
	file, err := types.DecodeFileEntry(buffer)
	if err != nil {
		return &types.FileEntry{}, fmt.Errorf("decode file entry error: %v", err)
	}

	return file, nil
}

// reads header bytes from socket and tries to parse them
// returns the parsed HeaderEntry
func (c *StreamClient) readHeaderEntry() (*types.HeaderEntry, error) {
	// Read header stream bytes
	binaryHeader, err := readBuffer(c.conn, types.HeaderSize)
	if err != nil {
		return &types.HeaderEntry{}, fmt.Errorf("failed to read header bytes %v", err)
	}

	// Decode bytes stream to header entry struct
	h, err := types.DecodeHeaderEntry(binaryHeader)
	if err != nil {
		return &types.HeaderEntry{}, fmt.Errorf("error decoding binary header: %v", err)
	}

	return h, nil
}

// reads result bytes and tries to parse them
// returns the parsed ResultEntry
func (c *StreamClient) readResultEntry(packet []byte) (*types.ResultEntry, error) {
	if len(packet) != 1 {
		return &types.ResultEntry{}, fmt.Errorf("expected packet size of 1, got: %d", len(packet))
	}

	// Read the rest of fixed size fields
	buffer, err := readBuffer(c.conn, types.ResultEntryMinSize-1)
	if err != nil {
		return &types.ResultEntry{}, fmt.Errorf("failed to read main result bytes %v", err)
	}
	buffer = append(packet, buffer...)

	// Read variable field (errStr)
	length := binary.BigEndian.Uint32(buffer[1:5])
	if length < types.ResultEntryMinSize {
		return &types.ResultEntry{}, fmt.Errorf("%s Error reading result entry", c.id)
	}

	// read the rest of the result
	bufferAux, err := readBuffer(c.conn, length-types.ResultEntryMinSize)
	if err != nil {
		return &types.ResultEntry{}, fmt.Errorf("failed to read result errStr bytes %v", err)
	}
	buffer = append(buffer, bufferAux...)

	// Decode binary entry result
	re, err := types.DecodeResultEntry(buffer)
	if err != nil {
		return &types.ResultEntry{}, fmt.Errorf("decode result entry error: %v", err)
	}

	return re, nil
}
