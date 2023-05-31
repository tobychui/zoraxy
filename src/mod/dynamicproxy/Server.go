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

	Routing Handler Priority (High to Low)
	- Blacklist
	- Whitelist
	- Redirectable
	- Subdomain Routing
	- Vitrual Directory Routing
*/

func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	/*
		General Access Check
	*/

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
		h.logRequest(r, false, 403, "blacklist", "")
		return
	}

	//Check if this ip is in whitelist
	if !h.Parent.Option.GeodbStore.IsWhitelisted(clientIpAddr) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		template, err := os.ReadFile("./web/forbidden.html")
		if err != nil {
			w.Write([]byte("403 - Forbidden"))
		} else {
			w.Write(template)
		}
		h.logRequest(r, false, 403, "whitelist", "")
		return
	}

	/*
		Redirection Routing
	*/
	//Check if this is a redirection url
	if h.Parent.Option.RedirectRuleTable.IsRedirectable(r) {
		statusCode := h.Parent.Option.RedirectRuleTable.HandleRedirect(w, r)
		h.logRequest(r, statusCode != 500, statusCode, "redirect", "")
		return
	}

	//Check if there are external routing rule matches.
	//If yes, route them via external rr
	matchedRoutingRule := h.Parent.GetMatchingRoutingRule(r)
	if matchedRoutingRule != nil {
		//Matching routing rule found. Let the sub-router handle it
		matchedRoutingRule.Route(w, r)
		return
	}

	//Extract request host to see if it is virtual directory or subdomain
	domainOnly := r.Host
	if strings.Contains(r.Host, ":") {
		hostPath := strings.Split(r.Host, ":")
		domainOnly = hostPath[0]
	}

	/*
		Subdomain Routing
	*/
	if strings.Contains(r.Host, ".") {
		//This might be a subdomain. See if there are any subdomain proxy router for this
		sep := h.Parent.getSubdomainProxyEndpointFromHostname(domainOnly)
		if sep != nil {
			if sep.RequireBasicAuth {
				err := h.handleBasicAuthRouting(w, r, sep)
				if err != nil {
					return
				}
			}
			h.subdomainRequest(w, r, sep)
			return
		}
	}

	/*
		Virtual Directory Routing
	*/
	//Clean up the request URI
	proxyingPath := strings.TrimSpace(r.RequestURI)
	targetProxyEndpoint := h.Parent.getTargetProxyEndpointFromRequestURI(proxyingPath)
	if targetProxyEndpoint != nil {
		if targetProxyEndpoint.RequireBasicAuth {
			err := h.handleBasicAuthRouting(w, r, targetProxyEndpoint)
			if err != nil {
				return
			}
		}
		h.proxyRequest(w, r, targetProxyEndpoint)
	} else if !strings.HasSuffix(proxyingPath, "/") {
		potentialProxtEndpoint := h.Parent.getTargetProxyEndpointFromRequestURI(proxyingPath + "/")

		if potentialProxtEndpoint != nil {
			//Missing tailing slash. Redirect to target proxy endpoint
			http.Redirect(w, r, r.RequestURI+"/", http.StatusTemporaryRedirect)
		} else {
			//Passthrough the request to root
			h.proxyRequest(w, r, h.Parent.Root)
		}
	} else {
		//No routing rules found. Route to root.
		h.proxyRequest(w, r, h.Parent.Root)
	}
}
