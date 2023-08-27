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
	Option             *RouterOption
	ProxyEndpoints     *sync.Map
	SubdomainEndpoint  *sync.Map
	Running            bool
	Root               *ProxyEndpoint
	RootRoutingOptions *RootRoutingOptions
	mux                http.Handler
	server             *http.Server
	tlsListener        net.Listener
	routingRules       []*RoutingRule

	tlsRedirectStop chan bool      //Stop channel for tls redirection server
	tldMap          map[string]int //Top level domain map, see tld.json
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
	BypassGlobalTLS         bool                      //Bypass global TLS setting options if TLS Listener enabled (parent.tlsListener != nil)
	SkipCertValidations     bool                      //Set to true to accept self signed certs
	RequireBasicAuth        bool                      //Set to true to request basic auth before proxy
	BasicAuthCredentials    []*BasicAuthCredentials   `json:"-"` //Basic auth credentials
	BasicAuthExceptionRules []*BasicAuthExceptionRule //Path to exclude in a basic auth enabled proxy target
	Proxy                   *dpcore.ReverseProxy      `json:"-"`

	parent *Router
}

// Root options are those that are required for reverse proxy handler to work
type RootOptions struct {
	ProxyLocation       string //Proxy Root target, all unset traffic will be forward to here
	RequireTLS          bool   //Proxy root target require TLS connection (not recommended)
	BypassGlobalTLS     bool   //Bypass global TLS setting and make root http only (not recommended)
	SkipCertValidations bool   //Skip cert validation, suitable for self-signed certs, CURRENTLY NOT USED

	//Basic Auth Related
	RequireBasicAuth        bool //Require basic auth, CURRENTLY NOT USED
	BasicAuthCredentials    []*BasicAuthCredentials
	BasicAuthExceptionRules []*BasicAuthExceptionRule
}

// Additional options are here for letting router knows how to route exception cases for root
type RootRoutingOptions struct {
	//Root only configs
	EnableRedirectForUnsetRules bool   //Force unset rules to redirect to custom domain
	UnsetRuleRedirectTarget     string //Custom domain to redirect to for unset rules
}

type VdirOptions struct {
	RootName                string
	Domain                  string
	RequireTLS              bool
	BypassGlobalTLS         bool
	SkipCertValidations     bool
	RequireBasicAuth        bool
	BasicAuthCredentials    []*BasicAuthCredentials
	BasicAuthExceptionRules []*BasicAuthExceptionRule
}

type SubdOptions struct {
	MatchingDomain          string
	Domain                  string
	RequireTLS              bool
	BypassGlobalTLS         bool
	SkipCertValidations     bool
	RequireBasicAuth        bool
	BasicAuthCredentials    []*BasicAuthCredentials
	BasicAuthExceptionRules []*BasicAuthExceptionRule
}
