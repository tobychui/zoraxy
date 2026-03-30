package dynamicproxy

import (
	"errors"
	"log"
	"net/url"
	"strings"
	"time"

	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
	"imuslab.com/zoraxy/mod/dynamicproxy/exploits"
	"imuslab.com/zoraxy/mod/utils"
)

/*
	Dynamic Proxy Router Functions

	This script handle the proxy rules router spawning
	and preparation
*/

// Prepare proxy route generate a proxy handler service object for your endpoint
func (router *Router) PrepareProxyRoute(endpoint *ProxyEndpoint) (*ProxyEndpoint, error) {
	for _, thisOrigin := range endpoint.ActiveOrigins {
		//Create the proxy routing handler
		err := thisOrigin.StartProxy()
		if err != nil {
			log.Println("Unable to setup upstream " + thisOrigin.OriginIpOrDomain + ": " + err.Error())
			continue
		}
	}

	endpoint.parent = router

	//Prepare proxy routing handler for each of the virtual directories
	for _, vdir := range endpoint.VirtualDirectories {
		domain := vdir.Domain
		if len(domain) == 0 {
			//invalid vdir
			continue
		}
		if domain[len(domain)-1:] == "/" {
			domain = domain[:len(domain)-1]
		}

		//Parse the web proxy endpoint
		webProxyEndpoint := domain
		if !strings.HasPrefix("http://", domain) && !strings.HasPrefix("https://", domain) {
			//TLS is not hardcoded in proxy target domain
			if vdir.RequireTLS {
				webProxyEndpoint = "https://" + webProxyEndpoint
			} else {
				webProxyEndpoint = "http://" + webProxyEndpoint
			}
		}

		path, err := url.Parse(webProxyEndpoint)
		if err != nil {
			return nil, err
		}

		proxy := dpcore.NewDynamicProxyCore(path, vdir.MatchingPath, &dpcore.DpcoreOptions{
			IgnoreTLSVerification: vdir.SkipCertValidations,
			FlushInterval:         500 * time.Millisecond,
		})
		vdir.proxy = proxy
		vdir.parent = endpoint
	}

	// Initialize the exploit detector for this endpoint
	endpoint.InitializeExploitDetector()

	return endpoint, nil
}

// Add Proxy Route to current runtime. Call to PrepareProxyRoute before adding to runtime
func (router *Router) AddProxyRouteToRuntime(endpoint *ProxyEndpoint) error {
	lookupHostname := strings.ToLower(endpoint.RootOrMatchingDomain)
	if len(endpoint.ActiveOrigins) == 0 {
		//There are no active origins. No need to check for ready
		router.ProxyEndpoints.Store(lookupHostname, endpoint)
		return nil
	}
	if !router.loadBalancer.UpstreamsReady(endpoint.ActiveOrigins) {
		//This endpoint is not prepared
		return errors.New("proxy endpoint not ready. Use PrepareProxyRoute before adding to runtime")
	}
	// Push record into running subdomain endpoints
	router.ProxyEndpoints.Store(lookupHostname, endpoint)
	return nil
}

// Set given Proxy Route as Root. Call to PrepareProxyRoute before adding to runtime
func (router *Router) SetProxyRouteAsRoot(endpoint *ProxyEndpoint) error {
	if !router.loadBalancer.UpstreamsReady(endpoint.ActiveOrigins) {
		//This endpoint is not prepared
		return errors.New("proxy endpoint not ready. Use PrepareProxyRoute before adding to runtime")
	}
	// Push record into running root endpoints
	router.Root = endpoint
	return nil
}

// ProxyEndpoint remove provide global access by key
func (router *Router) RemoveProxyEndpointByRootname(rootnameOrMatchingDomain string) error {
	targetEpt, err := router.LoadProxy(rootnameOrMatchingDomain)
	if err != nil {
		return err
	}

	return targetEpt.Remove()
}

// GetProxyEndpointById retrieves a proxy endpoint by its ID from the Router's ProxyEndpoints map.
// It returns the ProxyEndpoint if found, or an error if not found.
func (h *Router) GetProxyEndpointById(searchingDomain string, includeAlias bool) (*ProxyEndpoint, error) {
	var found *ProxyEndpoint
	h.ProxyEndpoints.Range(func(key, value interface{}) bool {
		proxy, ok := value.(*ProxyEndpoint)
		if ok && (proxy.RootOrMatchingDomain == searchingDomain || (includeAlias && utils.StringInArray(proxy.MatchingDomainAlias, searchingDomain))) {
			found = proxy
			return false // stop iteration
		}
		return true // continue iteration
	})
	if found != nil {
		return found, nil
	}
	return nil, errors.New("proxy rule with given id not found")
}

func (h *Router) GetProxyEndpointByAlias(alias string) (*ProxyEndpoint, error) {
	var found *ProxyEndpoint
	h.ProxyEndpoints.Range(func(key, value interface{}) bool {
		proxy, ok := value.(*ProxyEndpoint)
		if !ok {
			return true
		}
		//Also check for wildcard aliases that matches the alias
		for _, thisAlias := range proxy.MatchingDomainAlias {
			if ok && thisAlias == alias {
				found = proxy
				return false // stop iteration
			} else if ok && strings.HasPrefix(thisAlias, "*") {
				//Check if the alias matches a wildcard alias
				if strings.HasSuffix(alias, thisAlias[1:]) {
					found = proxy
					return false // stop iteration
				}
			}
		}
		return true // continue iteration
	})
	if found != nil {
		return found, nil
	}
	return nil, errors.New("proxy rule with given alias not found")
}

// InitializeExploitDetector initializes or updates the exploit detector for this proxy endpoint
func (pe *ProxyEndpoint) InitializeExploitDetector() {
	if pe.BlockCommonExploits || pe.BlockAICrawlers {
		exploitRespType := exploits.ExploitsRequestResponseType(pe.MitigationAction)
		pe.detector = exploits.NewExploitDetector(pe.BlockCommonExploits, pe.BlockAICrawlers, exploitRespType)
	} else {
		pe.detector = nil
	}
}
