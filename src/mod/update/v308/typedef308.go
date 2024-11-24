package v308

/*
	v308 type definition

	This file wrap up the self-contained data structure
	for v3.0.8 structure and allow automatic updates
	for future releases if required

	Some struct are identical as v307 and hence it is not redefined here
*/

/* Upstream or Origin Server */
type v308Upstream struct {
	//Upstream Proxy Configs
	OriginIpOrDomain         string //Target IP address or domain name with port
	RequireTLS               bool   //Require TLS connection
	SkipCertValidations      bool   //Set to true to accept self signed certs
	SkipWebSocketOriginCheck bool   //Skip origin check on websocket upgrade connections

	//Load balancing configs
	Weight  int //Prirotiy of fallback, set all to 0 for round robin
	MaxConn int //Maxmium connection to this server
}

// A proxy endpoint record, a general interface for handling inbound routing
type v308ProxyEndpoint struct {
	ProxyType            int             //The type of this proxy, see const def
	RootOrMatchingDomain string          //Matching domain for host, also act as key
	MatchingDomainAlias  []string        //A list of domains that alias to this rule
	ActiveOrigins        []*v308Upstream //Activated Upstream or origin servers IP or domain to proxy to
	InactiveOrigins      []*v308Upstream //Disabled Upstream or origin servers IP or domain to proxy to
	UseStickySession     bool            //Use stick session for load balancing
	Disabled             bool            //If the rule is disabled

	//Inbound TLS/SSL Related
	BypassGlobalTLS bool //Bypass global TLS setting options if TLS Listener enabled (parent.tlsListener != nil)

	//Virtual Directories
	VirtualDirectories []*v307VirtualDirectoryEndpoint

	//Custom Headers
	UserDefinedHeaders           []*v307UserDefinedHeader //Custom headers to append when proxying requests from this endpoint
	HSTSMaxAge                   int64                    //HSTS max age, set to 0 for disable HSTS headers
	EnablePermissionPolicyHeader bool                     //Enable injection of permission policy header
	PermissionPolicy             *v307PermissionsPolicy   //Permission policy header

	//Authentication
	RequireBasicAuth        bool                          //Set to true to request basic auth before proxy
	BasicAuthCredentials    []*v307BasicAuthCredentials   //Basic auth credentials
	BasicAuthExceptionRules []*v307BasicAuthExceptionRule //Path to exclude in a basic auth enabled proxy target

	// Rate Limiting
	RequireRateLimit bool
	RateLimit        int64 // Rate limit in requests per second

	//Access Control
	AccessFilterUUID string //Access filter ID

	//Fallback routing logic (Special Rule Sets Only)
	DefaultSiteOption int    //Fallback routing logic options
	DefaultSiteValue  string //Fallback routing target, optional
}
