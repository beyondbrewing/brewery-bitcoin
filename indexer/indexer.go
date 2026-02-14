// Package indexer provides a managed Bitcoin P2P indexing node built on top
// of the btcclient package. It handles peer resolution, lifecycle management,
// and graceful shutdown.
package indexer

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/beyondbrewing/brewery-bitcoin/db"
	"github.com/beyondbrewing/brewery-bitcoin/pkg/btcclient"
	"github.com/beyondbrewing/brewery-bitcoin/pkg/logger"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// Sentinel errors for the indexer package.
var (
	ErrAlreadyRunning = errors.New("indexer: already running")
	ErrNotRunning     = errors.New("indexer: not running")
	ErrUnknownNetwork = errors.New("indexer: unknown network")
	ErrNoPeers        = errors.New("indexer: no peers available")
)

// Config holds all settings for an Indexer instance.
type Config struct {
	// ChainParams specifies the Bitcoin network (mainnet, signet, testnet3, etc.).
	ChainParams *chaincfg.Params

	// MaxPeers limits the number of concurrent P2P connections.
	MaxPeers int

	// Peers is a list of explicit peer addresses to connect to.
	// DNS seeds from ChainParams are appended automatically.
	Peers []string

	// Listeners defines callbacks for P2P protocol messages.
	Listeners *btcclient.MessageListeners

	// ShutdownTimeout is the maximum duration to wait for a clean shutdown.
	ShutdownTimeout time.Duration

	// Logger is the structured logger. Falls back to logger.Default() if nil.
	Logger logger.Logger

	// Stores is a pointer to pebel db as a dependency injection that will be used for storage purpose
	Store *db.PebbleDB
}

// Option is a functional option for configuring an Indexer.
type Option func(*Config)

// DefaultConfig returns a Config with production-ready defaults.
func DefaultConfig() *Config {
	return &Config{
		ChainParams:     &chaincfg.MainNetParams,
		MaxPeers:        10,
		Listeners:       &btcclient.MessageListeners{},
		ShutdownTimeout: 10 * time.Second,
	}
}

func (c *Config) validate() error {
	if c.ChainParams == nil {
		return fmt.Errorf("%w: chain params must not be nil", ErrUnknownNetwork)
	}
	if c.MaxPeers <= 0 {
		return fmt.Errorf("indexer: max peers must be positive, got %d", c.MaxPeers)
	}
	if c.Listeners == nil {
		c.Listeners = &btcclient.MessageListeners{}
	}
	if c.ShutdownTimeout <= 0 {
		c.ShutdownTimeout = 10 * time.Second
	}
	return nil
}

// WithChainParams sets the Bitcoin network parameters directly.
func WithChainParams(params *chaincfg.Params) Option {
	return func(c *Config) { c.ChainParams = params }
}

// WithNetwork resolves a network name to chain parameters.
// Supported: "mainnet", "signet", "testnet3", "regtest", "simnet".
// Unknown names are silently ignored; use ResolveChainParams for error handling.
func WithNetwork(name string) Option {
	return func(c *Config) {
		if p, err := ResolveChainParams(name); err == nil {
			c.ChainParams = p
		}
	}
}

// WithMaxPeers sets the maximum number of concurrent peers.
func WithMaxPeers(n int) Option {
	return func(c *Config) { c.MaxPeers = n }
}

func WithDb(d *db.PebbleDB) Option {
	return func(c *Config) { c.Store = d }
}

// WithPeers sets explicit peer addresses to connect to.
func WithPeers(addrs ...string) Option {
	return func(c *Config) { c.Peers = addrs }
}

// WithListeners sets the message listener callbacks.
func WithListeners(l *btcclient.MessageListeners) Option {
	return func(c *Config) { c.Listeners = l }
}

// WithShutdownTimeout sets the maximum time to wait for graceful shutdown.
func WithShutdownTimeout(d time.Duration) Option {
	return func(c *Config) { c.ShutdownTimeout = d }
}

// WithLogger sets a structured logger for the indexer.
func WithLogger(l logger.Logger) Option {
	return func(c *Config) { c.Logger = l }
}

// Indexer manages the lifecycle of a Bitcoin P2P indexing node.
type Indexer struct {
	cfg    *Config
	client *btcclient.BtcClient
	logger logger.Logger

	mu      sync.Mutex
	tipHash *chainhash.Hash
	headers []wire.BlockHeader // NOTE : replace with RocksDB

	running bool
	synced  bool
}

// New creates an Indexer with the given options applied over DefaultConfig.
func New(opts ...Option) (*Indexer, error) {
	cfg := DefaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}

	log := cfg.Logger
	if log == nil {
		log = logger.Default()
	}
	log = log.With("component", "indexer")

	client, err := btcclient.NewBtcClient(
		btcclient.WithChainParams(cfg.ChainParams),
		btcclient.WithMaxPeers(cfg.MaxPeers),
		btcclient.WithLogger(log),
		btcclient.WithListeners(cfg.Listeners),
	)
	if err != nil {
		return nil, fmt.Errorf("indexer: failed to create btc client: %w", err)
	}

	return &Indexer{
		cfg:     cfg,
		client:  client,
		logger:  log,
		tipHash: cfg.ChainParams.GenesisHash, // read from database needed to be added here
	}, nil
}

// Run starts the indexer, connects to peers, and blocks until ctx is
// cancelled. It performs a graceful shutdown before returning.
func (idx *Indexer) Run(ctx context.Context) error {
	if err := idx.Start(); err != nil {
		return err
	}

	if err := idx.connectPeers(ctx); err != nil {
		_ = idx.Stop() // best-effort cleanup
		return err
	}

	idx.logger.Info("indexer running",
		"network", idx.cfg.ChainParams.Name,
		"peers", len(idx.resolvePeers()),
	)

	<-ctx.Done()

	idx.logger.Info("context cancelled, shutting down")
	return idx.Stop()
}

// Start initializes the underlying P2P client without blocking.
// Use Run for a blocking start-to-shutdown lifecycle.
func (idx *Indexer) Start() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.running {
		return ErrAlreadyRunning
	}

	if err := idx.client.Start(); err != nil {
		return fmt.Errorf("indexer: failed to start client: %w", err)
	}

	idx.running = true
	idx.logger.Info("indexer started", "network", idx.cfg.ChainParams.Name)
	return nil
}

// Stop performs a graceful shutdown within the configured timeout.
func (idx *Indexer) Stop() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if !idx.running {
		return ErrNotRunning
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), idx.cfg.ShutdownTimeout)
	defer cancel()

	err := idx.client.Stop(stopCtx)
	idx.running = false

	if err != nil {
		return fmt.Errorf("indexer: shutdown failed: %w", err)
	}

	idx.logger.Info("indexer stopped")
	return nil
}

// connectPeers resolves and connects to all configured peers.
// Returns an error only if every peer fails.
func (idx *Indexer) connectPeers(ctx context.Context) error {
	peers := idx.resolvePeers()
	if len(peers) == 0 {
		return ErrNoPeers
	}

	var errs []error
	for _, addr := range peers {
		if err := idx.client.AddPeer(ctx, addr); err != nil {
			idx.logger.Warn("failed to add peer", "address", addr, "error", err)
			errs = append(errs, fmt.Errorf("peer %s: %w", addr, err))
		}
	}

	if len(errs) == len(peers) {
		return fmt.Errorf("indexer: all peers failed: %w", errors.Join(errs...))
	}

	return nil
}

// resolvePeers returns deduplicated peers: explicit peers first,
// then DNS seeds from chain params.
func (idx *Indexer) resolvePeers() []string {
	seen := make(map[string]struct{}, len(idx.cfg.Peers))
	peers := make([]string, 0, len(idx.cfg.Peers))

	for _, p := range idx.cfg.Peers {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		peers = append(peers, p)
	}

	for _, seed := range idx.cfg.ChainParams.DNSSeeds {
		addr := fmt.Sprintf("%s:%s", seed.String(), idx.cfg.ChainParams.DefaultPort)
		if _, ok := seen[addr]; ok {
			continue
		}
		seen[addr] = struct{}{}
		peers = append(peers, addr)
	}

	return peers
}

// ResolveChainParams maps a network name to its btcd chain parameters.
// Returns ErrUnknownNetwork for unrecognized names.
func ResolveChainParams(name string) (*chaincfg.Params, error) {
	switch name {
	case "mainnet":
		return &chaincfg.MainNetParams, nil
	case "signet":
		return &chaincfg.SigNetParams, nil
	case "testnet3":
		return &chaincfg.TestNet3Params, nil
	case "regtest":
		return &chaincfg.RegressionNetParams, nil
	case "simnet":
		return &chaincfg.SimNetParams, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownNetwork, name)
	}
}
