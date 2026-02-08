package btcclient

import (
	"context"
	"log/slog"
	"sync"
)

// BtcClient is the top-level entry point for managing Bitcoin P2P connections.
type BtcClient struct {
	manager *PeerManager
	mu      sync.RWMutex
	logger  *slog.Logger
}

// NewBtcClient creates a new client with the given functional options applied
// over DefaultConfig. Returns an error if the resulting config is invalid.
func NewBtcClient(opts ...Option) (*BtcClient, error) {
	cfg := DefaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	logger := slog.Default().With("component", "btcclient")

	return &BtcClient{
		manager: newPeerManager(cfg, logger),
		logger:  logger,
	}, nil
}

// Start begins the health-check loop. Peers can be added before or after Start.
func (bc *BtcClient) Start() error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if bc.manager.started.Load() {
		return ErrClientAlreadyStarted
	}

	bc.manager.started.Store(true)

	bc.manager.wg.Add(1)
	go bc.manager.healthCheckLoop()

	bc.logger.Info("client started")
	return nil
}

// Stop gracefully shuts down all peers and waits for goroutines to finish.
// The provided context controls the maximum time to wait for a clean shutdown.
func (bc *BtcClient) Stop(ctx context.Context) error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	if !bc.manager.started.Load() {
		return ErrClientNotStarted
	}

	close(bc.manager.done)

	bc.manager.peersMu.Lock()
	for _, mp := range bc.manager.peers {
		mp.cancel()
		if p := mp.Peer(); p != nil {
			p.Disconnect()
		}
	}
	bc.manager.peersMu.Unlock()

	done := make(chan struct{})
	go func() {
		bc.manager.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		bc.logger.Info("client stopped gracefully")
	case <-ctx.Done():
		return ErrStopTimeout
	}

	bc.manager.started.Store(false)
	return nil
}

// AddPeer registers and connects to a new peer at the given address.
func (bc *BtcClient) AddPeer(ctx context.Context, address string) error {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	bc.manager.peersMu.Lock()
	defer bc.manager.peersMu.Unlock()

	return bc.manager.addPeer(ctx, address)
}

// RemovePeer disconnects and removes the peer at the given address.
func (bc *BtcClient) RemovePeer(address string) error {
	bc.mu.RLock()
	defer bc.mu.RUnlock()

	bc.manager.peersMu.Lock()
	defer bc.manager.peersMu.Unlock()

	return bc.manager.removePeer(address)
}

// PeerCount returns the current number of managed peers.
func (bc *BtcClient) PeerCount() int {
	bc.manager.peersMu.RLock()
	defer bc.manager.peersMu.RUnlock()
	return len(bc.manager.peers)
}
