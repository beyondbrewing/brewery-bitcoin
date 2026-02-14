// Package db provides a storage abstraction backed by Pebble with support for
// logical column families (via key-prefixing), atomic batch writes, ordered
// iteration, and graceful shutdown.
//
// The primary interface is [Store], satisfied by [PebbleDB] (production) and
// [MockStore] (testing). Create instances with [Open] or [NewMockStore] and
// inject them into consumers via constructor arguments or functional options.
package db

import (
	"errors"
	"io"
)

// Sentinel errors returned by Store implementations.
var (
	ErrClosed               = errors.New("db: database is closed")
	ErrColumnFamilyNotFound = errors.New("db: column family not found")
	ErrKeyNotFound          = errors.New("db: key not found")
	ErrNilKey               = errors.New("db: key must not be nil")
	ErrBatchClosed          = errors.New("db: batch is closed")
)

// DefaultColumnFamily is the column family used when no explicit family is
// specified. It is always registered automatically.
const DefaultColumnFamily = "default"

// Store defines the contract for all database operations.
// All methods are safe for concurrent use by multiple goroutines.
type Store interface {
	// Get retrieves the value for a key in the given column family.
	// Returns ErrKeyNotFound if the key does not exist.
	// Returns ErrColumnFamilyNotFound if the column family is unknown.
	Get(cf string, key []byte) ([]byte, error)

	// Put stores a key-value pair in the given column family.
	Put(cf string, key []byte, value []byte) error

	// Delete removes a key from the given column family.
	// Deleting a non-existent key is not an error.
	Delete(cf string, key []byte) error

	// Has reports whether a key exists in the given column family.
	Has(cf string, key []byte) (bool, error)

	// NewBatch creates an atomic write batch. Operations are buffered in
	// memory and applied atomically when Commit is called. The caller must
	// call Close when the batch is no longer needed.
	NewBatch() Batch

	// NewIterator creates a forward/backward iterator scoped to the given
	// column family. The caller must call Close on the returned Iterator.
	NewIterator(cf string) (Iterator, error)

	// Flush forces all buffered writes (memtable) to persistent storage.
	Flush() error

	// Close performs a graceful shutdown: flushes pending writes, closes
	// the underlying engine, and releases all resources.
	// After Close returns, every other method returns ErrClosed.
	io.Closer
}

// Batch is an atomic write batch. Operations are buffered in memory and
// applied atomically on Commit.
type Batch interface {
	// Put stages a key-value write in the given column family.
	Put(cf string, key []byte, value []byte) error

	// Delete stages a key deletion in the given column family.
	Delete(cf string, key []byte) error

	// Count returns the number of staged operations.
	Count() int

	// Commit atomically applies all staged operations.
	Commit() error

	// Close releases batch resources. Must be called even after Commit.
	Close()
}

// Iterator provides ordered traversal over keys in a single column family.
// Key and Value return copies that remain valid after the iterator advances.
type Iterator interface {
	// Seek positions the iterator at the first key >= target.
	Seek(target []byte)

	// SeekToFirst positions the iterator at the first key.
	SeekToFirst()

	// SeekToLast positions the iterator at the last key.
	SeekToLast()

	// Next advances the iterator by one key.
	Next()

	// Prev moves the iterator back by one key.
	Prev()

	// Valid reports whether the iterator is positioned at a valid entry.
	Valid() bool

	// Key returns a copy of the current key (prefix-stripped).
	// Only valid when Valid() is true.
	Key() []byte

	// Value returns a copy of the current value.
	// Only valid when Valid() is true.
	Value() []byte

	// Err returns any accumulated error from the underlying engine.
	Err() error

	// Close releases iterator resources.
	Close()
}
