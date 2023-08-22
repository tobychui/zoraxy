package dynamicproxy

import (
	"log"
	"net/url"

	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
)

/*
	Add an URL intoa custom subdomain service

*/

func (router *Router) AddSubdomainRoutingService(options *SubdOptions) error {
	domain := options.Domain
	if domain[len(domain)-1:] == "/" {
		domain = domain[:len(domain)-1]
	}

	webProxyEndpoint := domain
	if options.RequireTLS {
		webProxyEndpoint = "https://" + webProxyEndpoint
	} else {
		webProxyEndpoint = "http://" + webProxyEndpoint
	}

	//Create a new proxy agent for this root
	path, err := url.Parse(webProxyEndpoint)
	if err != nil {
		return err
	}

	proxy := dpcore.NewDynamicProxyCore(path, "", options.SkipCertValidations)

	router.SubdomainEndpoint.Store(options.MatchingDomain, &ProxyEndpoint{
		RootOrMatchingDomain:    options.MatchingDomain,
		Domain:                  domain,
		RequireTLS:              options.RequireTLS,
		Proxy:                   proxy,
		SkipCertValidations:     options.SkipCertValidations,
		RequireBasicAuth:        options.RequireBasicAuth,
		BasicAuthCredentials:    options.BasicAuthCredentials,
		BasicAuthExceptionRules: options.BasicAuthExceptionRules,
	})

	log.Println("Adding Subdomain Rule: ", options.MatchingDomain+" to "+domain)
	return nil
}
