/*
   Copyright 2021 Erigon contributors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package txpool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gateway-fm/cdk-erigon-lib/common/dbg"
	"github.com/gateway-fm/cdk-erigon-lib/direct"
	"github.com/gateway-fm/cdk-erigon-lib/gointerfaces/grpcutil"
	"github.com/gateway-fm/cdk-erigon-lib/gointerfaces/remote"
	"github.com/gateway-fm/cdk-erigon-lib/gointerfaces/sentry"
	"github.com/gateway-fm/cdk-erigon-lib/kv"
	"github.com/gateway-fm/cdk-erigon-lib/rlp"
	types2 "github.com/gateway-fm/cdk-erigon-lib/types"
	"github.com/holiman/uint256"
	"github.com/ledgerwatch/log/v3"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

// Fetch connects to sentry and implements eth/66 protocol regarding the transaction
// messages. It tries to "prime" the sentry with StatusData message containing given
// genesis hash and list of forks, but with zero max block and total difficulty
// Sentry should have a logic not to overwrite statusData with messages from tx pool
type Fetch struct {
	ctx                      context.Context // Context used for cancellation and closing of the fetcher
	pool                     Pool            // Transaction pool implementation
	coreDB                   kv.RoDB
	db                       kv.RwDB
	stateChangesClient       StateChangesClient
	wg                       *sync.WaitGroup // used for synchronisation in the tests (nil when not in tests)
	stateChangesParseCtx     *types2.TxParseContext
	pooledTxsParseCtx        *types2.TxParseContext
	sentryClients            []direct.SentryClient // sentry clients that will be used for accessing the network
	stateChangesParseCtxLock sync.Mutex
	pooledTxsParseCtxLock    sync.Mutex
}

type StateChangesClient interface {
	StateChanges(ctx context.Context, in *remote.StateChangeRequest, opts ...grpc.CallOption) (remote.KV_StateChangesClient, error)
}

// NewFetch creates a new fetch object that will work with given sentry clients. Since the
// SentryClient here is an interface, it is suitable for mocking in tests (mock will need
// to implement all the functions of the SentryClient interface).
func NewFetch(ctx context.Context, sentryClients []direct.SentryClient, pool Pool, stateChangesClient StateChangesClient, coreDB kv.RoDB, db kv.RwDB, chainID uint256.Int) *Fetch {
	f := &Fetch{
		ctx:                  ctx,
		sentryClients:        sentryClients,
		pool:                 pool,
		coreDB:               coreDB,
		db:                   db,
		stateChangesClient:   stateChangesClient,
		stateChangesParseCtx: types2.NewTxParseContext(chainID).ChainIDRequired(), //TODO: change ctx if rules changed
		pooledTxsParseCtx:    types2.NewTxParseContext(chainID).ChainIDRequired(),
	}
	f.pooledTxsParseCtx.ValidateRLP(f.pool.ValidateSerializedTxn)
	f.stateChangesParseCtx.ValidateRLP(f.pool.ValidateSerializedTxn)

	return f
}

func (f *Fetch) SetWaitGroup(wg *sync.WaitGroup) {
	f.wg = wg
}

func (f *Fetch) threadSafeParsePooledTxn(cb func(*types2.TxParseContext) error) error {
	f.pooledTxsParseCtxLock.Lock()
	defer f.pooledTxsParseCtxLock.Unlock()
	return cb(f.pooledTxsParseCtx)
}

func (f *Fetch) threadSafeParseStateChangeTxn(cb func(*types2.TxParseContext) error) error {
	f.stateChangesParseCtxLock.Lock()
	defer f.stateChangesParseCtxLock.Unlock()
	return cb(f.stateChangesParseCtx)
}

// ConnectSentries initialises connection to the sentry
func (f *Fetch) ConnectSentries() {
	for i := range f.sentryClients {
		go func(i int) {
			f.receiveMessageLoop(f.sentryClients[i])
		}(i)
		go func(i int) {
			f.receivePeerLoop(f.sentryClients[i])
		}(i)
	}
}
func (f *Fetch) ConnectCore() {
	go func() {
		for {
			select {
			case <-f.ctx.Done():
				return
			default:
			}
			if err := f.handleStateChanges(f.ctx, f.stateChangesClient); err != nil {
				if grpcutil.IsRetryLater(err) || grpcutil.IsEndOfStream(err) {
					time.Sleep(3 * time.Second)
					continue
				}
				log.Warn("[txpool.handleStateChanges]", "err", err)
			}
		}
	}()
}

func (f *Fetch) receiveMessageLoop(sentryClient sentry.SentryClient) {
	for {
		select {
		case <-f.ctx.Done():
			return
		default:
		}
		if _, err := sentryClient.HandShake(f.ctx, &emptypb.Empty{}, grpc.WaitForReady(true)); err != nil {
			if grpcutil.IsRetryLater(err) || grpcutil.IsEndOfStream(err) {
				time.Sleep(3 * time.Second)
				continue
			}
			// Report error and wait more
			log.Warn("[txpool.recvMessage] sentry not ready yet", "err", err)
			continue
		}

		if err := f.receiveMessage(f.ctx, sentryClient); err != nil {
			if grpcutil.IsRetryLater(err) || grpcutil.IsEndOfStream(err) {
				time.Sleep(3 * time.Second)
				continue
			}
			log.Warn("[txpool.recvMessage]", "err", err)
		}
	}
}

func (f *Fetch) receiveMessage(ctx context.Context, sentryClient sentry.SentryClient) error {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := sentryClient.Messages(streamCtx, &sentry.MessagesRequest{Ids: []sentry.MessageId{
		sentry.MessageId_NEW_POOLED_TRANSACTION_HASHES_66,
		sentry.MessageId_GET_POOLED_TRANSACTIONS_66,
		sentry.MessageId_TRANSACTIONS_66,
		sentry.MessageId_POOLED_TRANSACTIONS_66,
		sentry.MessageId_NEW_POOLED_TRANSACTION_HASHES_68,
	}}, grpc.WaitForReady(true))
	if err != nil {
		select {
		case <-f.ctx.Done():
			return ctx.Err()
		default:
		}
		return err
	}

	var req *sentry.InboundMessage
	for req, err = stream.Recv(); ; req, err = stream.Recv() {
		if err != nil {
			select {
			case <-f.ctx.Done():
				return ctx.Err()
			default:
			}
			return err
		}
		if req == nil {
			return nil
		}
		if err := f.handleInboundMessage(streamCtx, req, sentryClient); err != nil {
			if grpcutil.IsRetryLater(err) || grpcutil.IsEndOfStream(err) {
				time.Sleep(3 * time.Second)
				continue
			}

			if rlp.IsRLPError(err) {
				log.Debug("[txpool.fetch] Handling incoming message", "msg", req.Id.String(), "err", err)
			} else {
				log.Warn("[txpool.fetch] Handling incoming message", "msg", req.Id.String(), "err", err)
			}
		}
		if f.wg != nil {
			f.wg.Done()
		}
	}
}

func (f *Fetch) handleInboundMessage(ctx context.Context, req *sentry.InboundMessage, sentryClient sentry.SentryClient) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("%+v, trace: %s, rlp: %x", rec, dbg.Stack(), req.Data)
		}
	}()

	if !f.pool.Started() {
		return nil
	}
	tx, err := f.db.BeginRo(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	switch req.Id {
	case sentry.MessageId_NEW_POOLED_TRANSACTION_HASHES_66:
		hashCount, pos, err := types2.ParseHashesCount(req.Data, 0)
		if err != nil {
			return fmt.Errorf("parsing NewPooledTransactionHashes: %w", err)
		}
		var hashbuf [32]byte
		var unknownHashes types2.Hashes
		for i := 0; i < hashCount; i++ {
			_, pos, err = types2.ParseHash(req.Data, pos, hashbuf[:0])
			if err != nil {
				return fmt.Errorf("parsing NewPooledTransactionHashes: %w", err)
			}
			known, err := f.pool.IdHashKnown(tx, hashbuf[:])
			if err != nil {
				return err
			}
			if !known {
				unknownHashes = append(unknownHashes, hashbuf[:]...)
			}
		}
		if len(unknownHashes) > 0 {
			var encodedRequest []byte
			var messageID sentry.MessageId
			if encodedRequest, err = types2.EncodeGetPooledTransactions66(unknownHashes, uint64(1), nil); err != nil {
				return err
			}
			messageID = sentry.MessageId_GET_POOLED_TRANSACTIONS_66
			if _, err = sentryClient.SendMessageById(f.ctx, &sentry.SendMessageByIdRequest{
				Data:   &sentry.OutboundMessageData{Id: messageID, Data: encodedRequest},
				PeerId: req.PeerId,
			}, &grpc.EmptyCallOption{}); err != nil {
				return err
			}
		}
	case sentry.MessageId_NEW_POOLED_TRANSACTION_HASHES_68:
		_, _, hashes, _, err := rlp.ParseAnnouncements(req.Data, 0)
		if err != nil {
			return fmt.Errorf("parsing NewPooledTransactionHashes88: %w", err)
		}
		var unknownHashes types2.Hashes
		for i := 0; i < len(hashes); i += 32 {
			known, err := f.pool.IdHashKnown(tx, hashes[i:i+32])
			if err != nil {
				return err
			}
			if !known {
				unknownHashes = append(unknownHashes, hashes[i:i+32]...)
			}
		}
		if len(unknownHashes) > 0 {
			var encodedRequest []byte
			var messageID sentry.MessageId
			if encodedRequest, err = types2.EncodeGetPooledTransactions66(unknownHashes, uint64(1), nil); err != nil {
				return err
			}
			messageID = sentry.MessageId_GET_POOLED_TRANSACTIONS_66
			if _, err = sentryClient.SendMessageById(f.ctx, &sentry.SendMessageByIdRequest{
				Data:   &sentry.OutboundMessageData{Id: messageID, Data: encodedRequest},
				PeerId: req.PeerId,
			}, &grpc.EmptyCallOption{}); err != nil {
				return err
			}
		}
	case sentry.MessageId_GET_POOLED_TRANSACTIONS_66:
		//TODO: handleInboundMessage is single-threaded - means it can accept as argument couple buffers (or analog of txParseContext). Protobuf encoding will copy data anyway, but DirectClient doesn't
		var encodedRequest []byte
		var messageID sentry.MessageId
		switch req.Id {
		case sentry.MessageId_GET_POOLED_TRANSACTIONS_66:
			messageID = sentry.MessageId_POOLED_TRANSACTIONS_66
			requestID, hashes, _, err := types2.ParseGetPooledTransactions66(req.Data, 0, nil)
			if err != nil {
				return err
			}
			_ = requestID
			var txs [][]byte
			for i := 0; i < len(hashes); i += 32 {
				txn, err := f.pool.GetRlp(tx, hashes[i:i+32])
				if err != nil {
					return err
				}
				if txn == nil {
					continue
				}
				txs = append(txs, txn)
			}

			encodedRequest = types2.EncodePooledTransactions66(txs, requestID, nil)
		default:
			return fmt.Errorf("unexpected message: %s", req.Id.String())
		}

		if _, err := sentryClient.SendMessageById(f.ctx, &sentry.SendMessageByIdRequest{
			Data:   &sentry.OutboundMessageData{Id: messageID, Data: encodedRequest},
			PeerId: req.PeerId,
		}, &grpc.EmptyCallOption{}); err != nil {
			return err
		}
	case sentry.MessageId_POOLED_TRANSACTIONS_66, sentry.MessageId_TRANSACTIONS_66:
		txs := types2.TxSlots{}
		if err := f.threadSafeParsePooledTxn(func(parseContext *types2.TxParseContext) error {
			return nil
		}); err != nil {
			return err
		}

		switch req.Id {
		case sentry.MessageId_TRANSACTIONS_66:
			if err := f.threadSafeParsePooledTxn(func(parseContext *types2.TxParseContext) error {
				if _, err := types2.ParseTransactions(req.Data, 0, parseContext, &txs, func(hash []byte) error {
					known, err := f.pool.IdHashKnown(tx, hash)
					if err != nil {
						return err
					}
					if known {
						return types2.ErrRejected
					}
					return nil
				}); err != nil {
					return err
				}
				return nil
			}); err != nil {
				return err
			}
		case sentry.MessageId_POOLED_TRANSACTIONS_66:
			if err := f.threadSafeParsePooledTxn(func(parseContext *types2.TxParseContext) error {
				if _, _, err := types2.ParsePooledTransactions66(req.Data, 0, parseContext, &txs, func(hash []byte) error {
					known, err := f.pool.IdHashKnown(tx, hash)
					if err != nil {
						return err
					}
					if known {
						return types2.ErrRejected
					}
					return nil
				}); err != nil {
					return err
				}
				return nil
			}); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unexpected message: %s", req.Id.String())
		}
		if len(txs.Txs) == 0 {
			return nil
		}
		f.pool.AddRemoteTxs(ctx, txs)
	default:
		defer log.Trace("[txpool] dropped p2p message", "id", req.Id)
	}

	return nil
}

func (f *Fetch) receivePeerLoop(sentryClient sentry.SentryClient) {
	for {
		select {
		case <-f.ctx.Done():
			return
		default:
		}
		if _, err := sentryClient.HandShake(f.ctx, &emptypb.Empty{}, grpc.WaitForReady(true)); err != nil {
			if grpcutil.IsRetryLater(err) || grpcutil.IsEndOfStream(err) {
				time.Sleep(3 * time.Second)
				continue
			}
			// Report error and wait more
			log.Warn("[txpool.recvPeers] sentry not ready yet", "err", err)
			time.Sleep(time.Second)
			continue
		}
		if err := f.receivePeer(sentryClient); err != nil {
			if grpcutil.IsRetryLater(err) || grpcutil.IsEndOfStream(err) {
				time.Sleep(3 * time.Second)
				continue
			}

			log.Warn("[txpool.recvPeers]", "err", err)
		}
	}
}

func (f *Fetch) receivePeer(sentryClient sentry.SentryClient) error {
	streamCtx, cancel := context.WithCancel(f.ctx)
	defer cancel()

	stream, err := sentryClient.PeerEvents(streamCtx, &sentry.PeerEventsRequest{})
	if err != nil {
		select {
		case <-f.ctx.Done():
			return f.ctx.Err()
		default:
		}
		return err
	}

	var req *sentry.PeerEvent
	for req, err = stream.Recv(); ; req, err = stream.Recv() {
		if err != nil {
			return err
		}
		if req == nil {
			return nil
		}
		if err = f.handleNewPeer(req); err != nil {
			return err
		}
		if f.wg != nil {
			f.wg.Done()
		}
	}
}

func (f *Fetch) handleNewPeer(req *sentry.PeerEvent) error {
	if req == nil {
		return nil
	}
	switch req.EventId {
	case sentry.PeerEvent_Connect:
		f.pool.AddNewGoodPeer(req.PeerId)
	}

	return nil
}

func (f *Fetch) handleStateChanges(ctx context.Context, client StateChangesClient) error {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream, err := client.StateChanges(streamCtx, &remote.StateChangeRequest{WithStorage: false, WithTransactions: true}, grpc.WaitForReady(true))
	if err != nil {
		return err
	}
	for req, err := stream.Recv(); ; req, err = stream.Recv() {
		if err != nil {
			return err
		}
		if req == nil {
			return nil
		}

		var unwindTxs, minedTxs types2.TxSlots
		for _, change := range req.ChangeBatch {
			if change.Direction == remote.Direction_FORWARD {
				oldSize := len(minedTxs.Txs)
				minedTxs.Resize(uint(oldSize + len(change.Txs)))
				for i := range change.Txs {
					minedTxs.Txs[oldSize+i] = &types2.TxSlot{}
					if err = f.threadSafeParseStateChangeTxn(func(parseContext *types2.TxParseContext) error {
						_, err := parseContext.ParseTransaction(change.Txs[i], 0, minedTxs.Txs[oldSize+i], minedTxs.Senders.At(oldSize+i), false /* hasEnvelope */, nil)
						return err
					}); err != nil {
						log.Warn("stream.Recv", "err", err)
						continue
					}
				}
			}
			if change.Direction == remote.Direction_UNWIND {
				oldSize := len(unwindTxs.Txs)
				unwindTxs.Resize(uint(oldSize + len(change.Txs)))
				for i := range change.Txs {
					unwindTxs.Txs[oldSize+i] = &types2.TxSlot{}
					if err = f.threadSafeParseStateChangeTxn(func(parseContext *types2.TxParseContext) error {
						_, err = parseContext.ParseTransaction(change.Txs[i], 0, unwindTxs.Txs[oldSize+i], unwindTxs.Senders.At(oldSize+i), false /* hasEnvelope */, nil)
						return err
					}); err != nil {
						log.Warn("stream.Recv", "err", err)
						continue
					}
				}
			}
		}

		// transactions are marked as local in order bo by pass the spammer check
		markAsLocal(&unwindTxs)
		if err := f.db.View(ctx, func(tx kv.Tx) error {
			return f.pool.OnNewBlock(ctx, req, unwindTxs, minedTxs, tx)
		}); err != nil {
			log.Warn("onNewBlock", "err", err)
		}
		if f.wg != nil {
			f.wg.Done()
		}
	}
}
