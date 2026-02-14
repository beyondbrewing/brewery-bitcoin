package indexer

import (
	"github.com/beyondbrewing/brewery-bitcoin/pkg/btcclient"
	"github.com/btcsuite/btcd/wire"
)

func (idx *Indexer) OnBlock(mp *btcclient.ManagedPeer, blk *wire.MsgBlock) {
}
