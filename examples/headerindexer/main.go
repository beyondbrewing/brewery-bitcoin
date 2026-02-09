package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/beyondbrewing/brewery-bitcoin/pkg/btcclient"
	"github.com/beyondbrewing/brewery-bitcoin/pkg/logger"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// headerIndexer tracks block headers received from the Bitcoin P2P network.
type headerIndexer struct {
	mu      sync.Mutex
	headers []wire.BlockHeader
	tipHash *chainhash.Hash
	synced  bool
	params  *chaincfg.Params
	logger  logger.Logger
}

func newHeaderIndexer(params *chaincfg.Params, log logger.Logger) *headerIndexer {
	return &headerIndexer{
		tipHash: params.GenesisHash,
		params:  params,
		logger:  log,
	}
}

// height returns the current synced header height.
func (idx *headerIndexer) height() int32 {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return int32(len(idx.headers))
}

// onPeerConnected sends a getheaders request starting from the current tip.
func (idx *headerIndexer) onPeerConnected(mp *btcclient.ManagedPeer) {
	idx.mu.Lock()
	tip := idx.tipHash
	idx.mu.Unlock()

	requestHeaders(mp, tip)
}

// onHeaders processes a batch of headers and requests more if needed.
func (idx *headerIndexer) onHeaders(mp *btcclient.ManagedPeer, msg *wire.MsgHeaders) {
	if len(msg.Headers) == 0 {
		return
	}

	idx.mu.Lock()
	startHeight := len(idx.headers)
	for _, hdr := range msg.Headers {
		idx.headers = append(idx.headers, *hdr)
		h := hdr.BlockHash()
		idx.tipHash = &h
	}
	newHeight := len(idx.headers)
	tip := idx.tipHash
	idx.mu.Unlock()

	idx.logger.Info("indexed headers",
		"from", startHeight,
		"to", newHeight,
		"batch", len(msg.Headers),
		"tip", tip.String(),
		"peer", mp.Address(),
	)

	// A full batch (2000) means there are more headers available.
	if len(msg.Headers) == 2000 {
		requestHeaders(mp, tip)
	} else {
		idx.mu.Lock()
		wasSynced := idx.synced
		idx.synced = true
		idx.mu.Unlock()

		if !wasSynced {
			idx.logger.Info("initial header sync complete, listening for new blocks", "height", newHeight)
		} else {
			idx.logger.Info("new headers indexed", "height", newHeight, "tip", tip.String())
		}
	}
}

// onInv handles inventory announcements. When a peer advertises new blocks,
// request the headers starting from our current tip.
func (idx *headerIndexer) onInv(mp *btcclient.ManagedPeer, msg *wire.MsgInv) {
	hasBlock := false
	for _, iv := range msg.InvList {
		if iv.Type == wire.InvTypeBlock || iv.Type == wire.InvTypeWitnessBlock {
			hasBlock = true
			break
		}
	}
	if !hasBlock {
		return
	}

	idx.mu.Lock()
	tip := idx.tipHash
	synced := idx.synced
	idx.mu.Unlock()

	if !synced {
		return
	}

	idx.logger.Info("new block announced, requesting headers", "peer", mp.Address())
	requestHeaders(mp, tip)
}

// requestHeaders sends a getheaders message starting from the given hash.
func requestHeaders(mp *btcclient.ManagedPeer, fromHash *chainhash.Hash) {
	p := mp.Peer()
	if p == nil {
		return
	}

	msg := wire.NewMsgGetHeaders()
	msg.AddBlockLocatorHash(fromHash)
	msg.HashStop = chainhash.Hash{}
	p.QueueMessage(msg, nil)
}

func main() {
	log := logger.MustDevelopment()

	params := &chaincfg.SigNetParams
	idx := newHeaderIndexer(params, log)

	client, err := btcclient.NewBtcClient(
		btcclient.WithChainParams(params),
		btcclient.WithMaxPeers(8),
		btcclient.WithLogger(log),
		btcclient.WithListeners(&btcclient.MessageListeners{
			OnHeaders:       idx.onHeaders,
			OnPeerConnected: idx.onPeerConnected,
			OnInv:           idx.onInv,
			OnAddr: func(mp *btcclient.ManagedPeer, msg *wire.MsgAddr) {
				log.Info("received peer addresses",
					"count", len(msg.AddrList),
					"from", mp.Address(),
				)
			},
		}),
	)
	if err != nil {
		log.Fatal("failed to create client", "error", err)
	}

	if err := client.Start(); err != nil {
		log.Fatal("failed to start client", "error", err)
	}

	ctx := context.Background()

	// Seed peers for signet.
	seedPeers := []string{
		"seed.signet.bitcoin.sprovoost.nl:38333",
		"seed.signet.achow101.com:38333",
		"seed.signet.bublina.eu.org:38333",
	}
	for _, addr := range seedPeers {
		if err := client.AddPeer(ctx, addr); err != nil {
			log.Fatal("failed to add seed peer", "error", err, "addr", addr)
		}
	}

	log.Info("header indexer started", "network", params.Name)

	// Wait for interrupt.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig

	log.Info("shutting down", "height", idx.height())

	stopCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := client.Stop(stopCtx); err != nil {
		log.Error("shutdown error", "error", err)
	}
}
