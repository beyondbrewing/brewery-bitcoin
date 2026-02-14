package db

import (
	"runtime"

	"github.com/beyondbrewing/brewery-bitcoin/pkg/logger"
)

// Config holds all tunable parameters for a [PebbleDB] instance.
// Use functional [Option] values with [Open] rather than constructing
// a Config directly.
type Config struct {
	// ColumnFamilies lists logical column families to register.
	// Pebble simulates CFs via key-prefixing; this list controls which
	// CF names are accepted by Store methods. The [DefaultColumnFamily]
	// ("default") is always included automatically.
	ColumnFamilies []string

	// --- Performance Tuning ---

	// CacheSize is the shared block-cache capacity in bytes.
	// A larger cache reduces read I/O at the cost of memory.
	CacheSize int64

	// MemTableSize is the size of a single memtable in bytes.
	// Larger memtables improve write throughput and reduce write
	// amplification, but increase memory usage.
	MemTableSize uint64

	// MaxConcurrentCompactions controls parallelism for background
	// compactions. Higher values speed up compaction at the cost of
	// I/O and CPU.
	MaxConcurrentCompactions int

	// MaxOpenFiles limits the number of open file descriptors Pebble
	// keeps open. Use 0 for unlimited (recommended for SSDs).
	MaxOpenFiles int

	// L0CompactionThreshold is the number of L0 sub-levels that trigger
	// a compaction into L1. Lower values reduce read amplification at
	// the cost of more compaction work.
	L0CompactionThreshold int

	// L0StopWritesThreshold is the hard limit on L0 sub-levels. When
	// reached, foreground writes stall until compaction catches up.
	L0StopWritesThreshold int

	// LBaseMaxBytes is the maximum total size of the base level (L1).
	// Pebble automatically sizes subsequent levels as multiples of this.
	LBaseMaxBytes int64

	// WALDir overrides the WAL directory. Leave empty to co-locate WAL
	// files with the database. Set to a faster device (e.g. NVMe) for
	// improved write latency.
	WALDir string

	// SyncWrites controls whether each write is synced to stable storage.
	// false (default) gives better throughput; true gives durability per
	// write at a significant performance cost. The WAL is still flushed
	// on Flush() and Close() regardless of this setting.
	SyncWrites bool

	// Logger receives structured operational log messages.
	// If not set, the global logger.Default() is used.
	Logger logger.Logger
}

// DefaultConfig returns a Config with production-ready defaults tuned for
// a Bitcoin indexer workload (large sequential writes, point lookups).
func DefaultConfig() *Config {
	return &Config{
		CacheSize:                256 << 20, // 256 MB
		MemTableSize:             64 << 20,  // 64 MB
		MaxConcurrentCompactions: runtime.NumCPU(),
		MaxOpenFiles:             0, // unlimited
		L0CompactionThreshold:    4,
		L0StopWritesThreshold:    12,
		LBaseMaxBytes:            256 << 20, // 256 MB
	}
}

// Option is a functional option applied to [Config] during [Open].
type Option func(*Config)

// WithColumnFamilies registers logical column families.
// The [DefaultColumnFamily] ("default") is always present regardless.
func WithColumnFamilies(cfs ...string) Option {
	return func(c *Config) { c.ColumnFamilies = cfs }
}

// WithCacheSize sets the shared block-cache capacity in bytes.
func WithCacheSize(size int64) Option {
	return func(c *Config) { c.CacheSize = size }
}

// WithMemTableSize sets the memtable size in bytes.
func WithMemTableSize(size uint64) Option {
	return func(c *Config) { c.MemTableSize = size }
}

// WithMaxConcurrentCompactions sets background compaction parallelism.
func WithMaxConcurrentCompactions(n int) Option {
	return func(c *Config) { c.MaxConcurrentCompactions = n }
}

// WithMaxOpenFiles limits the number of open file descriptors.
// Use 0 for unlimited.
func WithMaxOpenFiles(n int) Option {
	return func(c *Config) { c.MaxOpenFiles = n }
}

// WithL0CompactionThreshold sets the L0 sub-level compaction trigger.
func WithL0CompactionThreshold(n int) Option {
	return func(c *Config) { c.L0CompactionThreshold = n }
}

// WithL0StopWritesThreshold sets the L0 write-stall limit.
func WithL0StopWritesThreshold(n int) Option {
	return func(c *Config) { c.L0StopWritesThreshold = n }
}

// WithLBaseMaxBytes sets the max size of the base compaction level.
func WithLBaseMaxBytes(size int64) Option {
	return func(c *Config) { c.LBaseMaxBytes = size }
}

// WithWALDir sets a separate directory for write-ahead log files.
func WithWALDir(dir string) Option {
	return func(c *Config) { c.WALDir = dir }
}

// WithSyncWrites enables per-write durability (fsync). Significantly
// reduces throughput. Only enable when durability per operation is required.
func WithSyncWrites(sync bool) Option {
	return func(c *Config) { c.SyncWrites = sync }
}

// WithLogger sets a custom logger for the database.
// If not set, the global logger.Default() is used.
func WithLogger(l logger.Logger) Option {
	return func(c *Config) { c.Logger = l }
}
