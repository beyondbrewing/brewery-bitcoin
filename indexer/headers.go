package indexer

import (
	"github.com/beyondbrewing/brewery-bitcoin/pkg/btcclient"
	"github.com/btcsuite/btcd/wire"
)

func (idx *Indexer) OnHeaders(mp *btcclient.ManagedPeer, msg *wire.MsgHeaders) {
	if len(msg.Headers) == 0 {
		return
	}
	// add a check if this range is already indexed if so dont skip over it
	idx.mu.Lock()
	startHeight := len(idx.headers) // fetch from rocksdb
	for _, hdr := range msg.Headers {
		idx.headers = append(idx.headers, *hdr)
		h := hdr.BlockHash()
		idx.tipHash = &h
	}

	newHeight := len(idx.headers)
	tip := idx.tipHash
	idx.mu.Unlock()

	idx.logger.Info("Received Headers",
		"from", startHeight,
		"to", newHeight,
		"batch", len(msg.Headers),
		"tip", tip.String(),
	)

	if len(msg.Headers) == 2000 {
		// requestHeaders
	} else {
		idx.mu.Lock()
		idx.synced = true
		idx.mu.Unlock()
	}

}
