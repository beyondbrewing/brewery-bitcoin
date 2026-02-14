package db

import (
	"bytes"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
)

// Compile-time interface check.
var _ Store = (*MockStore)(nil)

// MockStore is a fully functional, thread-safe, in-memory implementation of
// [Store]. It requires no external dependencies — ideal for unit and
// integration tests.
//
//	store := db.NewMockStore("headers", "blocks")
//	defer store.Close()
type MockStore struct {
	mu     sync.RWMutex
	data   map[string]map[string][]byte // cf -> key(string) -> value
	closed atomic.Bool
}

// NewMockStore creates a MockStore with the given column families.
// The [DefaultColumnFamily] ("default") is always included.
func NewMockStore(cfs ...string) *MockStore {
	m := &MockStore{
		data: make(map[string]map[string][]byte, 1+len(cfs)),
	}
	m.data[DefaultColumnFamily] = make(map[string][]byte)
	for _, cf := range cfs {
		if cf != DefaultColumnFamily {
			m.data[cf] = make(map[string][]byte)
		}
	}
	return m
}

// ---------------------------------------------------------------------------
// Store implementation
// ---------------------------------------------------------------------------

func (m *MockStore) Get(cf string, key []byte) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed.Load() {
		return nil, ErrClosed
	}
	if key == nil {
		return nil, ErrNilKey
	}

	bucket, ok := m.data[cf]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrColumnFamilyNotFound, cf)
	}

	v, ok := bucket[string(key)]
	if !ok {
		return nil, ErrKeyNotFound
	}

	out := make([]byte, len(v))
	copy(out, v)
	return out, nil
}

func (m *MockStore) Put(cf string, key, value []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed.Load() {
		return ErrClosed
	}
	if key == nil {
		return ErrNilKey
	}

	bucket, ok := m.data[cf]
	if !ok {
		return fmt.Errorf("%w: %q", ErrColumnFamilyNotFound, cf)
	}

	v := make([]byte, len(value))
	copy(v, value)
	bucket[string(key)] = v
	return nil
}

func (m *MockStore) Delete(cf string, key []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed.Load() {
		return ErrClosed
	}
	if key == nil {
		return ErrNilKey
	}

	bucket, ok := m.data[cf]
	if !ok {
		return fmt.Errorf("%w: %q", ErrColumnFamilyNotFound, cf)
	}

	delete(bucket, string(key))
	return nil
}

func (m *MockStore) Has(cf string, key []byte) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed.Load() {
		return false, ErrClosed
	}
	if key == nil {
		return false, ErrNilKey
	}

	bucket, ok := m.data[cf]
	if !ok {
		return false, fmt.Errorf("%w: %q", ErrColumnFamilyNotFound, cf)
	}

	_, exists := bucket[string(key)]
	return exists, nil
}

func (m *MockStore) NewBatch() Batch {
	return &mockBatch{store: m}
}

func (m *MockStore) NewIterator(cf string) (Iterator, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed.Load() {
		return nil, ErrClosed
	}

	bucket, ok := m.data[cf]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrColumnFamilyNotFound, cf)
	}

	// Snapshot: sorted copy of the current data.
	keys := make([]string, 0, len(bucket))
	for k := range bucket {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	entries := make([]mockEntry, len(keys))
	for i, k := range keys {
		v := make([]byte, len(bucket[k]))
		copy(v, bucket[k])
		entries[i] = mockEntry{key: []byte(k), value: v}
	}

	return &mockIterator{entries: entries, pos: -1}, nil
}

func (m *MockStore) Flush() error {
	if m.closed.Load() {
		return ErrClosed
	}
	return nil // in-memory — nothing to flush
}

func (m *MockStore) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed.Load() {
		return ErrClosed
	}
	m.closed.Store(true)
	m.data = nil
	return nil
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// Len returns the number of keys in the given column family. Returns -1 if
// the column family does not exist or the store is closed.
func (m *MockStore) Len(cf string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.closed.Load() {
		return -1
	}
	bucket, ok := m.data[cf]
	if !ok {
		return -1
	}
	return len(bucket)
}

// Reset clears all data in every column family without closing the store.
func (m *MockStore) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for cf := range m.data {
		m.data[cf] = make(map[string][]byte)
	}
}

// ---------------------------------------------------------------------------
// Batch implementation
// ---------------------------------------------------------------------------

type mockOp struct {
	del   bool
	cf    string
	key   string
	value []byte
}

type mockBatch struct {
	store  *MockStore
	ops    []mockOp
	closed bool
}

func (b *mockBatch) Put(cf string, key, value []byte) error {
	if b.closed {
		return ErrBatchClosed
	}
	if key == nil {
		return ErrNilKey
	}
	// CF map keys are immutable after construction — safe without lock.
	if _, ok := b.store.data[cf]; !ok {
		return fmt.Errorf("%w: %q", ErrColumnFamilyNotFound, cf)
	}
	v := make([]byte, len(value))
	copy(v, value)
	b.ops = append(b.ops, mockOp{cf: cf, key: string(key), value: v})
	return nil
}

func (b *mockBatch) Delete(cf string, key []byte) error {
	if b.closed {
		return ErrBatchClosed
	}
	if key == nil {
		return ErrNilKey
	}
	if _, ok := b.store.data[cf]; !ok {
		return fmt.Errorf("%w: %q", ErrColumnFamilyNotFound, cf)
	}
	b.ops = append(b.ops, mockOp{del: true, cf: cf, key: string(key)})
	return nil
}

func (b *mockBatch) Count() int {
	return len(b.ops)
}

func (b *mockBatch) Commit() error {
	if b.closed {
		return ErrBatchClosed
	}

	b.store.mu.Lock()
	defer b.store.mu.Unlock()

	if b.store.closed.Load() {
		return ErrClosed
	}

	for _, op := range b.ops {
		if op.del {
			delete(b.store.data[op.cf], op.key)
		} else {
			b.store.data[op.cf][op.key] = op.value
		}
	}
	return nil
}

func (b *mockBatch) Close() {
	b.closed = true
	b.ops = nil
}

// ---------------------------------------------------------------------------
// Iterator implementation
// ---------------------------------------------------------------------------

type mockEntry struct {
	key   []byte
	value []byte
}

type mockIterator struct {
	entries []mockEntry
	pos     int
}

func (it *mockIterator) Seek(target []byte) {
	it.pos = sort.Search(len(it.entries), func(i int) bool {
		return bytes.Compare(it.entries[i].key, target) >= 0
	})
}

func (it *mockIterator) SeekToFirst() { it.pos = 0 }

func (it *mockIterator) SeekToLast() {
	it.pos = len(it.entries) - 1
}

func (it *mockIterator) Next() { it.pos++ }
func (it *mockIterator) Prev() { it.pos-- }

func (it *mockIterator) Valid() bool {
	return it.pos >= 0 && it.pos < len(it.entries)
}

func (it *mockIterator) Key() []byte {
	if !it.Valid() {
		return nil
	}
	out := make([]byte, len(it.entries[it.pos].key))
	copy(out, it.entries[it.pos].key)
	return out
}

func (it *mockIterator) Value() []byte {
	if !it.Valid() {
		return nil
	}
	out := make([]byte, len(it.entries[it.pos].value))
	copy(out, it.entries[it.pos].value)
	return out
}

func (it *mockIterator) Err() error { return nil }
func (it *mockIterator) Close()     {}
