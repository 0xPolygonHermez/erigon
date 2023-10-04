package datastream

import (
	"github.com/ledgerwatch/erigon/zk/datastream/client"
	"github.com/ledgerwatch/erigon/zk/datastream/types"
)

const TestDatastreamUrl = "stream.internal.zkevm-test.net:6900"

func DownloadHeaders(datastreamUrl string, blockChannel chan types.FullL2Block) (uint64, error) {
	c := client.NewClient(datastreamUrl)

	// Start client (connect to the server)
	defer c.Stop()
	if err := c.Start(); err != nil {
		return 0, err
	}

	if err := c.GetHeader(); err != nil {
		return 0, err
	}

	entriesRead, err := c.ReadAllEntriesToChannel(blockChannel)
	if err != nil {
		return 0, err
	}

	return entriesRead, nil
}
