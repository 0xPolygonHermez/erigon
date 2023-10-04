package datastream

import (
	"github.com/ledgerwatch/erigon/zk/datastream/client"
	"github.com/ledgerwatch/erigon/zk/datastream/types"
	"github.com/pkg/errors"
)

const TestDatastreamUrl = "stream.internal.zkevm-test.net:6900"

// This code downloads headers and blocks from a datastream server.
func DownloadHeaders(datastreamUrl string, blockChannel chan types.FullL2Block) (uint64, error) {
	// Create client
	c := client.NewClient(datastreamUrl)

	// Start client (connect to the server)
	defer c.Stop()
	if err := c.Start(); err != nil {
		return 0, errors.Wrap(err, "failed to start client")
	}

	// Get header from server
	if err := c.GetHeader(); err != nil {
		return 0, errors.Wrap(err, "failed to get header")
	}

	// Read all entries from server
	entriesRead, err := c.ReadAllEntriesToChannel(blockChannel)
	if err != nil {
		return 0, errors.Wrap(err, "failed to read all entries to channel")
	}

	return entriesRead, nil
}
