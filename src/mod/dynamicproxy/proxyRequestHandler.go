package dynamicproxy

import (
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
	"imuslab.com/zoraxy/mod/netutils"
	"imuslab.com/zoraxy/mod/statistic"
	"imuslab.com/zoraxy/mod/websocketproxy"
)

// Check if the request URI matches any of the proxy endpoint
func (router *Router) getTargetProxyEndpointFromRequestURI(requestURI string) *ProxyEndpoint {
	var targetProxyEndpoint *ProxyEndpoint = nil
	router.ProxyEndpoints.Range(func(key, value interface{}) bool {
		rootname := key.(string)
		if strings.HasPrefix(requestURI, rootname) {
			thisProxyEndpoint := value.(*ProxyEndpoint)
			targetProxyEndpoint = thisProxyEndpoint
		}
		return true
	})

	return targetProxyEndpoint
}

// Get the proxy endpoint from hostname, which might includes checking of wildcard certificates
func (router *Router) getProxyEndpointFromHostname(hostname string) *ProxyEndpoint {
	var targetSubdomainEndpoint *ProxyEndpoint = nil
	ep, ok := router.ProxyEndpoints.Load(hostname)
	if ok {
		//Exact hit
		targetSubdomainEndpoint = ep.(*ProxyEndpoint)
		if !targetSubdomainEndpoint.Disabled {
			return targetSubdomainEndpoint
		}
	}

	//No hit. Try with wildcard and alias
	matchProxyEndpoints := []*ProxyEndpoint{}
	router.ProxyEndpoints.Range(func(k, v interface{}) bool {
		ep := v.(*ProxyEndpoint)
		match, err := filepath.Match(ep.RootOrMatchingDomain, hostname)
		if err != nil {
			//Bad pattern. Skip this rule
			return true
		}

		if match {
			//Wildcard matches. Skip checking alias
			matchProxyEndpoints = append(matchProxyEndpoints, ep)
			return true
		}

		//Wildcard not match. Check for alias
		if ep.MatchingDomainAlias != nil && len(ep.MatchingDomainAlias) > 0 {
			for _, aliasDomain := range ep.MatchingDomainAlias {
				match, err := filepath.Match(aliasDomain, hostname)
				if err != nil {
					//Bad pattern. Skip this alias
					continue
				}

				if match {
					//This alias match
					matchProxyEndpoints = append(matchProxyEndpoints, ep)
					return true
				}
			}
		}
		return true
	})

	if len(matchProxyEndpoints) == 1 {
		//Only 1 match
		return matchProxyEndpoints[0]
	} else if len(matchProxyEndpoints) > 1 {
		//More than one match. Get the best match one
		sort.Slice(matchProxyEndpoints, func(i, j int) bool {
			return matchProxyEndpoints[i].RootOrMatchingDomain < matchProxyEndpoints[j].RootOrMatchingDomain
		})
		return matchProxyEndpoints[0]
	}

	return targetSubdomainEndpoint
}

// Clearn URL Path (without the http:// part) replaces // in a URL to /
func (router *Router) clearnURL(targetUrlOPath string) string {
	return strings.ReplaceAll(targetUrlOPath, "//", "/")
}

// Rewrite URL rewrite the prefix part of a virtual directory URL with /
func (router *Router) rewriteURL(rooturl string, requestURL string) string {
	rewrittenURL := requestURL
	rewrittenURL = strings.TrimPrefix(rewrittenURL, strings.TrimSuffix(rooturl, "/"))

	if strings.Contains(rewrittenURL, "//") {
		rewrittenURL = router.clearnURL(rewrittenURL)
	}
	return rewrittenURL
}

// Handle host request
func (h *ProxyHandler) hostRequest(w http.ResponseWriter, r *http.Request, target *ProxyEndpoint) {
	r.Header.Set("X-Forwarded-Host", r.Host)
	r.Header.Set("X-Forwarded-Server", "zoraxy-"+h.Parent.Option.HostUUID)
	selectedUpstream, err := h.Parent.loadBalancer.GetRequestUpstreamTarget(r, target.ActiveOrigins)
	if err != nil {
		http.ServeFile(w, r, "./web/rperror.html")
		log.Println(err.Error())
		h.Parent.logRequest(r, false, 521, "subdomain-http", r.URL.Hostname())
		return
	}
	requestURL := r.URL.String()
	if r.Header["Upgrade"] != nil && strings.ToLower(r.Header["Upgrade"][0]) == "websocket" {
		//Handle WebSocket request. Forward the custom Upgrade header and rewrite origin
		r.Header.Set("Zr-Origin-Upgrade", "websocket")
		wsRedirectionEndpoint := selectedUpstream.OriginIpOrDomain
		if wsRedirectionEndpoint[len(wsRedirectionEndpoint)-1:] != "/" {
			//Append / to the end of the redirection endpoint if not exists
			wsRedirectionEndpoint = wsRedirectionEndpoint + "/"
		}
		if len(requestURL) > 0 && requestURL[:1] == "/" {
			//Remove starting / from request URL if exists
			requestURL = requestURL[1:]
		}
		u, _ := url.Parse("ws://" + wsRedirectionEndpoint + requestURL)
		if selectedUpstream.RequireTLS {
			u, _ = url.Parse("wss://" + wsRedirectionEndpoint + requestURL)
		}
		h.Parent.logRequest(r, true, 101, "subdomain-websocket", selectedUpstream.OriginIpOrDomain)
		wspHandler := websocketproxy.NewProxy(u, websocketproxy.Options{
			SkipTLSValidation: selectedUpstream.SkipCertValidations,
			SkipOriginCheck:   selectedUpstream.SkipWebSocketOriginCheck,
		})
		wspHandler.ServeHTTP(w, r)
		return
	}

	originalHostHeader := r.Host
	if r.URL != nil {
		r.Host = r.URL.Host
	} else {
		//Fallback when the upstream proxy screw something up in the header
		r.URL, _ = url.Parse(originalHostHeader)
	}

	//Build downstream and upstream header rules
	upstreamHeaders, downstreamHeaders := target.SplitInboundOutboundHeaders()

	err = selectedUpstream.ServeHTTP(w, r, &dpcore.ResponseRewriteRuleSet{
		ProxyDomain:       selectedUpstream.OriginIpOrDomain,
		OriginalHost:      originalHostHeader,
		UseTLS:            selectedUpstream.RequireTLS,
		NoCache:           h.Parent.Option.NoCache,
		PathPrefix:        "",
		UpstreamHeaders:   upstreamHeaders,
		DownstreamHeaders: downstreamHeaders,
		Version:           target.parent.Option.HostVersion,
	})

	var dnsError *net.DNSError
	if err != nil {
		if errors.As(err, &dnsError) {
			http.ServeFile(w, r, "./web/hosterror.html")
			log.Println(err.Error())
			h.Parent.logRequest(r, false, 404, "subdomain-http", r.URL.Hostname())
		} else {
			http.ServeFile(w, r, "./web/rperror.html")
			log.Println(err.Error())
			h.Parent.logRequest(r, false, 521, "subdomain-http", r.URL.Hostname())
		}
	}

	h.Parent.logRequest(r, true, 200, "subdomain-http", r.URL.Hostname())
}

// Handle vdir type request
func (h *ProxyHandler) vdirRequest(w http.ResponseWriter, r *http.Request, target *VirtualDirectoryEndpoint) {
	rewriteURL := h.Parent.rewriteURL(target.MatchingPath, r.RequestURI)
	r.URL, _ = url.Parse(rewriteURL)

	r.Header.Set("X-Forwarded-Host", r.Host)
	r.Header.Set("X-Forwarded-Server", "zoraxy-"+h.Parent.Option.HostUUID)

	if r.Header["Upgrade"] != nil && strings.ToLower(r.Header["Upgrade"][0]) == "websocket" {
		//Handle WebSocket request. Forward the custom Upgrade header and rewrite origin
		r.Header.Set("Zr-Origin-Upgrade", "websocket")
		wsRedirectionEndpoint := target.Domain
		if wsRedirectionEndpoint[len(wsRedirectionEndpoint)-1:] != "/" {
			wsRedirectionEndpoint = wsRedirectionEndpoint + "/"
		}
		u, _ := url.Parse("ws://" + wsRedirectionEndpoint + r.URL.String())
		if target.RequireTLS {
			u, _ = url.Parse("wss://" + wsRedirectionEndpoint + r.URL.String())
		}
		h.Parent.logRequest(r, true, 101, "vdir-websocket", target.Domain)
		wspHandler := websocketproxy.NewProxy(u, websocketproxy.Options{
			SkipTLSValidation: target.SkipCertValidations,
			SkipOriginCheck:   true, //You should not use websocket via virtual directory. But keep this to true for compatibility
		})
		wspHandler.ServeHTTP(w, r)
		return
	}

	originalHostHeader := r.Host
	if r.URL != nil {
		r.Host = r.URL.Host
	} else {
		//Fallback when the upstream proxy screw something up in the header
		r.URL, _ = url.Parse(originalHostHeader)
	}

	//Build downstream and upstream header rules
	upstreamHeaders, downstreamHeaders := target.parent.SplitInboundOutboundHeaders()

	err := target.proxy.ServeHTTP(w, r, &dpcore.ResponseRewriteRuleSet{
		ProxyDomain:       target.Domain,
		OriginalHost:      originalHostHeader,
		UseTLS:            target.RequireTLS,
		PathPrefix:        target.MatchingPath,
		UpstreamHeaders:   upstreamHeaders,
		DownstreamHeaders: downstreamHeaders,
		Version:           target.parent.parent.Option.HostVersion,
	})

	var dnsError *net.DNSError
	if err != nil {
		if errors.As(err, &dnsError) {
			http.ServeFile(w, r, "./web/hosterror.html")
			log.Println(err.Error())
			h.Parent.logRequest(r, false, 404, "vdir-http", target.Domain)
		} else {
			http.ServeFile(w, r, "./web/rperror.html")
			log.Println(err.Error())
			h.Parent.logRequest(r, false, 521, "vdir-http", target.Domain)
		}
	}
	h.Parent.logRequest(r, true, 200, "vdir-http", target.Domain)

}

func (router *Router) logRequest(r *http.Request, succ bool, statusCode int, forwardType string, target string) {
	if router.Option.StatisticCollector != nil {
		go func() {
			requestInfo := statistic.RequestInfo{
				IpAddr:                        netutils.GetRequesterIP(r),
				RequestOriginalCountryISOCode: router.Option.GeodbStore.GetRequesterCountryISOCode(r),
				Succ:                          succ,
				StatusCode:                    statusCode,
				ForwardType:                   forwardType,
				Referer:                       r.Referer(),
				UserAgent:                     r.UserAgent(),
				RequestURL:                    r.Host + r.RequestURI,
				Target:                        target,
			}
			router.Option.StatisticCollector.RecordRequest(requestInfo)
		}()
	}
}
