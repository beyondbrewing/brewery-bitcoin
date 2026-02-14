package db

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/beyondbrewing/brewery-bitcoin/pkg/logger"
	"github.com/cockroachdb/pebble"
)

// Compile-time interface check.
var _ Store = (*PebbleDB)(nil)

// PebbleDB is a production [Store] backed by Pebble. It is safe for
// concurrent use — Pebble handles its own internal synchronisation.
//
// Column families are simulated via key-prefixing: each logical CF name
// is mapped to a byte prefix (cf + '\x00'), keeping data from different
// families sorted in disjoint key ranges.
type PebbleDB struct {
	db *pebble.DB

	// prefixes maps registered CF names to their byte prefix.
	// Immutable after construction — safe for concurrent reads.
	prefixes map[string][]byte

	writeOpts *pebble.WriteOptions
	path      string
	logger    logger.Logger

	// closed + mu guard against use-after-close. Individual operations
	// take an RLock (allowing full concurrency). Close takes the write
	// lock, draining in-flight operations before teardown.
	closed atomic.Bool
	mu     sync.RWMutex
}

// Open creates or opens a Pebble database at path with the given options.
// The caller must call Close when done to release all resources.
func Open(path string, opts ...Option) (*PebbleDB, error) {
	cfg := DefaultConfig()
	for _, o := range opts {
		o(cfg)
	}

	log := cfg.Logger
	if log == nil {
		log = logger.Default()
	}
	log = log.With("component", "db")

	// --- Build Pebble options ---

	cache := pebble.NewCache(cfg.CacheSize)
	defer cache.Unref()

	pOpts := &pebble.Options{
		Cache:                       cache,
		MemTableSize:                cfg.MemTableSize,
		MaxOpenFiles:                cfg.MaxOpenFiles,
		MaxConcurrentCompactions:    func() int { return cfg.MaxConcurrentCompactions },
		L0CompactionThreshold:       cfg.L0CompactionThreshold,
		L0StopWritesThreshold:       cfg.L0StopWritesThreshold,
		LBaseMaxBytes:               cfg.LBaseMaxBytes,
		WALDir:                      cfg.WALDir,
		DisableAutomaticCompactions: false,
	}

	db, err := pebble.Open(path, pOpts)
	if err != nil {
		return nil, fmt.Errorf("db: failed to open %s: %w", path, err)
	}

	// --- Build CF prefix map ---

	prefixes := make(map[string][]byte, 1+len(cfg.ColumnFamilies))
	prefixes[DefaultColumnFamily] = cfPrefix(DefaultColumnFamily)
	for _, cf := range cfg.ColumnFamilies {
		if cf != DefaultColumnFamily {
			prefixes[cf] = cfPrefix(cf)
		}
	}

	writeOpts := pebble.NoSync
	if cfg.SyncWrites {
		writeOpts = pebble.Sync
	}

	pdb := &PebbleDB{
		db:        db,
		prefixes:  prefixes,
		writeOpts: writeOpts,
		path:      path,
		logger:    log,
	}

	log.Info("database opened",
		"path", path,
		"column_families", fmt.Sprintf("%v", cfg.ColumnFamilies),
	)
	return pdb, nil
}

// ---------------------------------------------------------------------------
// Store implementation
// ---------------------------------------------------------------------------

func (p *PebbleDB) Get(cf string, key []byte) ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed.Load() {
		return nil, ErrClosed
	}
	if key == nil {
		return nil, ErrNilKey
	}

	prefix, err := p.cfPrefix(cf)
	if err != nil {
		return nil, err
	}

	val, closer, err := p.db.Get(prefixedKey(prefix, key))
	if err != nil {
		if errors.Is(err, pebble.ErrNotFound) {
			return nil, ErrKeyNotFound
		}
		return nil, fmt.Errorf("db: get failed: %w", err)
	}
	defer closer.Close()

	// Copy — the returned slice is only valid until closer.Close().
	out := make([]byte, len(val))
	copy(out, val)
	return out, nil
}

func (p *PebbleDB) Put(cf string, key, value []byte) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed.Load() {
		return ErrClosed
	}
	if key == nil {
		return ErrNilKey
	}

	prefix, err := p.cfPrefix(cf)
	if err != nil {
		return err
	}

	if err := p.db.Set(prefixedKey(prefix, key), value, p.writeOpts); err != nil {
		return fmt.Errorf("db: put failed: %w", err)
	}
	return nil
}

func (p *PebbleDB) Delete(cf string, key []byte) error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed.Load() {
		return ErrClosed
	}
	if key == nil {
		return ErrNilKey
	}

	prefix, err := p.cfPrefix(cf)
	if err != nil {
		return err
	}

	if err := p.db.Delete(prefixedKey(prefix, key), p.writeOpts); err != nil {
		return fmt.Errorf("db: delete failed: %w", err)
	}
	return nil
}

func (p *PebbleDB) Has(cf string, key []byte) (bool, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed.Load() {
		return false, ErrClosed
	}
	if key == nil {
		return false, ErrNilKey
	}

	prefix, err := p.cfPrefix(cf)
	if err != nil {
		return false, err
	}

	_, closer, err := p.db.Get(prefixedKey(prefix, key))
	if err != nil {
		if errors.Is(err, pebble.ErrNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("db: has failed: %w", err)
	}
	closer.Close()
	return true, nil
}

func (p *PebbleDB) NewBatch() Batch {
	return &pebbleBatch{
		owner: p,
		batch: p.db.NewBatch(),
	}
}

func (p *PebbleDB) NewIterator(cf string) (Iterator, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed.Load() {
		return nil, ErrClosed
	}

	prefix, err := p.cfPrefix(cf)
	if err != nil {
		return nil, err
	}

	iter, err := p.db.NewIter(&pebble.IterOptions{
		LowerBound: prefix,
		UpperBound: cfUpperBound(cf),
	})
	if err != nil {
		return nil, fmt.Errorf("db: new iterator failed: %w", err)
	}

	return &pebbleIterator{
		iter:      iter,
		prefix:    prefix,
		prefixLen: len(prefix),
	}, nil
}

func (p *PebbleDB) Flush() error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.closed.Load() {
		return ErrClosed
	}

	if err := p.db.Flush(); err != nil {
		return fmt.Errorf("db: flush failed: %w", err)
	}
	return nil
}

// Close performs a graceful shutdown. It acquires an exclusive lock so
// all in-flight operations complete before teardown.
func (p *PebbleDB) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed.Load() {
		return ErrClosed
	}
	p.closed.Store(true)

	p.logger.Info("closing database", "path", p.path)

	if err := p.db.Flush(); err != nil {
		p.logger.Error("flush failed during shutdown", "error", err)
	}

	if err := p.db.Close(); err != nil {
		return fmt.Errorf("db: close failed: %w", err)
	}

	p.logger.Info("database closed", "path", p.path)
	return nil
}

// ---------------------------------------------------------------------------
// Batch implementation
// ---------------------------------------------------------------------------

type pebbleBatch struct {
	owner  *PebbleDB
	batch  *pebble.Batch
	closed bool
}

func (b *pebbleBatch) Put(cf string, key, value []byte) error {
	if b.closed {
		return ErrBatchClosed
	}
	if key == nil {
		return ErrNilKey
	}
	prefix, err := b.owner.cfPrefix(cf)
	if err != nil {
		return err
	}
	if err := b.batch.Set(prefixedKey(prefix, key), value, nil); err != nil {
		return fmt.Errorf("db: batch put failed: %w", err)
	}
	return nil
}

func (b *pebbleBatch) Delete(cf string, key []byte) error {
	if b.closed {
		return ErrBatchClosed
	}
	if key == nil {
		return ErrNilKey
	}
	prefix, err := b.owner.cfPrefix(cf)
	if err != nil {
		return err
	}
	if err := b.batch.Delete(prefixedKey(prefix, key), nil); err != nil {
		return fmt.Errorf("db: batch delete failed: %w", err)
	}
	return nil
}

func (b *pebbleBatch) Count() int {
	return int(b.batch.Count())
}

func (b *pebbleBatch) Commit() error {
	if b.closed {
		return ErrBatchClosed
	}

	b.owner.mu.RLock()
	defer b.owner.mu.RUnlock()

	if b.owner.closed.Load() {
		return ErrClosed
	}

	if err := b.batch.Commit(b.owner.writeOpts); err != nil {
		return fmt.Errorf("db: batch commit failed: %w", err)
	}
	return nil
}

func (b *pebbleBatch) Close() {
	if !b.closed {
		_ = b.batch.Close()
		b.closed = true
	}
}

// ---------------------------------------------------------------------------
// Iterator implementation
// ---------------------------------------------------------------------------

type pebbleIterator struct {
	iter      *pebble.Iterator
	prefix    []byte
	prefixLen int
	closed    bool
	err       error
}

func (it *pebbleIterator) Seek(target []byte) {
	it.iter.SeekGE(prefixedKey(it.prefix, target))
}

func (it *pebbleIterator) SeekToFirst() { it.iter.First() }
func (it *pebbleIterator) SeekToLast()  { it.iter.Last() }
func (it *pebbleIterator) Next()        { it.iter.Next() }
func (it *pebbleIterator) Prev()        { it.iter.Prev() }
func (it *pebbleIterator) Valid() bool  { return it.iter.Valid() }

func (it *pebbleIterator) Key() []byte {
	raw := it.iter.Key()
	if len(raw) < it.prefixLen {
		return nil
	}
	// Strip the CF prefix and return a copy.
	stripped := raw[it.prefixLen:]
	out := make([]byte, len(stripped))
	copy(out, stripped)
	return out
}

func (it *pebbleIterator) Value() []byte {
	val, err := it.iter.ValueAndErr()
	if err != nil {
		it.err = err
		return nil
	}
	out := make([]byte, len(val))
	copy(out, val)
	return out
}

func (it *pebbleIterator) Err() error {
	if it.err != nil {
		return it.err
	}
	return it.iter.Error()
}

func (it *pebbleIterator) Close() {
	if !it.closed {
		_ = it.iter.Close()
		it.closed = true
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// cfPrefix returns the registered prefix for the given column family name.
func (p *PebbleDB) cfPrefix(cf string) ([]byte, error) {
	prefix, ok := p.prefixes[cf]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrColumnFamilyNotFound, cf)
	}
	return prefix, nil
}

// cfPrefix builds the key prefix for a column family: "cf\x00".
func cfPrefix(cf string) []byte {
	b := make([]byte, len(cf)+1)
	copy(b, cf)
	b[len(cf)] = 0x00
	return b
}

// cfUpperBound builds the exclusive upper bound for iteration: "cf\x01".
func cfUpperBound(cf string) []byte {
	b := make([]byte, len(cf)+1)
	copy(b, cf)
	b[len(cf)] = 0x01
	return b
}

// prefixedKey concatenates a CF prefix and a user key into a single
// storage key: prefix + key.
func prefixedKey(prefix, key []byte) []byte {
	pk := make([]byte, len(prefix)+len(key))
	copy(pk, prefix)
	copy(pk[len(prefix):], key)
	return pk
}
