package loadbalance

import (
	"strings"
	"sync"
	"time"

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
	SessionStore *sessions.CookieStore
	OnlineStatus sync.Map //Store the online status notify by uptime monitor
	Options      Options  //Options for the load balancer

	cacheTicker     *time.Ticker //Ticker for cache cleanup
	cacheTickerStop chan bool    //Stop the cache cleanup
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

	//Create a ticker for cache cleanup every 12 hours
	cacheTicker := time.NewTicker(12 * time.Hour)
	cacheTickerStop := make(chan bool)
	go func() {
		options.Logger.PrintAndLog("LoadBalancer", "Upstream state cache ticker started", nil)
		for {
			select {
			case <-cacheTickerStop:
				return
			case <-cacheTicker.C:
				//Clean up the cache
				options.Logger.PrintAndLog("LoadBalancer", "Cleaning up upstream state cache", nil)
			}
		}
	}()

	//Generate a session store for stickySession
	store := sessions.NewCookieStore([]byte(options.SystemUUID))
	return &RouteManager{
		SessionStore: store,
		OnlineStatus: sync.Map{},
		Options:      *options,

		cacheTicker:     cacheTicker,
		cacheTickerStop: cacheTickerStop,
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
	if len(targets) == 0 {
		//No upstream
		return "(no upstream config)"
	}
	return strings.Join(targets, ", ")
}

// Reset the current session store and clear all previous sessions
func (m *RouteManager) ResetSessions() {
	m.SessionStore = sessions.NewCookieStore([]byte(m.Options.SystemUUID))
}

func (m *RouteManager) Close() {
	//Close the session store
	m.SessionStore.MaxAge(0)

	//Stop the cache cleanup
	if m.cacheTicker != nil {
		m.cacheTicker.Stop()
	}
	close(m.cacheTickerStop)
}

// Log Println, replace all log.Println or fmt.Println with this
func (m *RouteManager) println(message string, err error) {
	m.Options.Logger.PrintAndLog("LoadBalancer", message, err)
}
