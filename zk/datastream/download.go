package datastream

import (
	"github.com/ledgerwatch/erigon/zk/datastream/client"
	"github.com/ledgerwatch/erigon/zk/datastream/types"
	"github.com/pkg/errors"
)

const TestDatastreamUrl = "stream.internal.zkevm-test.net:6900"

// Download all available blocks from datastream server to channel
func DownloadAllL2BlocksToChannel(datastreamUrl string, blockChannel chan types.FullL2Block, bookmark []byte) (uint64, map[uint64][]byte, error) {
	// Create client
	c := client.NewClient(datastreamUrl)

	// Start client (connect to the server)
	defer c.Stop()
	if err := c.Start(); err != nil {
		return 0, nil, errors.Wrap(err, "failed to start client")
	}

	// Get header from server
	if err := c.GetHeader(); err != nil {
		return 0, nil, errors.Wrap(err, "failed to get header")
	}

	// Read all entries from server
	entriesRead, bookmarks, err := c.ReadAllEntriesToChannel(blockChannel, bookmark)
	if err != nil {
		return 0, nil, errors.Wrap(err, "failed to read all entries to channel")
	}

	return entriesRead, bookmarks, nil
}

// Download a set amount of blocks from datastream server to channel
func DownloadL2Blocks(datastreamUrl string, fromEntry uint64, l2BlocksAmount int) (*[]types.FullL2Block, map[uint64][]byte, uint64, error) {
	// Create client
	c := client.NewClient(datastreamUrl)

	// Start client (connect to the server)
	defer c.Stop()
	if err := c.Start(); err != nil {
		return nil, nil, 0, errors.Wrap(err, "failed to start client")
	}

	// Get header from server
	if err := c.GetHeader(); err != nil {
		return nil, nil, 0, errors.Wrap(err, "failed to get header")
	}

	// Read all entries from server
	l2Blocks, bookmarks, entriesRead, err := c.ReadEntries(fromEntry, l2BlocksAmount)
	if err != nil {
		return nil, nil, 0, errors.Wrap(err, "failed to read all entries to channel")
	}

	return l2Blocks, bookmarks, entriesRead, nil
}
