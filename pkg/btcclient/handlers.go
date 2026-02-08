package btcclient

import (
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/wire"
)

// MessageListeners contains callbacks invoked when protocol messages are
// received from a peer. All fields are optional; nil listeners are skipped.
type MessageListeners struct {
	OnHeaders          func(*ManagedPeer, *wire.MsgHeaders)
	OnInv              func(*ManagedPeer, *wire.MsgInv)
	OnBlock            func(*ManagedPeer, *wire.MsgBlock)
	OnTx               func(*ManagedPeer, *wire.MsgTx)
	OnAddr             func(*ManagedPeer, *wire.MsgAddr)
	OnPeerConnected    func(*ManagedPeer)
	OnPeerDisconnected func(*ManagedPeer)
}

// peerListeners builds the btcd peer.MessageListeners that forward messages to
// the manager's registered MessageListeners for the given ManagedPeer.
func (pm *PeerManager) peerListeners(mp *ManagedPeer) peer.MessageListeners {
	return peer.MessageListeners{
		OnVersion: pm.handleVersion(mp),
		OnVerAck:  pm.handleVerAck(mp),
		OnHeaders: pm.handleHeaders(mp),
		OnInv:     pm.handleInv(mp),
		OnBlock:   pm.handleBlock(mp),
		OnTx:      pm.handleTx(mp),
		OnAddr:    pm.handleAddr(mp),
		OnReject:  pm.handleReject(mp),
	}
}

func (pm *PeerManager) handleVersion(mp *ManagedPeer) func(*peer.Peer, *wire.MsgVersion) *wire.MsgReject {
	return func(p *peer.Peer, msg *wire.MsgVersion) *wire.MsgReject {
		pm.logger.Debug("received version",
			"peer", mp.address,
			"version", msg.ProtocolVersion,
			"user_agent", msg.UserAgent,
		)

		return nil
	}
}

func (pm *PeerManager) handleVerAck(mp *ManagedPeer) func(*peer.Peer, *wire.MsgVerAck) {
	return func(p *peer.Peer, msg *wire.MsgVerAck) {
		pm.logger.Debug("received verack", "peer", mp.address)
	}
}

func (pm *PeerManager) handleHeaders(mp *ManagedPeer) func(*peer.Peer, *wire.MsgHeaders) {
	return func(p *peer.Peer, msg *wire.MsgHeaders) {
		if pm.listeners.OnHeaders != nil {
			pm.listeners.OnHeaders(mp, msg)
		}
	}
}

func (pm *PeerManager) handleInv(mp *ManagedPeer) func(*peer.Peer, *wire.MsgInv) {
	return func(p *peer.Peer, msg *wire.MsgInv) {
		if pm.listeners.OnInv != nil {
			pm.listeners.OnInv(mp, msg)
		}
	}
}

func (pm *PeerManager) handleBlock(mp *ManagedPeer) func(*peer.Peer, *wire.MsgBlock, []byte) {
	return func(p *peer.Peer, msg *wire.MsgBlock, buf []byte) {
		if pm.listeners.OnBlock != nil {
			pm.listeners.OnBlock(mp, msg)
		}
	}
}

func (pm *PeerManager) handleTx(mp *ManagedPeer) func(*peer.Peer, *wire.MsgTx) {
	return func(p *peer.Peer, msg *wire.MsgTx) {
		if pm.listeners.OnTx != nil {
			pm.listeners.OnTx(mp, msg)
		}
	}
}

func (pm *PeerManager) handleAddr(mp *ManagedPeer) func(*peer.Peer, *wire.MsgAddr) {
	return func(p *peer.Peer, msg *wire.MsgAddr) {
		if pm.listeners.OnAddr != nil {
			pm.listeners.OnAddr(mp, msg)
		}
		pm.addDiscoveredPeers(msg)
	}
}

func (pm *PeerManager) handleReject(mp *ManagedPeer) func(*peer.Peer, *wire.MsgReject) {
	return func(p *peer.Peer, msg *wire.MsgReject) {
		pm.logger.Warn("received reject",
			"peer", mp.address,
			"command", msg.Cmd,
			"code", msg.Code,
			"reason", msg.Reason,
		)
	}
}
