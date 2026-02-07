package btclient

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/beyondbrewing/brewery-bitcoin/config"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire"
)

type PeerStatus int

// status of peers
const (
	PeerStatusDisconnected PeerStatus = iota
	PeerStatusConnected
	PeerStatusHealthy
	PeerStatusUnhealthy
)

// for metircs
func (ps PeerStatus) String() string {
	switch ps {
	case PeerStatusConnected:
		return "connected"
	case PeerStatusDisconnected:
		return "disconnected"
	case PeerStatusHealthy:
		return "healthy"
	case PeerStatusUnhealthy:
		return "unhealthy"
	}

	return "unknown"
}

// struct to manage peers
type ManagedPeer struct {
	address      string
	peer         *peer.Peer
	status       PeerStatus
	lastcheck    time.Time
	failureCount int32
	mu           sync.RWMutex
	ctx          context.Context
	cancel       context.CancelFunc

	// conditions for checking health
	initialPeerJoining time.Time // conditions to be added to replace a peer if needed
	latency            int
}

// manager that manages all peers
type PeerManager struct {
	chainParams *chaincfg.Params
	peers       map[string]*ManagedPeer
	peersMu     sync.RWMutex
	done        chan struct{}
	wg          sync.WaitGroup
	started     atomic.Bool

	// configurations
	maxPeers            int
	maxFailureCount     int32
	healthCheckInterval time.Duration
	reconnectDelay      time.Duration
	requestTimeout      time.Duration

	//function Listeners
	listeners *MessageListeners
}

type MessageListeners struct {
	// OnEvent func(*ManagedPeer) // template
	OnHeaders          func(*ManagedPeer, *wire.MsgHeaders)
	OnInv              func(*ManagedPeer, *wire.MsgInv)
	OnBlock            func(*ManagedPeer, *wire.MsgBlock)
	OnTx               func(*ManagedPeer, *wire.MsgTx)
	OnPeerConnected    func(*ManagedPeer)
	OnPeerDisconnected func(*ManagedPeer)
}

type BtcClient struct {
	manager *PeerManager
	mu      sync.RWMutex
}

type ConfigFunc func(*Config)

type Config struct {
	ChainParams     *chaincfg.Params
	MaxPeers        int
	MaxFailureCount int32

	HealthCheckInterval time.Duration
	ReconnectDelay      time.Duration
	RequestTimeout      time.Duration
	Listener            *MessageListeners
}

// defaults to mainnet
func DefaultConfig() *Config {
	return &Config{
		ChainParams:         &chaincfg.MainNetParams,
		MaxPeers:            30,
		MaxFailureCount:     10,
		HealthCheckInterval: 30 * time.Second,
		ReconnectDelay:      5 * time.Second,
		RequestTimeout:      30 * time.Second,
		Listener:            &MessageListeners{},
	}
}

func NewBtcClient(cfgfunc ...ConfigFunc) *BtcClient {

	config := DefaultConfig()
	for _, cfg := range cfgfunc {
		cfg(config)
	}

	manager := &PeerManager{
		chainParams:         config.ChainParams,
		peers:               make(map[string]*ManagedPeer),
		maxPeers:            config.MaxPeers,
		maxFailureCount:     config.MaxFailureCount,
		healthCheckInterval: config.HealthCheckInterval,
		reconnectDelay:      config.ReconnectDelay,
		requestTimeout:      config.RequestTimeout,
		done:                make(chan struct{}),
		listeners:           config.Listener,
	}
	return &BtcClient{
		manager: manager,
	}
}

func (bc *BtcClient) AddPeers(ctx context.Context, address string) error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	manager := bc.manager
	manager.peersMu.Lock()
	defer manager.peersMu.Unlock()

	if _, exists := manager.peers[address]; exists {
		return fmt.Errorf("peer %s already exists", address)
	}

	if len(manager.peers) >= manager.maxPeers {
		// need to change to add new peer with up to date values
		return fmt.Errorf("max peer limit reached")
	}

	peerCtx, cancel := context.WithCancel(context.Background())
	managedPeer := &ManagedPeer{
		address: address,
		status:  PeerStatusDisconnected,
		ctx:     peerCtx,
		cancel:  cancel,
	}

	manager.peers[address] = managedPeer

	manager.wg.Add(1)
	go manager.connectPeer(managedPeer)
	return nil
}

func (bc *BtcClient) RemovePeers(address string) error {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	manager := bc.manager
	manager.peersMu.Lock()
	defer manager.peersMu.Unlock()

	managedPeer, exists := manager.peers[address]
	if !exists {
		return fmt.Errorf("Peer %s not found", address)
	}

	managedPeer.cancel()
	if managedPeer.peer != nil {
		managedPeer.peer.Disconnect()
	}

	delete(manager.peers, address)

	if manager.listeners.OnPeerDisconnected != nil {
		manager.listeners.OnPeerDisconnected(managedPeer)
	}

	return nil

}

func (pm *PeerManager) connectPeer(managedPeer *ManagedPeer) {
	defer pm.wg.Done()

	for {
		select {
		case <-pm.done:
			return
		case <-managedPeer.ctx.Done():
			return
		default:
		}

		err := pm.establishConnection(managedPeer)
		if err != nil {
			log.Printf("[PEER] %s connection failed :%v", managedPeer.address, err)

			managedPeer.mu.Lock()
			managedPeer.status = PeerStatusDisconnected
			managedPeer.mu.Unlock()

			select {
			case <-time.After(pm.reconnectDelay): // wait here till unlock
			case <-pm.done:
				return
			case <-managedPeer.ctx.Done():
				return
			}
		}
		managedPeer.mu.Lock()
		managedPeer.status = PeerStatusConnected
		managedPeer.failureCount = 0
		managedPeer.mu.Unlock()

		if pm.listeners.OnPeerConnected != nil {
			pm.listeners.OnPeerConnected(managedPeer)
		}

		managedPeer.peer.WaitForDisconnect()

		managedPeer.mu.Lock()
		managedPeer.status = PeerStatusDisconnected
		managedPeer.peer = nil
		managedPeer.mu.Unlock()

		if pm.listeners.OnPeerDisconnected != nil {
			pm.listeners.OnPeerDisconnected(managedPeer)
		}

	}
}

func (pm *PeerManager) establishConnection(managedPeer *ManagedPeer) error {
	peerCfg := &peer.Config{
		UserAgentName:    config.APP_NAME,
		UserAgentVersion: config.APP_VERSION,
		ChainParams:      pm.chainParams,
		Services:         wire.SFNodeNetwork,
		TrickleInterval:  time.Second * 10,
		Listeners: peer.MessageListeners{
			OnHeaders: pm.onHeaders(managedPeer),
			OnVerAck:  pm.onVerAck(managedPeer),
			OnVersion: pm.onVersion(managedPeer),
			OnInv:     pm.onInv(managedPeer),
			OnBlock:   pm.onBlock(managedPeer),
			OnTx:      pm.onTx(managedPeer),
			OnReject:  pm.onReject(managedPeer),
		},
	}

	conn, err := net.Dial("tcp", managedPeer.address)
	if err != nil {
		conn.Close()
		return fmt.Errorf("tcp connection failed  :%w  ", err)
	}

	p, err := peer.NewOutboundPeer(peerCfg, managedPeer.address)
	if err != nil {
		return fmt.Errorf("peer creation failed : %w", err)
	}

	p.AssociateConnection(conn)
	managedPeer.peer = p

	return nil
}

// message handlers (contents inside to be edited later )

func (pm *PeerManager) onVersion(mp *ManagedPeer) func(*peer.Peer, *wire.MsgVersion) *wire.MsgReject {
	return func(p *peer.Peer, mv *wire.MsgVersion) *wire.MsgReject {
		return nil
	}
}

func (pm *PeerManager) onVerAck(mp *ManagedPeer) func(*peer.Peer, *wire.MsgVerAck) {
	return func(p *peer.Peer, mva *wire.MsgVerAck) {}
}

func (pm *PeerManager) onHeaders(mp *ManagedPeer) func(*peer.Peer, *wire.MsgHeaders) {
	return func(p *peer.Peer, mh *wire.MsgHeaders) {}
}

func (pm *PeerManager) onInv(mp *ManagedPeer) func(*peer.Peer, *wire.MsgInv) {
	return func(p *peer.Peer, mi *wire.MsgInv) {}
}

func (pm *PeerManager) onBlock(mp *ManagedPeer) func(*peer.Peer, *wire.MsgBlock, []byte) {
	return func(p *peer.Peer, mb *wire.MsgBlock, b []byte) {}
}

func (pm *PeerManager) onTx(mp *ManagedPeer) func(*peer.Peer, *wire.MsgTx) {
	return func(p *peer.Peer, mt *wire.MsgTx) {}
}

func (pm *PeerManager) onReject(mp *ManagedPeer) func(*peer.Peer, *wire.MsgReject) {
	return func(p *peer.Peer, mr *wire.MsgReject) {}
}

// end message handlers

func (pm *PeerManager) HealthCheckLoop() {
	defer pm.wg.Done()

	ticker := time.NewTicker(pm.healthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pm.done:
			return
		case <-ticker.C:
			pm.performHealthChecks()

		}
	}
}

func (pm *PeerManager) performHealthChecks() {
	pm.peersMu.Lock()
	defer pm.peersMu.Unlock()

	for address, managedPeer := range pm.peers {
		managedPeer.mu.RLock()
		status := managedPeer.status
		failures := managedPeer.failureCount
		managedPeer.mu.RUnlock()

		if failures >= pm.maxFailureCount {
			log.Printf("[PEER %s] marking as unhealthy after %d failures", address, failures)
			managedPeer.mu.Lock()
			managedPeer.status = PeerStatusUnhealthy
			managedPeer.mu.Unlock()
		}

		if status == PeerStatusDisconnected {
			log.Printf("[PEER %s] attempting reconnection", address)
		}

		managedPeer.mu.Lock()
		managedPeer.lastcheck = time.Now()
		managedPeer.mu.Unlock()
	}
}
