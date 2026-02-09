package config

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
)
