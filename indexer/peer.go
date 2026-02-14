package indexer

import "github.com/beyondbrewing/brewery-bitcoin/pkg/btcclient"

func (idx *Indexer) OnPeerConnected(mp *btcclient.ManagedPeer) {}

func (idx *Indexer) OnPeerDisconnected(mp *btcclient.ManagedPeer) {}
