package dynamicproxy

import (
	"errors"
	"log"
	"net/url"
	"strings"
	"time"

	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
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

	return endpoint, nil
}

// Add Proxy Route to current runtime. Call to PrepareProxyRoute before adding to runtime
func (router *Router) AddProxyRouteToRuntime(endpoint *ProxyEndpoint) error {
	if !router.loadBalancer.UpstreamsReady(endpoint.ActiveOrigins) {
		//This endpoint is not prepared
		return errors.New("proxy endpoint not ready. Use PrepareProxyRoute before adding to runtime")
	}
	// Push record into running subdomain endpoints
	router.ProxyEndpoints.Store(endpoint.RootOrMatchingDomain, endpoint)
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
