package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/beyondbrewing/brewery-bitcoin/pkg/btcclient"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	params := &chaincfg.MainNetParams

	client, err := btcclient.NewBtcClient(
		btcclient.WithChainParams(params),
		btcclient.WithMaxPeers(1),
		btcclient.WithListeners(&btcclient.MessageListeners{
			OnPeerConnected: func(mp *btcclient.ManagedPeer) {
				logger.Info("connected to peer", "address", mp.Address())
			},
			OnAddr: func(mp *btcclient.ManagedPeer, msg *wire.MsgAddr) {
				logger.Info("received peer addresses",
					"count", len(msg.AddrList),
					"from", mp.Address(),
				)
				for i, addr := range msg.AddrList {
					fmt.Printf("  [%d] %s:%d (services: %v, last seen: %s)\n",
						i+1,
						addr.IP.String(),
						addr.Port,
						addr.Services,
						addr.Timestamp.Format(time.RFC3339),
					)
				}
			},
			OnPeerDisconnected: func(mp *btcclient.ManagedPeer) {
				logger.Info("peer disconnected", "address", mp.Address())
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

	if err := client.AddPeer(ctx, "seed.bitcoin.sipa.be:8333"); err != nil {
		logger.Error("failed to add seed peer", "error", err)
		os.Exit(1)
	}

	logger.Info("requesting peers from seed node", "network", params.Name)

	// Periodically request peers from all connected nodes.

	// Wait for interrupt.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig

	logger.Info("shutting down")

	stopCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := client.Stop(stopCtx); err != nil {
		logger.Error("shutdown error", "error", err)
	}
}
