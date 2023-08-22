package dynamicproxy

import (
	"net"
	"net/http"
	"sync"

	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
	"imuslab.com/zoraxy/mod/dynamicproxy/redirection"
	"imuslab.com/zoraxy/mod/geodb"
	"imuslab.com/zoraxy/mod/statistic"
	"imuslab.com/zoraxy/mod/tlscert"
)

const (
	ProxyType_Subdomain = 0
	ProxyType_Vdir      = 1
)

type ProxyHandler struct {
	Parent *Router
}

type RouterOption struct {
	HostUUID           string //The UUID of Zoraxy, use for heading mod
	Port               int    //Incoming port
	UseTls             bool   //Use TLS to serve incoming requsts
	ForceTLSLatest     bool   //Force TLS1.2 or above
	ForceHttpsRedirect bool   //Force redirection of http to https endpoint
	TlsManager         *tlscert.Manager
	RedirectRuleTable  *redirection.RuleTable
	GeodbStore         *geodb.Store //GeoIP blacklist and whitelist
	StatisticCollector *statistic.Collector
}

type Router struct {
	Option            *RouterOption
	ProxyEndpoints    *sync.Map
	SubdomainEndpoint *sync.Map
	Running           bool
	Root              *ProxyEndpoint
	mux               http.Handler
	server            *http.Server
	tlsListener       net.Listener
	routingRules      []*RoutingRule

	tlsRedirectStop chan bool
}

// Auth credential for basic auth on certain endpoints
type BasicAuthCredentials struct {
	Username     string
	PasswordHash string
}

// Auth credential for basic auth on certain endpoints
type BasicAuthUnhashedCredentials struct {
	Username string
	Password string
}

// Paths to exclude in basic auth enabled proxy handler
type BasicAuthExceptionRule struct {
	PathPrefix string
}

// A proxy endpoint record
type ProxyEndpoint struct {
	ProxyType               int                       //The type of this proxy, see const def
	RootOrMatchingDomain    string                    //Root for vdir or Matching domain for subd, also act as key
	Domain                  string                    //Domain or IP to proxy to
	RequireTLS              bool                      //Target domain require TLS
	SkipCertValidations     bool                      //Set to true to accept self signed certs
	RequireBasicAuth        bool                      //Set to true to request basic auth before proxy
	BasicAuthCredentials    []*BasicAuthCredentials   `json:"-"` //Basic auth credentials
	BasicAuthExceptionRules []*BasicAuthExceptionRule //Path to exclude in a basic auth enabled proxy target
	Proxy                   *dpcore.ReverseProxy      `json:"-"`

	parent *Router
}

type RootOptions struct {
	ProxyLocation           string
	RequireTLS              bool
	SkipCertValidations     bool
	RequireBasicAuth        bool
	BasicAuthCredentials    []*BasicAuthCredentials
	BasicAuthExceptionRules []*BasicAuthExceptionRule
}

type VdirOptions struct {
	RootName                string
	Domain                  string
	RequireTLS              bool
	SkipCertValidations     bool
	RequireBasicAuth        bool
	BasicAuthCredentials    []*BasicAuthCredentials
	BasicAuthExceptionRules []*BasicAuthExceptionRule
}

type SubdOptions struct {
	MatchingDomain          string
	Domain                  string
	RequireTLS              bool
	SkipCertValidations     bool
	RequireBasicAuth        bool
	BasicAuthCredentials    []*BasicAuthCredentials
	BasicAuthExceptionRules []*BasicAuthExceptionRule
}
