package dynamicproxy

import (
	"errors"
	"net/url"
	"strings"

	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
)

// Prepare proxy route generate a proxy handler service object for your endpoint
func (router *Router) PrepareProxyRoute(endpoint *ProxyEndpoint) (*ProxyEndpoint, error) {
	//Filter the tailing slash if any
	domain := endpoint.Domain
	if domain[len(domain)-1:] == "/" {
		domain = domain[:len(domain)-1]
	}
	endpoint.Domain = domain

	//Parse the web proxy endpoint
	webProxyEndpoint := domain
	if !strings.HasPrefix("http://", domain) && !strings.HasPrefix("https://", domain) {
		//TLS is not hardcoded in proxy target domain
		if endpoint.RequireTLS {
			webProxyEndpoint = "https://" + webProxyEndpoint
		} else {
			webProxyEndpoint = "http://" + webProxyEndpoint
		}
	}

	//Create a new proxy agent for this root
	path, err := url.Parse(webProxyEndpoint)
	if err != nil {
		return nil, err
	}

	//Create the proxy routing handler
	proxy := dpcore.NewDynamicProxyCore(path, "", endpoint.SkipCertValidations)
	endpoint.proxy = proxy
	endpoint.parent = router

	return endpoint, nil
}

// Add Proxy Route to current runtime. Call to PrepareProxyRoute before adding to runtime
func (router *Router) AddProxyRouteToRuntime(endpoint *ProxyEndpoint) error {
	if endpoint.proxy == nil {
		//This endpoint is not prepared
		return errors.New("proxy endpoint not ready. Use PrepareProxyRoute before adding to runtime")
	}
	// Push record into running subdomain endpoints
	router.ProxyEndpoints.Store(endpoint.RootOrMatchingDomain, endpoint)
	return nil
}

// Set given Proxy Route as Root. Call to PrepareProxyRoute before adding to runtime
func (router *Router) SetProxyRouteAsRoot(endpoint *ProxyEndpoint) error {
	if endpoint.proxy == nil {
		//This endpoint is not prepared
		return errors.New("proxy endpoint not ready. Use PrepareProxyRoute before adding to runtime")
	}
	// Push record into running root endpoints
	router.Root = endpoint
	return nil
}
