package v315

import (
	"imuslab.com/zoraxy/mod/dynamicproxy/loadbalance"
	"imuslab.com/zoraxy/mod/dynamicproxy/permissionpolicy"
	"imuslab.com/zoraxy/mod/dynamicproxy/rewrite"
)

// A proxy endpoint record, a general interface for handling inbound routing
type v314ProxyEndpoint struct {
	ProxyType            int                     //The type of this proxy, see const def
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
	UserDefinedHeaders           []*rewrite.UserDefinedHeader        //Custom headers to append when proxying requests from this endpoint
	RequestHostOverwrite         string                              //If not empty, this domain will be used to overwrite the Host field in request header
	HSTSMaxAge                   int64                               //HSTS max age, set to 0 for disable HSTS headers
	EnablePermissionPolicyHeader bool                                //Enable injection of permission policy header
	PermissionPolicy             *permissionpolicy.PermissionsPolicy //Permission policy header
	DisableHopByHopHeaderRemoval bool                                //Do not remove hop-by-hop headers

	//Authentication
	RequireBasicAuth        bool                      //Set to true to request basic auth before proxy
	BasicAuthCredentials    []*BasicAuthCredentials   //Basic auth credentials
	BasicAuthExceptionRules []*BasicAuthExceptionRule //Path to exclude in a basic auth enabled proxy target
	UseSSOIntercept         bool                      //Allow SSO to intercept this endpoint and provide authentication via Oauth2 credentials

	// Rate Limiting
	RequireRateLimit bool
	RateLimit        int64 // Rate limit in requests per second

	//Access Control
	AccessFilterUUID string //Access filter ID

	//Fallback routing logic (Special Rule Sets Only)
	DefaultSiteOption int    //Fallback routing logic options
	DefaultSiteValue  string //Fallback routing target, optional
}
