package main

import (
	"fmt"

	"github.com/beyondbrewing/brewery-bitcoin/pkg/btcclient"
	"github.com/beyondbrewing/brewery-bitcoin/pkg/logger"
	"github.com/btcsuite/btcd/chaincfg"
)

func main() {
	logger.SetDefault(logger.MustDevelopment())
	defer logger.SyncDefault()

	btx, err := btcclient.NewBtcClient(
		btcclient.WithChainParams(&chaincfg.SigNetParams),
	)
	if err != nil {
		logger.Fatal("failed to create client", "error", err)
	}

	fmt.Printf("%+v\n", btx)
}
