package main

import (
	"context"
	"fmt"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/beyondbrewing/brewery-bitcoin/config"
	"github.com/beyondbrewing/brewery-bitcoin/db"
	"github.com/beyondbrewing/brewery-bitcoin/indexer"
	"github.com/beyondbrewing/brewery-bitcoin/pkg/logger"
)

func main() {
	config.LoadConfig()
	logger.SetDefault(logger.MustProduction())
	defer logger.SyncDefault()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	store, err := db.Open(filepath.Join(config.BREWERY_DATA_DIR, "db"),
		db.WithColumnFamilies("headers"),
		db.WithWALDir(filepath.Join(config.BREWERY_DATA_DIR, "wal")),
		db.WithLogger(logger.Default()),
	)

	if err != nil {
		logger.Fatal("failed to create database", "error", err)
	}

	idx, err := indexer.New(
		indexer.WithNetwork(config.BREWERY_CHAINPARAM),
		indexer.WithMaxPeers(int(config.BREWERY_MAXPEERS)),
		indexer.WithPeers(config.BREWERY_ENODE...),
		indexer.WithLogger(logger.Default()),
		indexer.WithDb(store),
		// indexer.WithListeners(&btcclient.MessageListeners{}),
	)
	if err != nil {
		logger.Fatal("failed to create indexer", "error", err)
	}

	fmt.Println(idx, ctx)

	// if err := idx.Run(ctx); err != nil {
	// 	logger.Fatal("indexer error", "error", err)
	// }
}
