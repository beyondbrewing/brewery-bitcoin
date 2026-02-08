package main

import (
	"fmt"

	"github.com/beyondbrewing/brewery-bitcoin/pkg/btcclient"
	"github.com/btcsuite/btcd/chaincfg"
)

func testnet(cfg *btcclient.Config) {
	cfg.ChainParams = &chaincfg.SigNetParams
}

func main() {

	btx := btcclient.NewBtcClient()
	fmt.Printf("%+v\n", btx)

}
