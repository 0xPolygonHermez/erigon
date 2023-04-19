package stagedsync

import (
	"context"

	"github.com/ledgerwatch/erigon-lib/kv"
)

// HeadersPOW progresses Headers stage for Proof-of-Work headers
func HeadersZK(
	s *StageState,
	u Unwinder,
	ctx context.Context,
	tx kv.RwTx,
	cfg HeadersCfg,
	initialCycle bool,
	test bool, // Set to true in tests, allows the stage to fail rather than wait indefinitely
	useExternalTx bool,
) error {

	/*
		zkSynchronizer, err := NewSynchronizer(true, nil, nil, nil, nil, nil, nil, nil)
		if err != nil {
			return err
		}

		return zkSynchronizer.Sync(cfg, tx)
	*/

	panic("boom")

	/*
				cfg.hd.SetPOSSync(false)
				cfg.hd.SetFetchingNew(true)
				defer cfg.hd.SetFetchingNew(false)

			headerProgress = cfg.hd.Progress()
			logPrefix := s.LogPrefix()
			// Check if this is called straight after the unwinds, which means we need to create new canonical markings
			hash, err := rawdb.ReadCanonicalHash(tx, headerProgress)
			if err != nil {
				return err
			}

			headerInserter := headerdownload.NewHeaderInserter(logPrefix, localTd, headerProgress, cfg.blockReader)
			cfg.hd.SetHeaderReader(&ChainReaderImpl{config: &cfg.chainConfig, tx: tx, blockReader: cfg.blockReader})

			stopped := false
			var noProgressCounter uint = 0
			prevProgress := headerProgress
			var wasProgress bool
			var lastSkeletonTime time.Time
			var peer [64]byte
			var sentToPeer bool
		Loop:
			for !stopped {

				transitionedToPoS, err := rawdb.Transitioned(tx, headerProgress, cfg.chainConfig.TerminalTotalDifficulty)
				if err != nil {
					return err
				}
				if transitionedToPoS {
					if err := s.Update(tx, headerProgress); err != nil {
						return err
					}
					break
				}

				sentToPeer = false
				currentTime := time.Now()
				req, penalties := cfg.hd.RequestMoreHeaders(currentTime)
				if req != nil {
					peer, sentToPeer = cfg.headerReqSend(ctx, req)
					if sentToPeer {
						cfg.hd.UpdateStats(req, false, peer)
						cfg.hd.UpdateRetryTime(req, currentTime, 5*time.Second)
					}
				}
				if len(penalties) > 0 {
					cfg.penalize(ctx, penalties)
				}
				maxRequests := 64 // Limit number of requests sent per round to let some headers to be inserted into the database
				for req != nil && sentToPeer && maxRequests > 0 {
					req, penalties = cfg.hd.RequestMoreHeaders(currentTime)
					if req != nil {
						peer, sentToPeer = cfg.headerReqSend(ctx, req)
						if sentToPeer {
							cfg.hd.UpdateStats(req, false, peer)
							cfg.hd.UpdateRetryTime(req, currentTime, 5*time.Second)
						}
					}
					if len(penalties) > 0 {
						cfg.penalize(ctx, penalties)
					}
					maxRequests--
				}

				// Send skeleton request if required
				if time.Since(lastSkeletonTime) > 1*time.Second {
					req = cfg.hd.RequestSkeleton()
					if req != nil {
						peer, sentToPeer = cfg.headerReqSend(ctx, req)
						if sentToPeer {
							cfg.hd.UpdateStats(req, true, peer)
							lastSkeletonTime = time.Now()
						}
					}
				}
				// Load headers into the database
				var inSync bool
				if inSync, err = cfg.hd.InsertHeaders(headerInserter.NewFeedHeaderFunc(tx, cfg.blockReader), cfg.chainConfig.TerminalTotalDifficulty, logPrefix, logEvery.C, uint64(currentTime.Unix())); err != nil {
					return err
				}

				if test {
					announces := cfg.hd.GrabAnnounces()
					if len(announces) > 0 {
						cfg.announceNewHashes(ctx, announces)
					}
				}

				if headerInserter.BestHeaderChanged() { // We do not break unless there best header changed
					noProgressCounter = 0
					wasProgress = true
					// if this is initial cycle, we want to make sure we insert all known headers (inSync)
					if inSync {
						break
					}
				}
				if test {
					break
				}
				timer := time.NewTimer(1 * time.Second)
				select {
				case <-ctx.Done():
					stopped = true
				case <-logEvery.C:
					progress := cfg.hd.Progress()
					logProgressHeaders(logPrefix, prevProgress, progress)
					stats := cfg.hd.ExtractStats()
					if prevProgress == progress {
						noProgressCounter++
					} else {
						noProgressCounter = 0 // Reset, there was progress
					}
					if noProgressCounter >= 5 {
						log.Info("Req/resp stats", "req", stats.Requests, "reqMin", stats.ReqMinBlock, "reqMax", stats.ReqMaxBlock,
							"skel", stats.SkeletonRequests, "skelMin", stats.SkeletonReqMinBlock, "skelMax", stats.SkeletonReqMaxBlock,
							"resp", stats.Responses, "respMin", stats.RespMinBlock, "respMax", stats.RespMaxBlock, "dups", stats.Duplicates)
						cfg.hd.LogAnchorState()
						if wasProgress {
							log.Warn("Looks like chain is not progressing, moving to the next stage")
							break Loop
						}
					}
					prevProgress = progress
				case <-timer.C:
					log.Trace("RequestQueueTime (header) ticked")
				case <-cfg.hd.DeliveryNotify:
					log.Trace("headerLoop woken up by the incoming request")
				}
				timer.Stop()
			}
			if headerInserter.Unwind() {
				u.UnwindTo(headerInserter.UnwindPoint(), libcommon.Hash{})
			}
			if headerInserter.GetHighest() != 0 {
				if !headerInserter.Unwind() {
					if err := fixCanonicalChain(logPrefix, logEvery, headerInserter.GetHighest(), headerInserter.GetHighestHash(), tx, cfg.blockReader); err != nil {
						return fmt.Errorf("fix canonical chain: %w", err)
					}
				}
				if err = rawdb.WriteHeadHeaderHash(tx, headerInserter.GetHighestHash()); err != nil {
					return fmt.Errorf("[%s] marking head header hash as %x: %w", logPrefix, headerInserter.GetHighestHash(), err)
				}
				if err = s.Update(tx, headerInserter.GetHighest()); err != nil {
					return fmt.Errorf("[%s] saving Headers progress: %w", logPrefix, err)
				}
			}
			if !useExternalTx {
				if err := tx.Commit(); err != nil {
					return err
				}
			}
			if stopped {
				return libcommon.ErrStopped
			}
			// We do not print the following line if the stage was interrupted
			log.Info(fmt.Sprintf("[%s] Processed", logPrefix), "highest inserted", headerInserter.GetHighest(), "age", common.PrettyAge(time.Unix(int64(headerInserter.GetHighestTimestamp()), 0)))

			return nil
	*/
}
