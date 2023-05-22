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

func (router *Router) getSubdomainProxyEndpointFromHostname(hostname string) *SubdomainEndpoint {
	var targetSubdomainEndpoint *SubdomainEndpoint = nil
	ep, ok := router.SubdomainEndpoint.Load(hostname)
	if ok {
		targetSubdomainEndpoint = ep.(*SubdomainEndpoint)
	}

	return targetSubdomainEndpoint
}

func (router *Router) rewriteURL(rooturl string, requestURL string) string {
	rewrittenURL := requestURL
	rewrittenURL = strings.TrimPrefix(rewrittenURL, strings.TrimSuffix(rooturl, "/"))
	return rewrittenURL
}

func (h *ProxyHandler) subdomainRequest(w http.ResponseWriter, r *http.Request, target *SubdomainEndpoint) {
	r.Header.Set("X-Forwarded-Host", r.Host)
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
		wspHandler := websocketproxy.NewProxy(u)
		wspHandler.ServeHTTP(w, r)
		return
	}

	r.Host = r.URL.Host
	err := target.Proxy.ServeHTTP(w, r)
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

func (h *ProxyHandler) proxyRequest(w http.ResponseWriter, r *http.Request, target *ProxyEndpoint) {
	rewriteURL := h.Parent.rewriteURL(target.Root, r.RequestURI)
	r.URL, _ = url.Parse(rewriteURL)

	r.Header.Set("X-Forwarded-Host", r.Host)
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
		wspHandler := websocketproxy.NewProxy(u)
		wspHandler.ServeHTTP(w, r)
		return
	}

	originalHostHeader := r.Host
	r.Host = r.URL.Host
	err := target.Proxy.ServeHTTP(w, r, &dpcore.ResponseRewriteRuleSet{
		ProxyDomain:  target.Domain,
		OriginalHost: originalHostHeader,
		UseTLS:       target.RequireTLS,
		PathPrefix:   target.Root,
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
