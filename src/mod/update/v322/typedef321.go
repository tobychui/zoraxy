package v322

import (
	"net/http"
	"sync"

	"imuslab.com/zoraxy/mod/dynamicproxy/loadbalance"
	"imuslab.com/zoraxy/mod/dynamicproxy/permissionpolicy"
	"imuslab.com/zoraxy/mod/dynamicproxy/rewrite"
)

type ProxyType int

// Pull from ratelimit.go
type RequestCountPerIpTable struct {
	table sync.Map
}

// Pull from special.go
type RoutingRule struct {
	ID                     string //ID of the routing rule
	Enabled                bool   //If the routing rule enabled
	UseSystemAccessControl bool   //Pass access control check to system white/black list, set this to false to bypass white/black list
	MatchRule              func(r *http.Request) bool
	RoutingHandler         func(http.ResponseWriter, *http.Request)
}

const (
	ProxyTypeRoot ProxyType = iota //Root Proxy, everything not matching will be routed here
	ProxyTypeHost                  //Host Proxy, match by host (domain) name
	ProxyTypeVdir                  //Virtual Directory Proxy, match by path prefix
)

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

// Paths to exclude in basic auth enabled proxy handler
type BasicAuthExceptionRule struct {
	PathPrefix string
}

/* Routing Rule Data Structures */

// A Virtual Directory endpoint, provide a subset of ProxyEndpoint for better
// program structure than directly using ProxyEndpoint
type VirtualDirectoryEndpoint struct {
	MatchingPath        string //Matching prefix of the request path, also act as key
	Domain              string //Domain or IP to proxy to
	RequireTLS          bool   //Target domain require TLS
	SkipCertValidations bool   //Set to true to accept self signed certs
	Disabled            bool   //If the rule is enabled
}

// Rules and settings for header rewriting
type HeaderRewriteRules struct {
	UserDefinedHeaders           []*rewrite.UserDefinedHeader        //Custom headers to append when proxying requests from this endpoint
	RequestHostOverwrite         string                              //If not empty, this domain will be used to overwrite the Host field in request header
	HSTSMaxAge                   int64                               //HSTS max age, set to 0 for disable HSTS headers
	EnablePermissionPolicyHeader bool                                //Enable injection of permission policy header
	PermissionPolicy             *permissionpolicy.PermissionsPolicy //Permission policy header
	DisableHopByHopHeaderRemoval bool                                //Do not remove hop-by-hop headers

}

/*

	Authentication Providers

*/

const (
	AuthMethodNone     AuthMethod = iota //No authentication required
	AuthMethodBasic                      //Basic Auth
	AuthMethodAuthelia                   //Authelia
	AuthMethodOauth2                     //Oauth2
	AuthMethodAuthentik
)

type AuthenticationProvider struct {
	AuthMethod AuthMethod //The authentication method to use
	/* Basic Auth Settings */
	BasicAuthCredentials    []*BasicAuthCredentials   //Basic auth credentials
	BasicAuthExceptionRules []*BasicAuthExceptionRule //Path to exclude in a basic auth enabled proxy target
	BasicAuthGroupIDs       []string                  //Group IDs that are allowed to access this endpoint

	/* Authelia Settings */
	AutheliaURL string //URL of the Authelia server, leave empty to use global settings e.g. authelia.example.com
	UseHTTPS    bool   //Whether to use HTTPS for the Authelia server
}

// A proxy endpoint record, a general interface for handling inbound routing
type ProxyEndpointv321 struct {
	ProxyType            ProxyType               //The type of this proxy, see const def
	RootOrMatchingDomain string                  //Matching domain for host, also act as key
	MatchingDomainAlias  []string                //A list of domains that alias to this rule
	ActiveOrigins        []*loadbalance.Upstream //Activated Upstream or origin servers IP or domain to proxy to
	InactiveOrigins      []*loadbalance.Upstream //Disabled Upstream or origin servers IP or domain to proxy to
	UseStickySession     bool                    //Use stick session for load balancing
	UseActiveLoadBalance bool                    //Use active loadbalancing, default passive
	Disabled             bool                    //If the rule is disabled

	//Inbound TLS/SSL Related
	BypassGlobalTLS bool //Bypass global TLS setting options if TLS Listener enabled (parent.tlsListener != nil)

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

	//Uptime Monitor
	DisableUptimeMonitor bool //Disable uptime monitor for this endpoint

	//Access Control
	AccessFilterUUID string //Access filter ID

	//Fallback routing logic (Special Rule Sets Only)
	DefaultSiteOption int    //Fallback routing logic options
	DefaultSiteValue  string //Fallback routing target, optional

	//Internal Logic Elements
	Tags []string // Tags for the proxy endpoint
}
