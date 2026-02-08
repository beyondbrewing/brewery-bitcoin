package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/beyondbrewing/brewery-bitcoin/pkg/btcclient"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// headerIndexer tracks block headers received from the Bitcoin P2P network.
type headerIndexer struct {
	mu      sync.Mutex
	headers []wire.BlockHeader
	tipHash *chainhash.Hash
	params  *chaincfg.Params
	logger  *slog.Logger
}

func newHeaderIndexer(params *chaincfg.Params, logger *slog.Logger) *headerIndexer {
	return &headerIndexer{
		tipHash: params.GenesisHash,
		params:  params,
		logger:  logger,
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
		idx.logger.Info("header sync complete", "height", newHeight)
	}
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
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	params := &chaincfg.SigNetParams
	idx := newHeaderIndexer(params, logger)

	client, err := btcclient.NewBtcClient(
		btcclient.WithChainParams(params),
		btcclient.WithMaxPeers(8),
		btcclient.WithListeners(&btcclient.MessageListeners{
			OnHeaders:       idx.onHeaders,
			OnPeerConnected: idx.onPeerConnected,
			OnAddr: func(mp *btcclient.ManagedPeer, msg *wire.MsgAddr) {
				logger.Info("received peer addresses",
					"count", len(msg.AddrList),
					"from", mp.Address(),
				)
			},
		}),
	)
	if err != nil {
		logger.Error("failed to create client", "error", err)
		os.Exit(1)
	}

	if err := client.Start(); err != nil {
		logger.Error("failed to start client", "error", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// Seed peer for signet.
	if err := client.AddPeer(ctx, "seed.signet.bitcoin.sprovoost.nl:38333"); err != nil {
		logger.Error("failed to add seed peer", "error", err)
		os.Exit(1)
	}

	logger.Info("header indexer started", "network", params.Name)
	go func(c *btcclient.BtcClient) {

	}(client)
	// Wait for interrupt.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig

	logger.Info("shutting down", "height", idx.height())

	stopCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := client.Stop(stopCtx); err != nil {
		logger.Error("shutdown error", "error", err)
	}

}
