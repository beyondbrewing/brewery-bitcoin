package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/beyondbrewing/brewery-bitcoin/config"
	"github.com/beyondbrewing/brewery-bitcoin/indexer"
	"github.com/beyondbrewing/brewery-bitcoin/pkg/logger"
)

func main() {
	logger.SetDefault(logger.MustProduction())
	defer logger.SyncDefault()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	idx, err := indexer.New(
		indexer.WithNetwork(config.BREWERY_CHAINPARAM),
		indexer.WithMaxPeers(int(config.BREWERY_MAXPEERS)),
		indexer.WithPeers(config.BREWERY_ENODE...),
		indexer.WithLogger(logger.Default()),
		// indexer.WithListeners(&btcclient.MessageListeners{}),
	)
	if err != nil {
		logger.Fatal("failed to create indexer", "error", err)
	}

	if err := idx.Run(ctx); err != nil {
		logger.Fatal("indexer error", "error", err)
	}
}
