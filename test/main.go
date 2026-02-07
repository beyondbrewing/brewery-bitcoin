package main

import (
	"fmt"

	"github.com/beyondbrewing/brewery-bitcoin/pkg/btclient"
	"github.com/btcsuite/btcd/chaincfg"
)

func testnet(cfg *btclient.Config) {
	cfg.ChainParams = &chaincfg.SigNetParams
}

func main() {

	btx := btclient.NewBtcClient()
	fmt.Printf("%+v\n", btx)

}
