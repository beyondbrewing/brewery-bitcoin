package indexer

import (
	"github.com/beyondbrewing/brewery-bitcoin/pkg/btcclient"
	"github.com/btcsuite/btcd/wire"
)

func (idx *Indexer) OnTx(mp *btcclient.ManagedPeer, tx *wire.MsgTx) {

}
