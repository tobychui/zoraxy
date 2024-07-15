package loadbalance

import (
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/sessions"
	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
	"imuslab.com/zoraxy/mod/geodb"
	"imuslab.com/zoraxy/mod/info/logger"
)

/*
	Load Balancer

	Handleing load balance request for upstream destinations
*/

type Options struct {
	SystemUUID           string       //Use for the session store
	UseActiveHealthCheck bool         //Use active health check, default to false
	Geodb                *geodb.Store //GeoIP resolver for checking incoming request origin country
	Logger               *logger.Logger
}

type RouteManager struct {
	SessionStore           *sessions.CookieStore
	LoadBalanceMap         sync.Map  //Sync map to store the last load balance state of a given node
	OnlineStatusMap        sync.Map  //Sync map to store the online status of a given ip address or domain name
	onlineStatusTickerStop chan bool //Stopping channel for the online status pinger
	Options                Options   //Options for the load balancer
}

/* Upstream or Origin Server */
type Upstream struct {
	//Upstream Proxy Configs
	OriginIpOrDomain         string //Target IP address or domain name with port
	RequireTLS               bool   //Require TLS connection
	SkipCertValidations      bool   //Set to true to accept self signed certs
	SkipWebSocketOriginCheck bool   //Skip origin check on websocket upgrade connections

	//Load balancing configs
	Weight  int //Random weight for round robin, 0 for fallback only
	MaxConn int //TODO: Maxmium connection to this server, 0 for unlimited

	//currentConnectionCounts atomic.Uint64 //Counter for number of client currently connected
	proxy *dpcore.ReverseProxy
}

// Create a new load balancer
func NewLoadBalancer(options *Options) *RouteManager {
	if options.SystemUUID == "" {
		//System UUID not passed in. Use random key
		options.SystemUUID = uuid.New().String()
	}

	//Generate a session store for stickySession
	store := sessions.NewCookieStore([]byte(options.SystemUUID))
	return &RouteManager{
		SessionStore:           store,
		LoadBalanceMap:         sync.Map{},
		OnlineStatusMap:        sync.Map{},
		onlineStatusTickerStop: nil,
		Options:                *options,
	}
}

// UpstreamsReady checks if the group of upstreams contains at least one
// origin server that is ready
func (m *RouteManager) UpstreamsReady(upstreams []*Upstream) bool {
	for _, upstream := range upstreams {
		if upstream.IsReady() {
			return true
		}
	}
	return false
}

// String format and convert a list of upstream into a string representations
func GetUpstreamsAsString(upstreams []*Upstream) string {
	targets := []string{}
	for _, upstream := range upstreams {
		targets = append(targets, upstream.String())
	}
	return strings.Join(targets, ", ")
}

func (m *RouteManager) Close() {
	if m.onlineStatusTickerStop != nil {
		m.onlineStatusTickerStop <- true
	}

}

// Print debug message
func (m *RouteManager) debugPrint(message string, err error) {
	m.Options.Logger.PrintAndLog("LoadBalancer", message, err)
}
