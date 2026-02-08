package btcclient

import (
	"errors"
	"fmt"
)

// Sentinel errors for the btcclient package.
var (
	ErrClientAlreadyStarted = errors.New("btcclient: client already started")
	ErrClientNotStarted     = errors.New("btcclient: client not started")
	ErrPeerAlreadyExists    = errors.New("btcclient: peer already exists")
	ErrPeerNotFound         = errors.New("btcclient: peer not found")
	ErrMaxPeersReached      = errors.New("btcclient: max peer limit reached")
	ErrStopTimeout          = errors.New("btcclient: graceful stop timeout exceeded")
	ErrInvalidConfig        = errors.New("btcclient: invalid configuration")
)

// PeerConnectionError wraps connection-level failures with the peer address.
type PeerConnectionError struct {
	Address string
	Err     error
}

func (e *PeerConnectionError) Error() string {
	return fmt.Sprintf("btcclient: peer %s connection failed: %v", e.Address, e.Err)
}

func (e *PeerConnectionError) Unwrap() error {
	return e.Err
}
