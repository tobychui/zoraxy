package v322

import "imuslab.com/zoraxy/mod/dynamicproxy/loadbalance"

/*

	Authentication Provider in v3.2.2

	The only change is the removal of the deprecated Authelia and Authentik SSO
	provider, and the addition of the new Forward Auth provider.

	Need to map all provider with ID = 4 into 2 and remove the old provider configs
*/

type AuthMethod int

/*
v3.2.1 Authentication Provider
const (
	AuthMethodNone     AuthMethod = iota //No authentication required
	AuthMethodBasic                      //Basic Auth
	AuthMethodAuthelia                   //Authelia => 2
	AuthMethodOauth2                     //Oauth2
	AuthMethodAuthentik					 //Authentik => 4
)

v3.2.2 Authentication Provider
const (
	AuthMethodNone    AuthMethod = iota //No authentication required
	AuthMethodBasic                     //Basic Auth
	AuthMethodForward                   //Forward => 2
	AuthMethodOauth2                    //Oauth2
)

We need to merge both Authelia and Authentik into the Forward Auth provider, and remove
*/
//The updated structure of the authentication provider
type AuthenticationProviderV322 struct {
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

// A proxy endpoint record, a general interface for handling inbound routing
type ProxyEndpointv322 struct {
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
	AuthenticationProvider *AuthenticationProviderV322

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
