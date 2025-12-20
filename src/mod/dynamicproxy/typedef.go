package dynamicproxy

/*
	typdef.go

	This script handle the type definition for dynamic proxy and endpoints

	If you are looking for the default object initailization, please refer to default.go
*/
import (
	_ "embed"
	"net"
	"net/http"
	"sync"

	"imuslab.com/zoraxy/mod/auth/sso/oauth2"

	"imuslab.com/zoraxy/mod/access"
	"imuslab.com/zoraxy/mod/auth/sso/forward"
	"imuslab.com/zoraxy/mod/dynamicproxy/captcha"
	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
	"imuslab.com/zoraxy/mod/dynamicproxy/exploits"
	"imuslab.com/zoraxy/mod/dynamicproxy/loadbalance"
	"imuslab.com/zoraxy/mod/dynamicproxy/permissionpolicy"
	"imuslab.com/zoraxy/mod/dynamicproxy/redirection"
	"imuslab.com/zoraxy/mod/dynamicproxy/rewrite"
	"imuslab.com/zoraxy/mod/geodb"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/plugins"
	"imuslab.com/zoraxy/mod/statistic"
	"imuslab.com/zoraxy/mod/tlscert"
)

type ProxyType int

const PassiveLoadBalanceNotifyTimeout = 60 //Time to assume a passive load balance is unreachable, in seconds
const (
	ProxyTypeRoot ProxyType = iota //Root Proxy, everything not matching will be routed here
	ProxyTypeHost                  //Host Proxy, match by host (domain) name
	ProxyTypeVdir                  //Virtual Directory Proxy, match by path prefix
)

type ProxyHandler struct {
	Parent *Router
}

/* Router Object Options */
type RouterOption struct {
	/* Basic Settings */
	HostUUID           string //The UUID of Zoraxy, use for heading mod
	HostVersion        string //The version of Zoraxy, use for heading mod
	Port               int    //Incoming port
	UseTls             bool   //Use TLS to serve incoming requsts
	MinTLSVersion      uint16 //Minimum TLS version
	NoCache            bool   //Force set Cache-Control: no-store
	ListenOnPort80     bool   //Enable port 80 http listener
	ForceHttpsRedirect bool   //Force redirection of http to https endpoint

	/* Routing Service Managers */
	TlsManager         *tlscert.Manager          //TLS manager for serving SAN certificates
	RedirectRuleTable  *redirection.RuleTable    //Redirection rules handler and table
	GeodbStore         *geodb.Store              //GeoIP resolver
	AccessController   *access.Controller        //Blacklist / whitelist controller
	StatisticCollector *statistic.Collector      //Statistic collector for storing stats on incoming visitors
	WebDirectory       string                    //The static web server directory containing the templates folder
	LoadBalancer       *loadbalance.RouteManager //Load balancer that handle load balancing of proxy target
	PluginManager      *plugins.Manager          //Plugin manager for handling plugin routing

	/* Authentication Providers */
	ForwardAuthRouter *forward.AuthRouter
	OAuth2Router      *oauth2.OAuth2Router //OAuth2Router router for OAuth2Router authentication

	/* Utilities */
	DevelopmentMode bool           //Enable development mode, provide more debug information in headers
	Logger          *logger.Logger //Logger for reverse proxy requests
}

/* Router Object */
type Router struct {
	Option         *RouterOption
	ProxyEndpoints *sync.Map      //Map of ProxyEndpoint objects, each ProxyEndpoint object is a routing rule that handle incoming requests
	Running        bool           //If the router is running
	Root           *ProxyEndpoint //Root proxy endpoint, default site

	/* Internals */
	mux          http.Handler              //HTTP handler
	server       *http.Server              //HTTP server
	loadBalancer *loadbalance.RouteManager //Load balancer routing manager
	routingRules []*RoutingRule            //Special routing rules, handle high priority routing like ACME request handling

	tlsListener      net.Listener //TLS listener, handle SNI routing
	tlsBehaviorMutex sync.RWMutex //Mutex for tlsBehavior map
	tlsRedirectStop  chan bool    //Stop channel for tls redirection server

	rateLimterStop   chan bool              //Stop channel for rate limiter
	rateLimitCounter RequestCountPerIpTable //Request counter for rate limter

	captchaSessionStore *captcha.SessionStore //CAPTCHA session store for tracking verified sessions

	// Secondary listening ports and their servers
	secondaryServers     map[string]*http.Server //Map of secondary listening servers, key is the listening address (ip:port or :port)
	secondaryStopChans   map[string]chan bool    //Stop channels for secondary listening servers
	secondaryServerMutex sync.RWMutex            //Mutex for accessing secondary server maps

}

/* Basic Auth Related Data structure*/
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

type AuthExceptionType int

const (
	AuthExceptionType_Paths AuthExceptionType = iota //Path exception, match by path prefix
	AuthExceptionType_CIDR                           //CIDR exception, match by CIDR
)

// Paths to exclude in basic auth enabled proxy handler
type BasicAuthExceptionRule struct {
	RuleType   AuthExceptionType //The type of the exception rule
	PathPrefix string            //Path prefix to match, e.g. /api/v1/
	CIDR       string            //CIDR to match, e.g. 192.168.1.0/24 or IP address, e.g. 192.168.1.1
}

/* Routing Rule Data Structures */

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

// Rules and settings for header rewriting
type HeaderRewriteRules struct {
	UserDefinedHeaders            []*rewrite.UserDefinedHeader        //Custom headers to append when proxying requests from this endpoint
	RequestHostOverwrite          string                              //If not empty, this domain will be used to overwrite the Host field in request header
	HSTSMaxAge                    int64                               //HSTS max age, set to 0 for disable HSTS headers
	EnablePermissionPolicyHeader  bool                                //Enable injection of permission policy header
	PermissionPolicy              *permissionpolicy.PermissionsPolicy //Permission policy header
	DisableHopByHopHeaderRemoval  bool                                //Do not remove hop-by-hop headers
	DisableUserAgentHeaderRemoval bool                                //Do not remove User-Agent header from server response

}

/*

	Authentication Provider

	TODO: Move these into a dedicated module
*/

type AuthMethod int

const (
	AuthMethodNone    AuthMethod = iota //No authentication required
	AuthMethodBasic                     //Basic Auth
	AuthMethodForward                   //Forward
	AuthMethodOauth2                    //Oauth2
)

type AuthenticationProvider struct {
	AuthMethod AuthMethod //The authentication method to use
	/* Basic Auth Settings */
	BasicAuthCredentials    []*BasicAuthCredentials   //Basic auth credentials
	BasicAuthExceptionRules []*BasicAuthExceptionRule //Path to exclude in a basic auth enabled proxy target
	BasicAuthGroupIDs       []string                  //Group IDs that are allowed to access this endpoint

	/* Forward Auth Settings */
	ForwardAuthURL                    string   // Full URL of the Forward Auth endpoint. Example: https://auth.example.com/api/authz/forward-auth
	ForwardAuthResponseHeaders        []string // List of headers to copy from the forward auth server response to the request.
	ForwardAuthResponseClientHeaders  []string // List of headers to copy from the forward auth server response to the client response.
	ForwardAuthRequestHeaders         []string // List of headers to copy from the original request to the auth server. If empty all are copied.
	ForwardAuthRequestExcludedCookies []string // List of cookies to exclude from the request after sending it to the forward auth server.
}

/* CAPTCHA Provider Configuration */
// Type aliases for backward compatibility
type CaptchaProvider = captcha.Provider
type CaptchaConfig = captcha.Config
type CaptchaExceptionType = captcha.ExceptionType
type CaptchaExceptionRule = captcha.ExceptionRule

const (
	CaptchaProviderCloudflare = captcha.ProviderCloudflare
	CaptchaProviderGoogle     = captcha.ProviderGoogle

	CaptchaExceptionType_Paths = captcha.ExceptionTypePaths
	CaptchaExceptionType_CIDR  = captcha.ExceptionTypeCIDR
)

// A proxy endpoint record, a general interface for handling inbound routing
type ProxyEndpoint struct {
	ProxyType            ProxyType               //The type of this proxy, see const def
	RootOrMatchingDomain string                  //Matching domain for host, also act as key
	MatchingDomainAlias  []string                //A list of domains that alias to this rule
	ActiveOrigins        []*loadbalance.Upstream //Activated Upstream or origin servers IP or domain to proxy to
	InactiveOrigins      []*loadbalance.Upstream //Disabled Upstream or origin servers IP or domain to proxy to
	UseStickySession     bool                    //Use stick session for load balancing
	UseActiveLoadBalance bool                    //Use active loadbalancing, default passive
	Disabled             bool                    //If the rule is disabled
	ListeningPorts       []string                //Alternative listening ports in format "ip:port" or ":port" (e.g., ":8080", "192.168.1.1:8080")

	//Inbound TLS/SSL Related
	BypassGlobalTLS bool                             //Bypass global TLS setting options if TLS Listener enabled (parent.tlsListener != nil)
	TlsOptions      *tlscert.HostSpecificTlsBehavior //TLS options for this endpoint, if nil, use global TLS options

	//Virtual Directories
	VirtualDirectories []*VirtualDirectoryEndpoint

	//Custom Headers
	HeaderRewriteRules           *HeaderRewriteRules
	EnableWebsocketCustomHeaders bool //Enable custom headers for websocket connections as well (default only http reqiests)

	//Authentication
	AuthenticationProvider *AuthenticationProvider

	// Rate Limiting
	RequireRateLimit bool
	RateLimit        int64 // Rate limit in requests per second

	// CAPTCHA Gating
	RequireCaptcha bool           // Enable CAPTCHA gating for this endpoint
	CaptchaConfig  *CaptchaConfig // CAPTCHA provider configuration

	//Uptime Monitor
	DisableUptimeMonitor       bool //Disable uptime monitor for this endpoint
	DisableAutoFallback        bool //Disable automatic fallback when uptime monitor detects an upstream is down (continue monitoring but don't auto-disable upstream)
	DisableLogging             bool //Disable logging of reverse proxy requests
	DisableStatisticCollection bool //Disable statistic collection for this endpoint

	//Exploit Detection
	BlockCommonExploits bool //Enable blocking of common exploits (SQLi, XSS, etc.)
	BlockAICrawlers     bool //Enable blocking of AI crawlers and bots
	MitigationAction    int  //Action to take when exploit/crawler detected (0=404, 1=403, 2=400, 3=Drop, 4=Delay, 5=Captcha)

	// Chunked Transfer Encoding
	DisableChunkedTransferEncoding bool //Disable chunked transfer encoding for this endpoint

	//Access Control
	AccessFilterUUID string //Access filter ID

	//Fallback routing logic (Special Rule Sets Only)
	DefaultSiteOption int    //Fallback routing logic options
	DefaultSiteValue  string //Fallback routing target, optional

	//Internal Logic Elements
	parent   *Router            `json:"-"` //Parent router, excluded from JSON
	detector *exploits.Detector `json:"-"` //Exploit detector instance, excluded from JSON
	Tags     []string           // Tags for the proxy endpoint
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
	DefaultSite_NoResponse              = 4

	DefaultSite_TeaPot = 418 //I'm a teapot
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
