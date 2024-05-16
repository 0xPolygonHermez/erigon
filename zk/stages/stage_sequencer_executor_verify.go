package stages

import (
	"context"
	"sort"

	"bytes"
	"fmt"

	"github.com/gateway-fm/cdk-erigon-lib/common"
	"github.com/gateway-fm/cdk-erigon-lib/kv"
	"github.com/ledgerwatch/erigon/core/rawdb"
	"github.com/ledgerwatch/erigon/core/types"
	"github.com/ledgerwatch/erigon/eth/stagedsync"
	"github.com/ledgerwatch/erigon/eth/stagedsync/stages"
	"github.com/ledgerwatch/erigon/zk/hermez_db"
	"github.com/ledgerwatch/erigon/zk/legacy_executor_verifier"
	"github.com/ledgerwatch/erigon/zk/txpool"
	"github.com/ledgerwatch/log/v3"
)

type SequencerExecutorVerifyCfg struct {
	db       kv.RwDB
	verifier *legacy_executor_verifier.LegacyExecutorVerifier
	txPool   *txpool.TxPool
}

func StageSequencerExecutorVerifyCfg(
	db kv.RwDB,
	verifier *legacy_executor_verifier.LegacyExecutorVerifier,
	pool *txpool.TxPool,
) SequencerExecutorVerifyCfg {
	return SequencerExecutorVerifyCfg{
		db:       db,
		verifier: verifier,
		txPool:   pool,
	}
}

func SpawnSequencerExecutorVerifyStage(
	s *stagedsync.StageState,
	u stagedsync.Unwinder,
	tx kv.RwTx,
	ctx context.Context,
	cfg SequencerExecutorVerifyCfg,
	initialCycle bool,
	quiet bool,
) error {
	var err error
	freshTx := tx == nil
	if freshTx {
		tx, err = cfg.db.BeginRw(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback()
	}

	hermezDb := hermez_db.NewHermezDb(tx)

	// progress here is at the batch level
	progress, err := stages.GetStageProgress(tx, stages.SequenceExecutorVerify)
	if err != nil {
		return err
	}

	// progress here is at the block level
	executeProgress, err := stages.GetStageProgress(tx, stages.Execution)
	if err != nil {
		return err
	}

	// we need to get the batch number for the latest block, so we can search for new batches to send for
	// verification
	latestBatch, err := hermezDb.GetBatchNoByL2Block(executeProgress)
	if err != nil {
		return err
	}

	// we could be running in a state with no executors so we need instant response that we are in an
	// ok state to save lag in the data stream !!Dragons: there will be no witnesses stored running in
	// this mode of operation
	canVerify := cfg.verifier.HasExecutors()
	if !canVerify {
		if err = stages.SaveStageProgress(tx, stages.SequenceExecutorVerify, latestBatch); err != nil {
			return err
		}
		if freshTx {
			if err = tx.Commit(); err != nil {
				return err
			}
		}
		return nil
	}

	// get ordered promises from the verifier
	// NB: this call is where the stream write happens (so it will be delayed until this stage is run)
	responses, err := cfg.verifier.ConsumeResultsUnsafe(tx)
	if err != nil {
		return err
	}

	for _, response := range responses {
		if response == nil {
			// something went wrong in the verification process (but not a failed verification)
			return fmt.Errorf("verifier failed (but not due to verification)")
		}

		// ensure that the first response is the next batch based on the current stage progress
		// otherwise just return early until we get it
		if response.BatchNumber != progress+1 {
			return nil
		}

		// now check that we are indeed in a good state to continue
		if !response.Valid {
			log.Info(fmt.Sprintf("[%s] identified an invalid batch, entering limbo", s.LogPrefix()), "batch", response.BatchNumber)
			// we have an invalid batch, so we need to notify the txpool that these transactions are spurious
			// and need to go into limbo and then trigger a rewind.  The rewind will put all TX back into the
			// pool, but as it knows about these limbo transactions it will place them into limbo instead
			// of queueing them again
			limboDetails := txpool.LimboBatchDetails{
				Witness:               response.Witness,
				BatchNumber:           response.BatchNumber,
				ExecutorResponse:      response.ExecutorResponse,
				BadTransactionsHashes: make([]common.Hash, 0),
				BadTransactionsRLP:    make([][]byte, 0),
			}

			// now we need to figure out the highest block number in the batch
			// and grab all the transaction hashes along the way to inform the
			// pool of hashes to avoid
			blockNumbers, err := hermezDb.GetL2BlockNosByBatch(response.BatchNumber)
			if err != nil {
				return err
			}

			// sort the block numbers into ascending order
			sort.Slice(blockNumbers, func(i, j int) bool {
				return blockNumbers[i] < blockNumbers[j]
			})

			var lowestBlock *types.Block

			for _, blockNumber := range blockNumbers {
				block, err := rawdb.ReadBlockByNumber(tx, blockNumber)
				if err != nil {
					return err
				}
				if lowestBlock == nil {
					// capture the first block, then we can set the bad block hash in the unwind to terminate the
					// stage loop and broadcast the accumulator changes to the txpool before the next stage loop
					// run
					lowestBlock = block
				}
				for _, transaction := range block.Transactions() {
					hash := transaction.Hash()
					var b []byte
					buffer := bytes.NewBuffer(b)
					err = transaction.EncodeRLP(buffer)
					if err != nil {
						return err
					}
					limboDetails.BadTransactionsHashes = append(limboDetails.BadTransactionsHashes, hash)
					limboDetails.BadTransactionsRLP = append(limboDetails.BadTransactionsRLP, buffer.Bytes())
					log.Info(fmt.Sprintf("[%s] adding transaction to limbo", s.LogPrefix()), "hash", hash)
				}
			}

			cfg.txPool.NewLimboBatchDetails(limboDetails)

			if lowestBlock != nil {
				u.UnwindTo(lowestBlock.NumberU64()-1, lowestBlock.Hash())
				cfg.verifier.CancelAllRequestsUnsafe()
			}
			return nil
		}

		// all good so just update the stage progress for now
		if err = stages.SaveStageProgress(tx, stages.SequenceExecutorVerify, response.BatchNumber); err != nil {
			return err
		}

		// we know that if the batch has been marked as OK we can update the datastream progress to match
		// as the verifier will have handled writing to the stream
		highestBlock, err := hermezDb.GetHighestBlockInBatch(response.BatchNumber)
		if err != nil {
			return err
		}

		if err = stages.SaveStageProgress(tx, stages.DataStream, highestBlock); err != nil {
			return err
		}

		// store the witness
		errWitness := hermezDb.WriteWitness(response.BatchNumber, response.Witness)
		if errWitness != nil {
			log.Warn("Failed to write witness", "batch", response.BatchNumber, "err", errWitness)
		}

		progress = response.BatchNumber
	}

	// send off the new batches to the verifier to be processed
	for batch := progress + 1; batch <= latestBatch; batch++ {
		// we do not need to verify batch 1 as this is the injected batch so just updated progress and move on
		if batch == injectedBatchNumber {
			if err = stages.SaveStageProgress(tx, stages.SequenceExecutorVerify, injectedBatchNumber); err != nil {
				return err
			}
		} else {
			if cfg.verifier.IsRequestAddedUnsafe(batch) {
				continue
			}

			// we need the state root of the last block in the batch to send to the executor
			highestBlock, err := hermezDb.GetHighestBlockInBatch(batch)
			if err != nil {
				return err
			}
			if highestBlock == 0 {
				// maybe nothing in this batch and we know we don't handle batch 0 (genesis)
				continue
			}
			block, err := rawdb.ReadBlockByNumber(tx, highestBlock)
			if err != nil {
				return err
			}

			counters, err := hermezDb.GetBatchCounters(batch)
			if err != nil {
				return err
			}

			forkId, err := hermezDb.GetForkId(batch)
			if err != nil {
				return err
			}

			_, addErr := cfg.verifier.AddRequestUnsafe(ctx, tx, &legacy_executor_verifier.VerifierRequest{BatchNumber: batch, ForkId: forkId, StateRoot: block.Root(), Counters: counters})
			if addErr != nil {
				log.Error("Failed to add request to verifier", "batch", batch, "err", addErr)
			}
		}
	}

	if freshTx {
		if err = tx.Commit(); err != nil {
			return err
		}
	}

	return nil
}

func UnwindSequencerExecutorVerifyStage(
	u *stagedsync.UnwindState,
	s *stagedsync.StageState,
	tx kv.RwTx,
	ctx context.Context,
	cfg SequencerExecutorVerifyCfg,
	initialCycle bool,
) (err error) {
	/*
		The "Unwinder" keeps stage's progress in blocks.
		If a stage's current progress is <= unwindPoint then the unwind is not invoked for this stage (sync.go line 386)
		For this particular case, the progress is in batches => its progress is always <= unwindPoint, because unwindPoint is in blocks
		This is not a problem, because this stage's progress actually keeps the number of last verified batch and we never unwind the last verified batch
	*/

	// freshTx := tx == nil
	// if freshTx {
	// 	tx, err = cfg.db.BeginRw(ctx)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	defer tx.Rollback()
	// }

	// logPrefix := u.LogPrefix()
	// log.Info(fmt.Sprintf("[%s] Unwind Executor Verify", logPrefix), "from", s.BlockNumber, "to", u.UnwindPoint)

	// if err = u.Done(tx); err != nil {
	// 	return err
	// }

	// if freshTx {
	// 	if err = tx.Commit(); err != nil {
	// 		return err
	// 	}
	// }

	return nil
}

func PruneSequencerExecutorVerifyStage(
	s *stagedsync.PruneState,
	tx kv.RwTx,
	cfg SequencerExecutorVerifyCfg,
	ctx context.Context,
	initialCycle bool,
) error {
	return nil
}
