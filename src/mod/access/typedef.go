package access

import (
	"sync"
	"time"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/geodb"
	"imuslab.com/zoraxy/mod/info/logger"
)

type Options struct {
	Logger       logger.Logger
	ConfigFolder string             //Path for storing config files
	GeoDB        *geodb.Store       //For resolving country code
	Database     *database.Database //System key-value database

	/* Public IP monitoring */
	PublicIpCheckInterval int64 //in Seconds
}

type AccessRule struct {
	ID                             string
	Name                           string
	Desc                           string
	BlacklistEnabled               bool
	WhitelistEnabled               bool
	WhitelistAllowLocalAndLoopback bool //Allow local and loopback address to bypass whitelist

	/* Whitelist Blacklist Table, value is comment if supported */
	WhiteListCountryCode *map[string]string
	WhiteListIP          *map[string]string
	BlackListContryCode  *map[string]string
	BlackListIP          *map[string]string

	parent *Controller
}

type Controller struct {
	ServerPublicIP    string
	DefaultAccessRule *AccessRule
	ProxyAccessRule   *sync.Map
	Options           *Options

	//Internal
	publicIpTicker     *time.Ticker
	publicIpTickerStop chan bool
}
