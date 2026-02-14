package btcclient

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/beyondbrewing/brewery-bitcoin/config"
	"github.com/beyondbrewing/brewery-bitcoin/pkg/logger"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire"
)

// PeerManager coordinates the lifecycle and health of all managed peers.
type PeerManager struct {
	chainParams *chaincfg.Params
	peers       map[string]*ManagedPeer
	peersMu     sync.RWMutex
	done        chan struct{}
	wg          sync.WaitGroup
	started     atomic.Bool
	logger      logger.Logger

	maxPeers            int
	maxFailureCount     int32
	healthCheckInterval time.Duration
	reconnectDelay      time.Duration
	connectionTimeout   time.Duration

	listeners *MessageListeners
}

// newPeerManager creates a PeerManager from a validated Config.
func newPeerManager(cfg *Config, l logger.Logger) *PeerManager {
	return &PeerManager{
		chainParams:         cfg.ChainParams,
		peers:               make(map[string]*ManagedPeer),
		done:                make(chan struct{}),
		logger:              l,
		maxPeers:            cfg.MaxPeers,
		maxFailureCount:     cfg.MaxFailureCount,
		healthCheckInterval: cfg.HealthCheckInterval,
		reconnectDelay:      cfg.ReconnectDelay,
		connectionTimeout:   cfg.ConnectionTimeout,
		listeners:           cfg.Listeners,
	}
}

// addPeer registers a new peer and starts its connection loop.
// Caller must hold peersMu write lock.
func (pm *PeerManager) addPeer(ctx context.Context, address string) error {
	if _, exists := pm.peers[address]; exists {
		return ErrPeerAlreadyExists
	}
	if len(pm.peers) >= pm.maxPeers {
		return ErrMaxPeersReached
	}

	peerCtx, cancel := context.WithCancel(ctx)
	mp := &ManagedPeer{
		address: address,
		status:  PeerStatusDisconnected,
		ctx:     peerCtx,
		cancel:  cancel,
	}

	pm.peers[address] = mp

	pm.wg.Add(1)
	go pm.connectionLoop(mp)

	return nil
}

// removePeer disconnects and removes a peer.
// Caller must hold peersMu write lock.
func (pm *PeerManager) removePeer(address string) error {
	mp, exists := pm.peers[address]
	if !exists {
		return ErrPeerNotFound
	}

	mp.cancel()
	if p := mp.Peer(); p != nil {
		p.Disconnect()
	}
	delete(pm.peers, address)

	if pm.listeners.OnPeerDisconnected != nil {
		pm.listeners.OnPeerDisconnected(mp)
	}

	pm.logger.Info("peer removed", "address", address)
	return nil
}

// connectionLoop continuously attempts to maintain a connection to a peer.
// It reconnects with a delay after disconnections until the peer context or
// manager done channel is closed.
func (pm *PeerManager) connectionLoop(mp *ManagedPeer) {
	defer pm.wg.Done()

	log := pm.logger.With("peer", mp.address)

	for {
		if pm.isStopping() || mp.ctx.Err() != nil {
			return
		}

		mp.setStatus(PeerStatusConnecting)

		err := pm.dial(mp)
		if err != nil {
			log.Warn("connection attempt failed", "error", err)
			mp.setStatus(PeerStatusDisconnected)
			mp.incrementFailure()

			if mp.FailureCount() >= pm.maxFailureCount {
				log.Warn("evicting peer: failure threshold exceeded", "failures", mp.FailureCount())
				pm.evictPeer(mp)
				return
			}

			if !pm.waitOrCancel(mp.ctx, pm.reconnectDelay) {
				return
			}
			continue
		}

		mp.setStatus(PeerStatusConnected)
		mp.resetFailures()
		log.Info("connected")

		if pm.listeners.OnPeerConnected != nil {
			pm.listeners.OnPeerConnected(mp)
		}

		// Ask this peer for other peers it knows about.
		if p := mp.Peer(); p != nil {
			p.QueueMessage(wire.NewMsgGetAddr(), nil)
		}

		// Block until the peer disconnects.
		mp.Peer().WaitForDisconnect()

		mp.setStatus(PeerStatusDisconnected)
		mp.clearPeer()
		log.Info("disconnected")

		if pm.listeners.OnPeerDisconnected != nil {
			pm.listeners.OnPeerDisconnected(mp)
		}

		// Brief pause before reconnecting.
		if !pm.waitOrCancel(mp.ctx, pm.reconnectDelay) {
			return
		}
	}
}

// dial establishes a TCP connection and associates it with a btcd peer.
func (pm *PeerManager) dial(mp *ManagedPeer) error {
	peerCfg := &peer.Config{
		UserAgentName:    config.APP_NAME,
		UserAgentVersion: config.APP_VERSION,
		ChainParams:      pm.chainParams,
		Services:         wire.SFNodeNetwork,
		TrickleInterval:  10 * time.Second,
		Listeners:        pm.peerListeners(mp),
	}

	dialer := net.Dialer{Timeout: pm.connectionTimeout}
	conn, err := dialer.DialContext(mp.ctx, "tcp", mp.address)
	if err != nil {
		return &PeerConnectionError{Address: mp.address, Err: err}
	}

	p, err := peer.NewOutboundPeer(peerCfg, mp.address)
	if err != nil {
		conn.Close()
		return &PeerConnectionError{Address: mp.address, Err: err}
	}

	p.AssociateConnection(conn)
	mp.setPeer(p)
	return nil
}

// healthCheckLoop periodically evaluates peer health until the manager stops.
func (pm *PeerManager) healthCheckLoop() {
	defer pm.wg.Done()

	ticker := time.NewTicker(pm.healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pm.done:
			return
		case <-ticker.C:
			pm.runHealthChecks()
		}
	}
}

// runHealthChecks iterates over all peers and removes those that exceeded
// the configured failure threshold.
func (pm *PeerManager) runHealthChecks() {
	pm.peersMu.Lock()
	defer pm.peersMu.Unlock()

	var toRemove []string
	for addr, mp := range pm.peers {
		fmt.Println(">>>>", addr, ">>>", mp.FailureCount())
		if mp.FailureCount() >= pm.maxFailureCount {
			toRemove = append(toRemove, addr)
		}
	}

	for _, addr := range toRemove {
		pm.logger.Warn("removing unhealthy peer",
			"peer", addr,
			"failures", pm.peers[addr].FailureCount(),
		)
		pm.removePeer(addr)
	}
}

// evictPeer acquires a write lock and removes the peer.
// Safe to call from the peer's own connectionLoop.
func (pm *PeerManager) evictPeer(mp *ManagedPeer) {
	pm.peersMu.Lock()
	defer pm.peersMu.Unlock()
	pm.removePeer(mp.address)
}

// isStopping returns true if the manager's done channel has been closed.
func (pm *PeerManager) isStopping() bool {
	select {
	case <-pm.done:
		return true
	default:
		return false
	}
}

// addDiscoveredPeers attempts to add peers received from an addr message.
// It silently skips duplicates and stops when MaxPeers is reached.
func (pm *PeerManager) addDiscoveredPeers(msg *wire.MsgAddr) {
	pm.peersMu.Lock()
	defer pm.peersMu.Unlock()

	added := 0
	for _, addr := range msg.AddrList {
		if len(pm.peers) >= pm.maxPeers {
			break
		}

		address := fmt.Sprintf("%s:%d", addr.IP.String(), addr.Port)

		if _, exists := pm.peers[address]; exists {
			continue
		}

		ctx, cancel := context.WithCancel(context.Background())
		mp := &ManagedPeer{
			address: address,
			status:  PeerStatusDisconnected,
			ctx:     ctx,
			cancel:  cancel,
		}

		pm.peers[address] = mp
		pm.wg.Add(1)
		go pm.connectionLoop(mp)
		added++
	}

	if added > 0 {
		pm.logger.Info("discovered new peers", "added", added, "total", len(pm.peers))
	}
}

// waitOrCancel sleeps for the given duration or returns false if the peer
// context or manager done channel fires first.
func (pm *PeerManager) waitOrCancel(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-t.C:
		return true
	case <-pm.done:
		return false
	case <-ctx.Done():
		return false
	}
}
