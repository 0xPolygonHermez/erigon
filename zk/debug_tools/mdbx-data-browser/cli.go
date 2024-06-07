package mdbxdatabrowser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/gateway-fm/cdk-erigon-lib/kv"
	"github.com/gateway-fm/cdk-erigon-lib/kv/mdbx"
	"github.com/urfave/cli/v2"

	"github.com/ledgerwatch/erigon/cmd/utils"
	types "github.com/ledgerwatch/erigon/zk/rpcdaemon"
)

var (
	// common flags
	verboseFlag = &cli.BoolFlag{
		Name: "verbose",
		Usage: "If verbose output is enabled, it prints all the details about blocks and transactions in the batches, " +
			"otherwise just its hashes",
		Destination: &verboseOutput,
	}

	fileOutputFlag = &cli.BoolFlag{
		Name:        "file-output",
		Usage:       "If file output is enabled, all the results are persisted within a file",
		Destination: &fileOutput,
	}

	// commands
	getBatchByNumberCmd = &cli.Command{
		Action: dumpBatchesByNumbers,
		Name:   "output-batches",
		Usage:  "Outputs batches by numbers",
		Flags: []cli.Flag{
			&utils.DataDirFlag,
			&cli.Uint64SliceFlag{
				Name:        "bn",
				Usage:       "Batch numbers",
				Destination: batchOrBlockNumbers,
			},
			verboseFlag,
			fileOutputFlag,
		},
	}

	getBlockByNumberCmd = &cli.Command{
		Action: dumpBlocksByNumbers,
		Name:   "output-blocks",
		Usage:  "Outputs blocks by numbers",
		Flags: []cli.Flag{
			&utils.DataDirFlag,
			&cli.Uint64SliceFlag{
				Name:        "bn",
				Usage:       "Block numbers",
				Destination: batchOrBlockNumbers,
			},
			verboseFlag,
			fileOutputFlag,
		},
	}

	// parameters
	chainDataDir        string
	batchOrBlockNumbers *cli.Uint64Slice
	verboseOutput       bool
	fileOutput          bool
)

// dumpBatchesByNumbers retrieves batches by given numbers and dumps them either on standard output or to a file
func dumpBatchesByNumbers(cliCtx *cli.Context) error {
	if !cliCtx.IsSet(utils.DataDirFlag.Name) {
		return errors.New("chain data directory is not provided")
	}

	chainDataDir = cliCtx.String(utils.DataDirFlag.Name)

	tx, err := createDbTx(chainDataDir, cliCtx.Context)
	if err != nil {
		return fmt.Errorf("failed to create read-only db transaction: %w", err)
	}
	defer tx.Rollback()

	r := newDbDataRetriever(tx)
	batches := make([]*types.Batch, 0, len(batchOrBlockNumbers.Value()))
	for _, batchNum := range batchOrBlockNumbers.Value() {
		batch, err := r.GetBatchByNumber(batchNum, verboseOutput)
		if err != nil {
			return fmt.Errorf("failed to retrieve the batch %d: %w", batchOrBlockNumbers, err)
		}

		batches = append(batches, batch)
	}

	jsonBatches, err := json.MarshalIndent(batches, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize batches into the JSON format: %w", err)
	}

	if err := printResults(string(jsonBatches)); err != nil {
		return fmt.Errorf("failed to print results: %w", err)
	}

	return nil
}

// dumpBlocksByNumbers retrieves batches by given numbers and dumps them either on standard output or to a file
func dumpBlocksByNumbers(cliCtx *cli.Context) error {
	if !cliCtx.IsSet(utils.DataDirFlag.Name) {
		return errors.New("chain data directory is not provided")
	}

	chainDataDir = cliCtx.String(utils.DataDirFlag.Name)

	tx, err := createDbTx(chainDataDir, cliCtx.Context)
	if err != nil {
		return fmt.Errorf("failed to create read-only db transaction: %w", err)
	}
	defer tx.Rollback()

	r := newDbDataRetriever(tx)
	blocks := make([]*types.Block, 0, len(batchOrBlockNumbers.Value()))
	for _, batchNum := range batchOrBlockNumbers.Value() {
		block, err := r.GetBlockByNumber(batchNum, verboseOutput, verboseOutput)
		if err != nil {
			return fmt.Errorf("failed to retrieve the block %d: %w", batchOrBlockNumbers, err)
		}

		blocks = append(blocks, block)
	}

	jsonBlocks, err := json.MarshalIndent(blocks, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize blocks into the JSON format: %w", err)
	}

	if err := printResults(string(jsonBlocks)); err != nil {
		return fmt.Errorf("failed to print results: %w", err)
	}

	return nil
}

// createDbTx creates a read-only database transaction, that allows querying it.
func createDbTx(chainDataDir string, ctx context.Context) (kv.Tx, error) {
	db := mdbx.MustOpenRo(chainDataDir)
	defer db.Close()

	return db.BeginRo(ctx)
}

// printResults prints results either to the terminal or to the file
func printResults(results string) error {
	// output results to the file
	if fileOutput {
		formattedTime := time.Now().Format("02-01-2006 15:04:05")
		fileName := fmt.Sprintf("output_%s.json", formattedTime)

		file, err := os.Create(fileName)
		if err != nil {
			return fmt.Errorf("error creating file: %w", err)
		}
		defer file.Close()

		_, err = file.Write([]byte(results))
		return err
	}

	// output results to the standard output
	fmt.Println(results)

	return nil
}
