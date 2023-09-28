package client

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"reflect"

	"github.com/ledgerwatch/erigon/zkevm/datastream/types"
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
			types.EntryTypeL2Block: {
				Name:       "L2Block",
				StreamType: StSequencer,
				Definition: reflect.TypeOf(types.L2Block{}),
			},
			types.EntryTypeL2Tx: {
				Name:       "L2Transaction",
				StreamType: StSequencer,
				Definition: reflect.TypeOf(types.L2Transaction{}),
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
		return fmt.Errorf("%s %v", c.id, err)
	}

	// Read packet
	packet, err := readBuffer(c.conn, 1)
	if err != nil {
		return fmt.Errorf("%s %v", c.id, err)
	}

	// Check packet type
	if packet[0] != PtResult {
		return fmt.Errorf("%s Error expecting result packet type %d and received %d", c.id, PtResult, packet[0])
	}

	// Read server result entry for the command
	r, err := c.readResultEntry(packet)
	if err != nil {
		return fmt.Errorf("%s %v", c.id, err)
	}
	if err := r.GetError(); err != nil {
		return fmt.Errorf("%s got Result error code %d: %v", c.id, r.ErrorNum, err)
	}

	// Read header entry
	h, err := c.readHeaderEntry()
	if err != nil {
		return fmt.Errorf("%s %v", c.id, err)
	}

	c.Header = *h

	return nil
}

// sends start command, reads entries until limit reached and sends end command
func (c *StreamClient) ReadEntries(from, amount uint64) (*[]types.L2Block, *[]types.L2Transaction, error) {
	// send start command
	if err := c.sendStartCmd(from); err != nil {
		return &[]types.L2Block{}, &[]types.L2Transaction{}, fmt.Errorf("%s %v", c.id, err)
	}

	// Read packet
	packet, err := readBuffer(c.conn, 1)
	if err != nil {
		return &[]types.L2Block{}, &[]types.L2Transaction{}, fmt.Errorf("%s %v", c.id, err)
	}

	// Read server result entry for the command
	r, err := c.readResultEntry(packet)
	if err != nil {
		return &[]types.L2Block{}, &[]types.L2Transaction{}, fmt.Errorf("%s %v", c.id, err)
	}

	if err := r.GetError(); err != nil {
		return &[]types.L2Block{}, &[]types.L2Transaction{}, fmt.Errorf("got Result error code %d: %v", r.ErrorNum, err)
	}

	l2Blocks := []types.L2Block{}
	l2Txs := []types.L2Transaction{}
	count := uint64(0)
	for {
		// Wait next data entry streamed
		file, err := c.readFileEntry()
		if err != nil {
			return &[]types.L2Block{}, &[]types.L2Transaction{}, fmt.Errorf("%s %v", c.id, err)
		}

		if file.EntryType == types.EntryTypeL2Tx {
			l2Tx, err := types.DecodeL2Transaction(file.Data)
			if err != nil {
				return &[]types.L2Block{}, &[]types.L2Transaction{}, fmt.Errorf("%s %v", c.id, err)
			}
			l2Txs = append(l2Txs, *l2Tx)
		} else if file.EntryType == types.EntryTypeL2Block {
			l2Block, err := types.DecodeL2Block(file.Data)
			if err != nil {
				return &[]types.L2Block{}, &[]types.L2Transaction{}, fmt.Errorf("%s %v", c.id, err)
			}
			l2Blocks = append(l2Blocks, *l2Block)
		} else {
			if err != nil {
				return &[]types.L2Block{}, &[]types.L2Transaction{}, fmt.Errorf("%s unknown Entry type: %d", c.id, file.EntryType)
			}
		}

		count++
		if count == amount {
			break
		}
	}

	return &l2Blocks, &l2Txs, nil
}

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
		return &types.FileEntry{}, fmt.Errorf("%s Error expecting data packet type %d and received %d", c.id, PtData, packet[0])
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
		return &types.FileEntry{}, fmt.Errorf("%s %v", c.id, err)
	}

	return file, nil
}

// reads header bytes from socket and tries to parse them
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
		return &types.ResultEntry{}, err
	}

	return re, nil
}
