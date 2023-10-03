package main

import (
	"fmt"

	"github.com/ledgerwatch/erigon/zk/datastream/client"
)

func main() {
	c := client.NewClient("stream.internal.zkevm-test.net:6900")

	// Start client (connect to the server)
	defer c.Stop()
	if err := c.Start(); err != nil {
		panic(err)
	}

	if err := c.GetHeader(); err != nil {
		panic(err)
	}

	totalEntries := c.Header.TotalEntries
	fmt.Println("Total entries:", totalEntries)
	l2Blocks, _, err := c.ReadEntries(0, 10)
	if err != nil {
		panic(err)
	}

	fmt.Println((*l2Blocks)[0])
	fmt.Println((*l2Blocks)[1])
	fmt.Println((*l2Blocks)[2])
	fmt.Println("Downloaded Blocks count: ", len(*l2Blocks))
}
