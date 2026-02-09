package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/beyondbrewing/brewery-bitcoin/pkg/btcclient"
	"github.com/beyondbrewing/brewery-bitcoin/pkg/logger"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
)

func main() {
	log := logger.MustDevelopment()

	params := &chaincfg.MainNetParams

	client, err := btcclient.NewBtcClient(
		btcclient.WithChainParams(params),
		btcclient.WithMaxPeers(1),
		btcclient.WithLogger(log),
		btcclient.WithListeners(&btcclient.MessageListeners{
			OnPeerConnected: func(mp *btcclient.ManagedPeer) {
				log.Info("connected to peer", "address", mp.Address())
			},
			OnAddr: func(mp *btcclient.ManagedPeer, msg *wire.MsgAddr) {
				log.Info("received peer addresses",
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
				log.Info("peer disconnected", "address", mp.Address())
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

	if err := client.AddPeer(ctx, "seed.bitcoin.sipa.be:8333"); err != nil {
		log.Fatal("failed to add seed peer", "error", err)
	}

	log.Info("requesting peers from seed node", "network", params.Name)

	// Wait for interrupt.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig

	log.Info("shutting down")

	stopCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	if err := client.Stop(stopCtx); err != nil {
		log.Error("shutdown error", "error", err)
	}
}
