package datastream

import (
	"encoding/binary"

	"github.com/ledgerwatch/erigon/zk/datastream/client"
	"github.com/ledgerwatch/erigon/zk/datastream/types"
	"github.com/pkg/errors"
)

// Download all available blocks from datastream server to channel
func DownloadAllL2BlocksToChannel(datastreamUrl string, blockChannel chan types.FullL2Block, gerUpdateChan chan types.GerUpdate, fromBlock uint64) (uint64, map[uint64][]byte, error) {
	// Create client
	c := client.NewClient(datastreamUrl)

	// Start client (connect to the server)
	if err := c.Start(); err != nil {
		return 0, nil, errors.Wrap(err, "failed to start client")
	}
	defer c.Stop()

	// Get header from server
	if err := c.GetHeader(); err != nil {
		return 0, nil, errors.Wrap(err, "failed to get header")
	}

	// Create bookmark
	bookmark := []byte{0}
	bookmark = binary.LittleEndian.AppendUint64(bookmark, fromBlock)

	// Read all entries from server
	entriesRead, bookmarks, err := c.ReadAllEntriesToChannel(blockChannel, gerUpdateChan, bookmark)
	if err != nil {
		return 0, nil, err
	}

	return entriesRead, bookmarks, nil
}

// Download a set amount of blocks from datastream server to channel
func DownloadL2Blocks(datastreamUrl string, fromBlock uint64, l2BlocksAmount int) (*[]types.FullL2Block, *[]types.GerUpdate, map[uint64][]byte, uint64, error) {
	// Create client
	c := client.NewClient(datastreamUrl)

	// Start client (connect to the server)
	defer c.Stop()
	if err := c.Start(); err != nil {
		return nil, nil, nil, 0, errors.Wrap(err, "failed to start client")
	}

	// Create bookmark
	bookmark := types.NewL2BlockBookmark(fromBlock)

	// Read all entries from server
	l2Blocks, gerUpdates, bookmarks, entriesRead, err := c.ReadEntries(bookmark, l2BlocksAmount)
	if err != nil {
		return nil, nil, nil, 0, err
	}

	return l2Blocks, gerUpdates, bookmarks, entriesRead, nil
}
