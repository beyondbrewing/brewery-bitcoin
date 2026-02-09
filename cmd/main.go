package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/beyondbrewing/brewery-bitcoin/indexer"
	"github.com/beyondbrewing/brewery-bitcoin/pkg/logger"
)

func main() {
	logger.SetDefault(logger.MustProduction())
	defer logger.SyncDefault()

	// Create a context that is cancelled on SIGINT or SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := indexer.Start(ctx); err != nil {
		logger.Fatal("indexer error", "error", err)
	}
}
