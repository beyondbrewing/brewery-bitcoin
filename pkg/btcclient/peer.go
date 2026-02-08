package btcclient

import (
	"context"
	"sync"

	"github.com/btcsuite/btcd/peer"
)

// PeerStatus represents the lifecycle state of a managed peer.
type PeerStatus int

const (
	PeerStatusDisconnected PeerStatus = iota
	PeerStatusConnecting
	PeerStatusConnected
	PeerStatusUnhealthy
)

// String returns a human-readable label for the peer status.
func (ps PeerStatus) String() string {
	switch ps {
	case PeerStatusDisconnected:
		return "disconnected"
	case PeerStatusConnecting:
		return "connecting"
	case PeerStatusConnected:
		return "connected"
	case PeerStatusUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// ManagedPeer wraps a btcd peer with lifecycle management and health tracking.
type ManagedPeer struct {
	address string
	peer    *peer.Peer
	status  PeerStatus

	failureCount int32

	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

// Address returns the peer's network address.
func (mp *ManagedPeer) Address() string {
	return mp.address
}

// Status returns the current peer status in a thread-safe manner.
func (mp *ManagedPeer) Status() PeerStatus {
	mp.mu.RLock()
	defer mp.mu.RUnlock()
	return mp.status
}

// FailureCount returns the current failure count in a thread-safe manner.
func (mp *ManagedPeer) FailureCount() int32 {
	mp.mu.RLock()
	defer mp.mu.RUnlock()
	return mp.failureCount
}

// Peer returns the underlying btcd peer. May be nil if disconnected.
func (mp *ManagedPeer) Peer() *peer.Peer {
	mp.mu.RLock()
	defer mp.mu.RUnlock()
	return mp.peer
}

// setStatus updates the peer status under a write lock.
func (mp *ManagedPeer) setStatus(s PeerStatus) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	mp.status = s
}

// incrementFailure atomically increments the failure counter and returns the new value.
func (mp *ManagedPeer) incrementFailure() int32 {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	mp.failureCount++
	return mp.failureCount
}

// resetFailures clears the failure counter and marks the connected timestamp.
func (mp *ManagedPeer) resetFailures() {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	mp.failureCount = 0
}

// clearPeer sets the underlying peer to nil under a write lock.
func (mp *ManagedPeer) clearPeer() {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	mp.peer = nil
}

// setPeer assigns the underlying btcd peer under a write lock.
func (mp *ManagedPeer) setPeer(p *peer.Peer) {
	mp.mu.Lock()
	defer mp.mu.Unlock()
	mp.peer = p
}
