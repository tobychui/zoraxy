package dynamicproxy

import (
	"log"
	"net/http"
	"net/url"

	"imuslab.com/arozos/ReverseProxy/mod/websocketproxy"
)

func (router *Router) getTargetProxyEndpointFromRequestURI(requestURI string) *ProxyEndpoint {
	var targetProxyEndpoint *ProxyEndpoint = nil
	router.ProxyEndpoints.Range(func(key, value interface{}) bool {
		rootname := key.(string)
		if len(requestURI) >= len(rootname) && requestURI[:len(rootname)] == rootname {
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
	if len(requestURL) > len(rooturl) {
		return requestURL[len(rooturl):]
	}
	return ""
}

func (h *ProxyHandler) subdomainRequest(w http.ResponseWriter, r *http.Request, target *SubdomainEndpoint) {
	r.Header.Set("X-Forwarded-Host", r.Host)
	requestURL := r.URL.String()
	if r.Header["Upgrade"] != nil && r.Header["Upgrade"][0] == "websocket" {
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
		wspHandler := websocketproxy.NewProxy(u)
		wspHandler.ServeHTTP(w, r)
		return
	}

	r.Host = r.URL.Host
	err := target.Proxy.ServeHTTP(w, r)
	if err != nil {
		log.Println(err.Error())
	}

}

func (h *ProxyHandler) proxyRequest(w http.ResponseWriter, r *http.Request, target *ProxyEndpoint) {
	rewriteURL := h.Parent.rewriteURL(target.Root, r.RequestURI)
	r.URL, _ = url.Parse(rewriteURL)
	r.Header.Set("X-Forwarded-Host", r.Host)
	if r.Header["Upgrade"] != nil && r.Header["Upgrade"][0] == "websocket" {
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
		wspHandler := websocketproxy.NewProxy(u)
		wspHandler.ServeHTTP(w, r)
		return
	}

	r.Host = r.URL.Host
	err := target.Proxy.ServeHTTP(w, r)
	if err != nil {
		log.Println(err.Error())
	}
}
