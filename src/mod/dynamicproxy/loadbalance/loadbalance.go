package loadbalance

import (
	"imuslab.com/zoraxy/mod/geodb"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/uptime"
)

/*
	Load Balancer

	Handleing load balance request for upstream destinations
*/

type BalancePolicy int

const (
	BalancePolicy_RoundRobin BalancePolicy = 0 //Round robin, will ignore upstream if down
	BalancePolicy_Fallback   BalancePolicy = 1 //Fallback only. Will only switch to next node if the first one failed
	BalancePolicy_Random     BalancePolicy = 2 //Random, randomly pick one from the list that is online
	BalancePolicy_GeoRegion  BalancePolicy = 3 //Use the one defined for this geo-location, when down, pick the next avaible node
)

type LoadBalanceRule struct {
	Upstreams         []string      //Reverse proxy upstream servers
	LoadBalancePolicy BalancePolicy //Policy in deciding which target IP to proxy
	UseRegionLock     bool          //If this is enabled with BalancePolicy_Geo, when the main site failed, it will not pick another node
	UseStickySession  bool          //Use sticky session, if you are serving EU countries, make sure to add the "Do you want cookie" warning

	parent *RouteManager
}

type Options struct {
	Geodb         *geodb.Store    //GeoIP resolver for checking incoming request origin country
	UptimeMonitor *uptime.Monitor //For checking if the target is online, this might be nil when the module starts
}

type RouteManager struct {
	Options Options
	Logger  *logger.Logger
}

// Create a new load balance route manager
func NewRouteManager(options *Options, logger *logger.Logger) *RouteManager {
	newManager := RouteManager{
		Options: *options,
		Logger:  logger,
	}
	logger.PrintAndLog("INFO", "Load Balance Route Manager started", nil)
	return &newManager
}

func (b *LoadBalanceRule) GetProxyTargetIP() {
//TODO: Implement get proxy target IP logic here
}

// Print debug message
func (m *RouteManager) debugPrint(message string, err error) {
	m.Logger.PrintAndLog("LB", message, err)
}
