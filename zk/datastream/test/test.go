package main

import (
	"fmt"

	"github.com/ledgerwatch/erigon/zk/datastream"
	"github.com/ledgerwatch/erigon/zk/datastream/client"
)

// This code downloads headers and blocks from a datastream server.
func main() {
	// Create client
	c := client.NewClient(datastream.TestDatastreamUrl)

	// Start client (connect to the server)
	defer c.Stop()
	if err := c.Start(); err != nil {
		panic(err)
	}

	// Get header from server
	if err := c.GetHeader(); err != nil {
		panic(err)
	}

	// Read all entries from server
	blocksRead, entriesReadAmount, err := c.ReadEntries(0, 500000)
	if err != nil {
		panic(err)
	}

	fmt.Println("Entries read amount: ", entriesReadAmount)
	fmt.Println("Blocks read amount: ", len(*blocksRead))

	for i, block := range *blocksRead {
		if i + +1 != int(block.L2BlockNumber) {
			fmt.Println("Block #", i)
			fmt.Println("BatchNumber: ", block.L2BlockNumber)
		}
	}
	// fmt.Println("Blocks: ", (*blocksRead))
}
