package indexer

import (
	"github.com/beyondbrewing/brewery-bitcoin/pkg/btcclient"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

func RequestHeaders(mp *btcclient.ManagedPeer, fromHash *chainhash.Hash) {
	p := mp.Peer()
	if p == nil {
		return
	}

	msg := wire.NewMsgGetHeaders()
	msg.AddBlockLocatorHash(fromHash)
	msg.HashStop = chainhash.Hash{}
	p.QueueMessage(msg, nil)
}

func RequestBlock(mp *btcclient.ManagedPeer, blockHash *chainhash.Hash) {
	p := mp.Peer()
	if p == nil {
		return
	}

	msg := wire.NewMsgGetData()
	iv := wire.NewInvVect(wire.InvTypeWitnessBlock, blockHash)
	msg.AddInvVect(iv)
	p.QueueMessage(msg, nil)
}
