package dynamicproxy

import (
	"log"
	"net/url"

	"imuslab.com/arozos/ReverseProxy/mod/reverseproxy"
)

/*
	Add an URL intoa custom subdomain service

*/

func (router *Router) AddSubdomainRoutingService(hostnameWithSubdomain string, domain string, requireTLS bool) error {
	if domain[len(domain)-1:] == "/" {
		domain = domain[:len(domain)-1]
	}

	webProxyEndpoint := domain
	if requireTLS {
		webProxyEndpoint = "https://" + webProxyEndpoint
	} else {
		webProxyEndpoint = "http://" + webProxyEndpoint
	}

	//Create a new proxy agent for this root
	path, err := url.Parse(webProxyEndpoint)
	if err != nil {
		return err
	}

	proxy := reverseproxy.NewReverseProxy(path)

	router.SubdomainEndpoint.Store(hostnameWithSubdomain, &SubdomainEndpoint{
		MatchingDomain: hostnameWithSubdomain,
		Domain:         domain,
		RequireTLS:     requireTLS,
		Proxy:          proxy,
	})

	log.Println("Adding Subdomain Rule: ", hostnameWithSubdomain+" to "+domain)
	return nil
}
