package main

import (
	"fmt"
	"log"

	"github.com/beyondbrewing/brewery-bitcoin/pkg/btcclient"
	"github.com/btcsuite/btcd/chaincfg"
)

func main() {
	btx, err := btcclient.NewBtcClient(
		btcclient.WithChainParams(&chaincfg.SigNetParams),
	)
	if err != nil {
		log.Fatalf("failed to create client: %v", err)
	}

	fmt.Printf("%+v\n", btx)
}
