package indexer

import (
	"github.com/beyondbrewing/brewery-bitcoin/pkg/btcclient"
	"github.com/btcsuite/btcd/wire"
)

func (idx *Indexer) OnInv(mp *btcclient.ManagedPeer, msg *wire.MsgInv) {
}
