package dynamicproxy

import (
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"

	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
	"imuslab.com/zoraxy/mod/geodb"
	"imuslab.com/zoraxy/mod/statistic"
	"imuslab.com/zoraxy/mod/websocketproxy"
)

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

func (router *Router) getSubdomainProxyEndpointFromHostname(hostname string) *ProxyEndpoint {
	var targetSubdomainEndpoint *ProxyEndpoint = nil
	ep, ok := router.SubdomainEndpoint.Load(hostname)
	if ok {
		targetSubdomainEndpoint = ep.(*ProxyEndpoint)
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

// Handle subdomain request
func (h *ProxyHandler) subdomainRequest(w http.ResponseWriter, r *http.Request, target *ProxyEndpoint) {
	r.Header.Set("X-Forwarded-Host", r.Host)
	r.Header.Set("X-Forwarded-Server", "zoraxy-"+h.Parent.Option.HostUUID)
	requestURL := r.URL.String()
	if r.Header["Upgrade"] != nil && strings.ToLower(r.Header["Upgrade"][0]) == "websocket" {
		//Handle WebSocket request. Forward the custom Upgrade header and rewrite origin
		r.Header.Set("A-Upgrade", "websocket")
		wsRedirectionEndpoint := target.Domain
		if wsRedirectionEndpoint[len(wsRedirectionEndpoint)-1:] != "/" {
			//Append / to the end of the redirection endpoint if not exists
			wsRedirectionEndpoint = wsRedirectionEndpoint + "/"
		}
		if len(requestURL) > 0 && requestURL[:1] == "/" {
			//Remove starting / from request URL if exists
			requestURL = requestURL[1:]
		}
		u, _ := url.Parse("ws://" + wsRedirectionEndpoint + requestURL)
		if target.RequireTLS {
			u, _ = url.Parse("wss://" + wsRedirectionEndpoint + requestURL)
		}
		h.logRequest(r, true, 101, "subdomain-websocket", target.Domain)
		wspHandler := websocketproxy.NewProxy(u, target.SkipCertValidations)
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

	err := target.Proxy.ServeHTTP(w, r, &dpcore.ResponseRewriteRuleSet{
		ProxyDomain:  target.Domain,
		OriginalHost: originalHostHeader,
		UseTLS:       target.RequireTLS,
		PathPrefix:   "",
	})

	var dnsError *net.DNSError
	if err != nil {
		if errors.As(err, &dnsError) {
			http.ServeFile(w, r, "./web/hosterror.html")
			log.Println(err.Error())
			h.logRequest(r, false, 404, "subdomain-http", target.Domain)
		} else {
			http.ServeFile(w, r, "./web/rperror.html")
			log.Println(err.Error())
			h.logRequest(r, false, 521, "subdomain-http", target.Domain)
		}
	}

	h.logRequest(r, true, 200, "subdomain-http", target.Domain)
}

// Handle vdir type request
func (h *ProxyHandler) proxyRequest(w http.ResponseWriter, r *http.Request, target *ProxyEndpoint) {
	rewriteURL := h.Parent.rewriteURL(target.RootOrMatchingDomain, r.RequestURI)
	r.URL, _ = url.Parse(rewriteURL)

	r.Header.Set("X-Forwarded-Host", r.Host)
	r.Header.Set("X-Forwarded-Server", "zoraxy-"+h.Parent.Option.HostUUID)
	if r.Header["Upgrade"] != nil && strings.ToLower(r.Header["Upgrade"][0]) == "websocket" {
		//Handle WebSocket request. Forward the custom Upgrade header and rewrite origin
		r.Header.Set("A-Upgrade", "websocket")
		wsRedirectionEndpoint := target.Domain
		if wsRedirectionEndpoint[len(wsRedirectionEndpoint)-1:] != "/" {
			wsRedirectionEndpoint = wsRedirectionEndpoint + "/"
		}
		u, _ := url.Parse("ws://" + wsRedirectionEndpoint + r.URL.String())
		if target.RequireTLS {
			u, _ = url.Parse("wss://" + wsRedirectionEndpoint + r.URL.String())
		}
		h.logRequest(r, true, 101, "vdir-websocket", target.Domain)
		wspHandler := websocketproxy.NewProxy(u, target.SkipCertValidations)
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

	err := target.Proxy.ServeHTTP(w, r, &dpcore.ResponseRewriteRuleSet{
		ProxyDomain:  target.Domain,
		OriginalHost: originalHostHeader,
		UseTLS:       target.RequireTLS,
		PathPrefix:   target.RootOrMatchingDomain,
	})

	var dnsError *net.DNSError
	if err != nil {
		if errors.As(err, &dnsError) {
			http.ServeFile(w, r, "./web/hosterror.html")
			log.Println(err.Error())
			h.logRequest(r, false, 404, "vdir-http", target.Domain)
		} else {
			http.ServeFile(w, r, "./web/rperror.html")
			log.Println(err.Error())
			h.logRequest(r, false, 521, "vdir-http", target.Domain)
		}
	}
	h.logRequest(r, true, 200, "vdir-http", target.Domain)

}

func (h *ProxyHandler) logRequest(r *http.Request, succ bool, statusCode int, forwardType string, target string) {
	if h.Parent.Option.StatisticCollector != nil {
		go func() {
			requestInfo := statistic.RequestInfo{
				IpAddr:                        geodb.GetRequesterIP(r),
				RequestOriginalCountryISOCode: h.Parent.Option.GeodbStore.GetRequesterCountryISOCode(r),
				Succ:                          succ,
				StatusCode:                    statusCode,
				ForwardType:                   forwardType,
				Referer:                       r.Referer(),
				UserAgent:                     r.UserAgent(),
				RequestURL:                    r.Host + r.RequestURI,
				Target:                        target,
			}
			h.Parent.Option.StatisticCollector.RecordRequest(requestInfo)
		}()

	}
}
