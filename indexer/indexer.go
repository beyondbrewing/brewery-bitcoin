package indexer

import (
	"context"
	"time"

	"github.com/beyondbrewing/brewery-bitcoin/pkg/btcclient"
	"github.com/beyondbrewing/brewery-bitcoin/pkg/logger"
)

// StartIndexer runs the indexer until ctx is cancelled, then gracefully
// shuts down the BtcClient and all its peers before returning.
func StartIndexer(ctx context.Context) error {
	log := logger.Default().With("component", "indexer")

	client, err := btcclient.NewBtcClient()
	if err != nil {
		return err
	}

	if err := client.Start(); err != nil {
		return err
	}

	log.Info("indexer started")

	// Block until the caller cancels the context.
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
