package dynamicproxy

import (
	"net/http"
	"os"
	"strings"

	"imuslab.com/zoraxy/mod/geodb"
)

/*
	Server.go

	Main server for dynamic proxy core
*/

func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	//Check if this ip is in blacklist
	clientIpAddr := geodb.GetRequesterIP(r)
	if h.Parent.Option.GeodbStore.IsBlacklisted(clientIpAddr) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		template, err := os.ReadFile("./web/forbidden.html")
		if err != nil {
			w.Write([]byte("403 - Forbidden"))
		} else {
			w.Write(template)
		}
		h.logRequest(r, false, 403, "blacklist")
		return
	}

	//Check if this is a redirection url
	if h.Parent.Option.RedirectRuleTable.IsRedirectable(r) {
		statusCode := h.Parent.Option.RedirectRuleTable.HandleRedirect(w, r)
		h.logRequest(r, statusCode != 500, statusCode, "redirect")
		return
	}

	//Extract request host to see if it is virtual directory or subdomain
	domainOnly := r.Host
	if strings.Contains(r.Host, ":") {
		hostPath := strings.Split(r.Host, ":")
		domainOnly = hostPath[0]
	}

	if strings.Contains(r.Host, ".") {
		//This might be a subdomain. See if there are any subdomain proxy router for this
		//Remove the port if any

		sep := h.Parent.getSubdomainProxyEndpointFromHostname(domainOnly)
		if sep != nil {
			h.subdomainRequest(w, r, sep)
			return
		}
	}

	//Clean up the request URI
	proxyingPath := strings.TrimSpace(r.RequestURI)

	targetProxyEndpoint := h.Parent.getTargetProxyEndpointFromRequestURI(proxyingPath)
	if targetProxyEndpoint != nil {
		h.proxyRequest(w, r, targetProxyEndpoint)
	} else {
		h.proxyRequest(w, r, h.Parent.Root)
	}
}
