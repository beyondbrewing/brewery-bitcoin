package config

import (
	"github.com/beyondbrewing/brewery-bitcoin/utils"
	"github.com/spf13/viper"
)

// injected configurations
var (
	APP_NAME    string = "brewery-bitcoin"
	APP_VERSION string = "0.0.1"
)

// value changed by paramaters from config
var (
	BREWERY_CHAINPARAM string = "mainnet"
	BREWERY_MAXPEERS   uint8  = 10 // intentionally limiting to max 255
	BREWERY_ENODE      []string
	BREWERY_DATA_DIR   string
)

func LoadConfig() {
	utils.ImportEnv()
	BREWERY_DATA_DIR = viper.GetString("DATA_DIR")
}
