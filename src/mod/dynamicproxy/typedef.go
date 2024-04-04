package dynamicproxy

import (
	_ "embed"
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
	ProxyType_Root = 0
	ProxyType_Host = 1
	ProxyType_Vdir = 2
)

type ProxyHandler struct {
	Parent *Router
}

type RouterOption struct {
	HostUUID           string //The UUID of Zoraxy, use for heading mod
	HostVersion        string //The version of Zoraxy, use for heading mod
	Port               int    //Incoming port
	UseTls             bool   //Use TLS to serve incoming requsts
	ForceTLSLatest     bool   //Force TLS1.2 or above
	NoCache            bool   //Force set Cache-Control: no-store
	ListenOnPort80     bool   //Enable port 80 http listener
	ForceHttpsRedirect bool   //Force redirection of http to https endpoint
	TlsManager         *tlscert.Manager
	RedirectRuleTable  *redirection.RuleTable
	GeodbStore         *geodb.Store //GeoIP blacklist and whitelist
	StatisticCollector *statistic.Collector
	WebDirectory       string //The static web server directory containing the templates folder
}

type Router struct {
	Option         *RouterOption
	ProxyEndpoints *sync.Map
	Running        bool
	Root           *ProxyEndpoint
	mux            http.Handler
	server         *http.Server
	tlsListener    net.Listener
	routingRules   []*RoutingRule

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

// User defined headers to add into a proxy endpoint
type UserDefinedHeader struct {
	Key   string
	Value string
}

// A Virtual Directory endpoint, provide a subset of ProxyEndpoint for better
// program structure than directly using ProxyEndpoint
type VirtualDirectoryEndpoint struct {
	MatchingPath        string               //Matching prefix of the request path, also act as key
	Domain              string               //Domain or IP to proxy to
	RequireTLS          bool                 //Target domain require TLS
	SkipCertValidations bool                 //Set to true to accept self signed certs
	Disabled            bool                 //If the rule is enabled
	proxy               *dpcore.ReverseProxy `json:"-"`
	parent              *ProxyEndpoint       `json:"-"`
}

// A proxy endpoint record, a general interface for handling inbound routing
type ProxyEndpoint struct {
	ProxyType            int    //The type of this proxy, see const def
	RootOrMatchingDomain string //Matching domain for host, also act as key
	Domain               string //Domain or IP to proxy to

	//TLS/SSL Related
	RequireTLS               bool //Target domain require TLS
	BypassGlobalTLS          bool //Bypass global TLS setting options if TLS Listener enabled (parent.tlsListener != nil)
	SkipCertValidations      bool //Set to true to accept self signed certs
	SkipWebSocketOriginCheck bool //Skip origin check on websocket upgrade connections

	//Virtual Directories
	VirtualDirectories []*VirtualDirectoryEndpoint

	//Custom Headers
	UserDefinedHeaders []*UserDefinedHeader //Custom headers to append when proxying requests from this endpoint

	//Authentication
	RequireBasicAuth        bool                      //Set to true to request basic auth before proxy
	BasicAuthCredentials    []*BasicAuthCredentials   //Basic auth credentials
	BasicAuthExceptionRules []*BasicAuthExceptionRule //Path to exclude in a basic auth enabled proxy target

	//Fallback routing logic
	DefaultSiteOption int    //Fallback routing logic options
	DefaultSiteValue  string //Fallback routing target, optional

	Disabled bool //If the rule is disabled

	//Internal Logic Elements
	parent *Router
	proxy  *dpcore.ReverseProxy `json:"-"`
}

/*
	Routing type specific interface
	These are options that only avaible for a specific interface
	when running, these are converted into "ProxyEndpoint" objects
	for more generic routing logic
*/

// Root options are those that are required for reverse proxy handler to work
const (
	DefaultSite_InternalStaticWebServer = 0
	DefaultSite_ReverseProxy            = 1
	DefaultSite_Redirect                = 2
	DefaultSite_NotFoundPage            = 3
)

/*
Web Templates
*/
var (
	//go:embed templates/forbidden.html
	page_forbidden []byte
	//go:embed templates/hosterror.html
	page_hosterror []byte
)
