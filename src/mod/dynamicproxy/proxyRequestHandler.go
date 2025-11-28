package dynamicproxy

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
	"imuslab.com/zoraxy/mod/dynamicproxy/loadbalance"
	"imuslab.com/zoraxy/mod/dynamicproxy/rewrite"
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
func (router *Router) GetProxyEndpointFromHostname(hostname string) *ProxyEndpoint {
	var targetSubdomainEndpoint *ProxyEndpoint = nil
	hostname = strings.ToLower(hostname)
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
		if ep.Disabled {
			//Skip disabled endpoint
			return true
		}

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
		if len(ep.MatchingDomainAlias) > 0 {
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
		return matchProxyEndpoints[0]
	} else if len(matchProxyEndpoints) > 1 {
		// More than one match, pick one that is:
		// 1. longer RootOrMatchingDomain (more specific)
		// 2. fewer wildcard characters (* and ?) (more specific)
		// 3. fallback to lexicographic order
		sort.SliceStable(matchProxyEndpoints, func(i, j int) bool {
			a := matchProxyEndpoints[i].RootOrMatchingDomain
			b := matchProxyEndpoints[j].RootOrMatchingDomain
			if len(a) != len(b) {
				return len(a) > len(b)
			}
			aw := strings.Count(a, "*") + strings.Count(a, "?")
			bw := strings.Count(b, "*") + strings.Count(b, "?")
			if aw != bw {
				return aw < bw
			}
			return a < b
		})
		return matchProxyEndpoints[0]
	}

	return targetSubdomainEndpoint
}

// Rewrite URL rewrite the prefix part of a virtual directory URL with /
func (router *Router) rewriteURL(rooturl string, requestURL string) string {
	rewrittenURL := requestURL
	rewrittenURL = strings.TrimPrefix(rewrittenURL, strings.TrimSuffix(rooturl, "/"))

	if strings.Contains(rewrittenURL, "//") {
		rewrittenURL = strings.ReplaceAll(rewrittenURL, "//", "/")
	}
	return rewrittenURL
}

// upstreamHostSwap check if this loopback to one of the proxy rule in the system. If yes, do a shortcut target swap
// this prevents unnecessary external DNS lookup and connection, return true if swapped and request is already handled
// by the loopback handler. Only continue if return is false
func (h *ProxyHandler) upstreamHostSwap(w http.ResponseWriter, r *http.Request, selectedUpstream *loadbalance.Upstream, currentTarget *ProxyEndpoint) bool {
	upstreamHostname := selectedUpstream.OriginIpOrDomain
	if strings.Contains(upstreamHostname, ":") {
		upstreamHostname = strings.Split(upstreamHostname, ":")[0]
	}
	loopbackProxyEndpoint := h.Parent.GetProxyEndpointFromHostname(upstreamHostname)
	if loopbackProxyEndpoint != nil && loopbackProxyEndpoint != currentTarget {
		//This is a loopback request. Swap the target to the loopback target
		//h.Parent.Option.Logger.PrintAndLog("proxy", "Detected a loopback request to self. Swap the target to "+loopbackProxyEndpoint.RootOrMatchingDomain, nil)
		if loopbackProxyEndpoint.IsEnabled() {
			h.hostRequest(w, r, loopbackProxyEndpoint)
		} else {
			//Endpoint disabled, return 503
			http.ServeFile(w, r, "./web/rperror.html")
			h.Parent.logRequest(r, false, 521, "host-http", r.Host, upstreamHostname, currentTarget)
		}
		return true
	}
	return false
}

// Handle host request
func (h *ProxyHandler) hostRequest(w http.ResponseWriter, r *http.Request, target *ProxyEndpoint) {
	r.Header.Set("X-Forwarded-Host", r.Host)
	r.Header.Set("X-Forwarded-Server", "zoraxy-"+h.Parent.Option.HostUUID)
	reqHostname := r.Host

	/* Load balancing */
	selectedUpstream, err := h.Parent.loadBalancer.GetRequestUpstreamTarget(w, r, target.ActiveOrigins, target.UseStickySession, target.DisableAutoFallback)
	if err != nil {
		http.ServeFile(w, r, "./web/rperror.html")
		h.Parent.Option.Logger.PrintAndLog("proxy", "Failed to assign an upstream for this request", err)
		h.Parent.logRequest(r, false, 521, "subdomain-http", r.URL.Hostname(), r.Host, target)
		return
	}

	/* Upstream Host Swap (use to detect loopback to self) */
	if h.upstreamHostSwap(w, r, selectedUpstream, target) {
		//Request handled by the loopback handler
		return
	}

	/* WebSocket automatic proxy */
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
		h.Parent.logRequest(r, true, 101, "host-websocket", reqHostname, selectedUpstream.OriginIpOrDomain, target)

		if target.HeaderRewriteRules == nil {
			target.HeaderRewriteRules = GetDefaultHeaderRewriteRules()
		}

		wspHandler := websocketproxy.NewProxy(u, websocketproxy.Options{
			SkipTLSValidation:  selectedUpstream.SkipCertValidations,
			SkipOriginCheck:    selectedUpstream.SkipWebSocketOriginCheck,
			CopyAllHeaders:     target.EnableWebsocketCustomHeaders,
			UserDefinedHeaders: target.HeaderRewriteRules.UserDefinedHeaders,
			Logger:             h.Parent.Option.Logger,
		})
		wspHandler.ServeHTTP(w, r)
		return
	}

	if r.URL != nil {
		r.Host = r.URL.Host
	} else {
		//Fallback when the upstream proxy screw something up in the header
		r.URL, _ = url.Parse(reqHostname)
	}

	//Populate the user-defined headers with the values from the request
	headerRewriteOptions := GetDefaultHeaderRewriteRules()
	if target.HeaderRewriteRules != nil {
		headerRewriteOptions = target.HeaderRewriteRules
	}
	rewrittenUserDefinedHeaders := rewrite.PopulateRequestHeaderVariables(r, headerRewriteOptions.UserDefinedHeaders)

	//Build downstream and upstream header rules
	upstreamHeaders, downstreamHeaders := rewrite.SplitUpDownStreamHeaders(&rewrite.HeaderRewriteOptions{
		UserDefinedHeaders:           rewrittenUserDefinedHeaders,
		HSTSMaxAge:                   headerRewriteOptions.HSTSMaxAge,
		HSTSIncludeSubdomains:        target.ContainsWildcardName(true),
		EnablePermissionPolicyHeader: headerRewriteOptions.EnablePermissionPolicyHeader,
		PermissionPolicy:             headerRewriteOptions.PermissionPolicy,
	})

	//Handle the request reverse proxy
	statusCode, err := selectedUpstream.ServeHTTP(w, r, &dpcore.ResponseRewriteRuleSet{
		ProxyDomain:                    selectedUpstream.OriginIpOrDomain,
		OriginalHost:                   reqHostname,
		UseTLS:                         selectedUpstream.RequireTLS,
		NoCache:                        h.Parent.Option.NoCache,
		PathPrefix:                     "",
		UpstreamHeaders:                upstreamHeaders,
		DownstreamHeaders:              downstreamHeaders,
		DisableChunkedTransferEncoding: target.DisableChunkedTransferEncoding,
		NoRemoveUserAgentHeader:        headerRewriteOptions.DisableUserAgentHeaderRemoval,
		HostHeaderOverwrite:            headerRewriteOptions.RequestHostOverwrite,
		NoRemoveHopByHop:               headerRewriteOptions.DisableHopByHopHeaderRemoval,
		Version:                        target.parent.Option.HostVersion,
		DevelopmentMode:                target.parent.Option.DevelopmentMode,
	})

	//validate the error
	var dnsError *net.DNSError
	upstreamHostname := selectedUpstream.OriginIpOrDomain
	if err != nil {
		if errors.As(err, &dnsError) {
			http.ServeFile(w, r, "./web/hosterror.html")
			h.Parent.logRequest(r, false, 404, "host-http", reqHostname, upstreamHostname, target)
		} else if errors.Is(err, context.Canceled) {
			//Request canceled by client, usually due to manual refresh before page load
			http.Error(w, "Request canceled", http.StatusRequestTimeout)
			h.Parent.logRequest(r, false, http.StatusRequestTimeout, "host-http", reqHostname, upstreamHostname, target)
		} else {
			http.ServeFile(w, r, "./web/rperror.html")
			h.Parent.logRequest(r, false, 521, "host-http", reqHostname, upstreamHostname, target)
		}
	}

	h.Parent.logRequest(r, true, statusCode, "host-http", reqHostname, upstreamHostname, target)
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

		if target.parent.HeaderRewriteRules != nil {
			target.parent.HeaderRewriteRules = GetDefaultHeaderRewriteRules()
		}

		h.Parent.logRequest(r, true, 101, "vdir-websocket", r.Host, target.Domain, target.parent)
		wspHandler := websocketproxy.NewProxy(u, websocketproxy.Options{
			SkipTLSValidation:  target.SkipCertValidations,
			SkipOriginCheck:    true,                                       //You should not use websocket via virtual directory. But keep this to true for compatibility
			CopyAllHeaders:     target.parent.EnableWebsocketCustomHeaders, //Left this as default to prevent nginx user setting / as vdir
			UserDefinedHeaders: target.parent.HeaderRewriteRules.UserDefinedHeaders,
			Logger:             h.Parent.Option.Logger,
		})
		wspHandler.ServeHTTP(w, r)
		return
	}

	reqHostname := r.Host
	if r.URL != nil {
		r.Host = r.URL.Host
	} else {
		//Fallback when the upstream proxy screw something up in the header
		r.URL, _ = url.Parse(reqHostname)
	}

	//Populate the user-defined headers with the values from the request
	headerRewriteOptions := GetDefaultHeaderRewriteRules()
	if target.parent.HeaderRewriteRules != nil {
		headerRewriteOptions = target.parent.HeaderRewriteRules
	}

	rewrittenUserDefinedHeaders := rewrite.PopulateRequestHeaderVariables(r, headerRewriteOptions.UserDefinedHeaders)

	//Build downstream and upstream header rules, use the parent (subdomain) endpoint's headers
	upstreamHeaders, downstreamHeaders := rewrite.SplitUpDownStreamHeaders(&rewrite.HeaderRewriteOptions{
		UserDefinedHeaders:           rewrittenUserDefinedHeaders,
		HSTSMaxAge:                   headerRewriteOptions.HSTSMaxAge,
		HSTSIncludeSubdomains:        target.parent.ContainsWildcardName(true),
		EnablePermissionPolicyHeader: headerRewriteOptions.EnablePermissionPolicyHeader,
		PermissionPolicy:             headerRewriteOptions.PermissionPolicy,
	})

	//Handle the virtual directory reverse proxy request
	statusCode, err := target.proxy.ServeHTTP(w, r, &dpcore.ResponseRewriteRuleSet{
		ProxyDomain:                    target.Domain,
		OriginalHost:                   reqHostname,
		UseTLS:                         target.RequireTLS,
		PathPrefix:                     target.MatchingPath,
		UpstreamHeaders:                upstreamHeaders,
		DownstreamHeaders:              downstreamHeaders,
		DisableChunkedTransferEncoding: target.parent.DisableChunkedTransferEncoding,
		NoRemoveUserAgentHeader:        headerRewriteOptions.DisableUserAgentHeaderRemoval,
		HostHeaderOverwrite:            headerRewriteOptions.RequestHostOverwrite,
		Version:                        target.parent.parent.Option.HostVersion,
		DevelopmentMode:                target.parent.parent.Option.DevelopmentMode,
	})

	var dnsError *net.DNSError
	if err != nil {
		if errors.As(err, &dnsError) {
			http.ServeFile(w, r, "./web/hosterror.html")
			log.Println(err.Error())
			h.Parent.logRequest(r, false, 404, "vdir-http", reqHostname, target.Domain, target.parent)
		} else {
			http.ServeFile(w, r, "./web/rperror.html")
			log.Println(err.Error())
			h.Parent.logRequest(r, false, 521, "vdir-http", reqHostname, target.Domain, target.parent)
		}
	}
	h.Parent.logRequest(r, true, statusCode, "vdir-http", reqHostname, target.Domain, target.parent)

}

// This logger collect data for the statistical analysis. For log to file logger, check the Logger and LogHTTPRequest handler
func (router *Router) logRequest(r *http.Request, succ bool, statusCode int, forwardType string, originalHostname string, upstreamHostname string, endpoint *ProxyEndpoint) {
	if endpoint != nil && endpoint.DisableLogging {
		// Notes: endpoint can be nil if the request has been handled before a host name can be resolved
		// e.g. Redirection matching rule
		// in that case we will log it by default and will not enter this routine
		return
	}

	router.Option.Logger.LogHTTPRequest(r, forwardType, statusCode, originalHostname, upstreamHostname)

	if router.Option.StatisticCollector == nil {
		// Statistic collection not yet initialized
		return
	}

	if endpoint != nil && endpoint.DisableStatisticCollection {
		// Endpoint level statistic collection disabled
		return
	}

	// Proceed to record the request info
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
			Target:                        originalHostname,
			Upstream:                      upstreamHostname,
		}
		router.Option.StatisticCollector.RecordRequest(requestInfo)
	}()

}
