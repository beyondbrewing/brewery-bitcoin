package btcclient

import (
	"fmt"
	"time"

	"github.com/beyondbrewing/brewery-bitcoin/pkg/logger"
	"github.com/btcsuite/btcd/chaincfg"
)

// Config holds all configuration for a BtcClient instance.
type Config struct {
	ChainParams     *chaincfg.Params
	MaxPeers        int
	MaxFailureCount int32

	HealthCheckInterval time.Duration
	ReconnectDelay      time.Duration
	ConnectionTimeout   time.Duration

	Listeners *MessageListeners

	Logger logger.Logger
}

// Option is a functional option for configuring a BtcClient.
type Option func(*Config)

// DefaultConfig returns a production-ready configuration defaulting to mainnet.
func DefaultConfig() *Config {
	return &Config{
		ChainParams:         &chaincfg.MainNetParams,
		MaxPeers:            30,
		MaxFailureCount:     10,
		HealthCheckInterval: 30 * time.Second,
		ReconnectDelay:      5 * time.Second,
		ConnectionTimeout:   10 * time.Second,
		Listeners:           &MessageListeners{},
	}
}

// validate checks that all Config fields contain sane values.
func (c *Config) validate() error {
	if c.ChainParams == nil {
		return fmt.Errorf("%w: ChainParams must not be nil", ErrInvalidConfig)
	}
	if c.MaxPeers <= 0 {
		return fmt.Errorf("%w: MaxPeers must be positive, got %d", ErrInvalidConfig, c.MaxPeers)
	}
	if c.MaxFailureCount <= 0 {
		return fmt.Errorf("%w: MaxFailureCount must be positive, got %d", ErrInvalidConfig, c.MaxFailureCount)
	}
	if c.HealthCheckInterval <= 0 {
		return fmt.Errorf("%w: HealthCheckInterval must be positive", ErrInvalidConfig)
	}
	if c.ReconnectDelay <= 0 {
		return fmt.Errorf("%w: ReconnectDelay must be positive", ErrInvalidConfig)
	}
	if c.ConnectionTimeout <= 0 {
		return fmt.Errorf("%w: ConnectionTimeout must be positive", ErrInvalidConfig)
	}
	if c.Listeners == nil {
		c.Listeners = &MessageListeners{}
	}
	return nil
}

// WithChainParams sets the Bitcoin network parameters.
func WithChainParams(params *chaincfg.Params) Option {
	return func(c *Config) {
		c.ChainParams = params
	}
}

// WithMaxPeers sets the maximum number of concurrent peers.
func WithMaxPeers(n int) Option {
	return func(c *Config) { c.MaxPeers = n }
}

// WithMaxFailureCount sets the threshold before a peer is marked unhealthy.
func WithMaxFailureCount(n int32) Option {
	return func(c *Config) { c.MaxFailureCount = n }
}

// WithHealthCheckInterval sets how often peer health is evaluated.
func WithHealthCheckInterval(d time.Duration) Option {
	return func(c *Config) { c.HealthCheckInterval = d }
}

// WithReconnectDelay sets the backoff delay between reconnection attempts.
func WithReconnectDelay(d time.Duration) Option {
	return func(c *Config) { c.ReconnectDelay = d }
}

// WithConnectionTimeout sets the TCP dial timeout for peer connections.
func WithConnectionTimeout(d time.Duration) Option {
	return func(c *Config) { c.ConnectionTimeout = d }
}

// WithListeners sets the message listener callbacks.
func WithListeners(l *MessageListeners) Option {
	return func(c *Config) { c.Listeners = l }
}

// WithLogger sets a custom logger for the client.
// If not set, the global logger.Default() is used.
func WithLogger(l logger.Logger) Option {
	return func(c *Config) { c.Logger = l }
}
