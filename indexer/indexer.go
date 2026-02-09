package indexer

import (
	"context"
	"fmt"
	"time"

	"github.com/beyondbrewing/brewery-bitcoin/config"
	"github.com/beyondbrewing/brewery-bitcoin/pkg/btcclient"
	"github.com/beyondbrewing/brewery-bitcoin/pkg/logger"
	"github.com/btcsuite/btcd/chaincfg"
)

func Start(ctx context.Context) error {
	log := logger.MustProduction().With("component", "indexer")
	client, err := btcclient.NewBtcClient(
		btcclient.WithChainParams(returnChainParams(config.BREWERY_CHAINPARAM)),
		btcclient.WithMaxPeers(int(config.BREWERY_MAXPEERS)),
		btcclient.WithLogger(log),
		btcclient.WithListeners(&btcclient.MessageListeners{}), // need to add functions here
	)

	if err != nil {
		return err
	}

	if err := client.Start(); err != nil {
		return err
	}
	for _, addr := range returnPeers(config.BREWERY_CHAINPARAM, config.BREWERY_ENODE) {
		if err := client.AddPeer(ctx, addr); err != nil {
			log.Fatal("failed to add seed peer", "error", err, "addr", addr)
		}
	}

	<-ctx.Done()

	log.Info("context cancelled, shutting down")

	stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Stop(stopCtx); err != nil {
		return err
	}

	log.Info("indexer stopped")
	return nil
}

// add config peers apart from the enode provided
func returnPeers(chain string, enode []string) []string {
	var peers []string
	if len(enode) > 0 {
		peers = append(peers, enode...)
	}

	chainparam := returnChainParams(chain)
	for _, seed := range chainparam.DNSSeeds {
		peers = append(peers, fmt.Sprintf("%s:%s", seed.String(), chainparam.DefaultPort))
	}
	return peers
}

func returnChainParams(chain string) *chaincfg.Params {
	switch chain {
	case "mainnet":
		return &chaincfg.MainNetParams
	case "signet":
		return &chaincfg.SigNetParams
	case "testnet3":
		return &chaincfg.TestNet3Params
	case "regtest":
		return &chaincfg.RegressionNetParams
	case "simnet":
		return &chaincfg.SimNetParams
	default:
		return &chaincfg.MainNetParams
	}
}
