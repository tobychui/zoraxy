package dynamicproxy

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
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
		Special Routing Rules, bypass most of the limitations
	*/

	//Check if there are external routing rule matches.
	//If yes, route them via external rr
	matchedRoutingRule := h.Parent.GetMatchingRoutingRule(r)
	if matchedRoutingRule != nil {
		//Matching routing rule found. Let the sub-router handle it
		if matchedRoutingRule.UseSystemAccessControl {
			//This matching rule request system access control.
			//check access logic
			respWritten := h.handleAccessRouting(w, r)
			if respWritten {
				return
			}
		}
		matchedRoutingRule.Route(w, r)
		return
	}

	//Inject headers
	w.Header().Set("x-proxy-by", "zoraxy/"+h.Parent.Option.HostVersion)

	/*
		General Access Check
	*/
	respWritten := h.handleAccessRouting(w, r)
	if respWritten {
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

	//Extract request host to see if it is virtual directory or subdomain
	domainOnly := r.Host
	if strings.Contains(r.Host, ":") {
		hostPath := strings.Split(r.Host, ":")
		domainOnly = hostPath[0]
	}

	/*
		Host Routing
	*/

	sep := h.Parent.getProxyEndpointFromHostname(domainOnly)
	if sep != nil && !sep.Disabled {
		if sep.RequireBasicAuth {
			err := h.handleBasicAuthRouting(w, r, sep)
			if err != nil {
				return
			}
		}

		//Check if any virtual directory rules matches
		proxyingPath := strings.TrimSpace(r.RequestURI)
		targetProxyEndpoint := sep.GetVirtualDirectoryHandlerFromRequestURI(proxyingPath)
		if targetProxyEndpoint != nil && !targetProxyEndpoint.Disabled {
			//Virtual directory routing rule found. Route via vdir mode
			h.vdirRequest(w, r, targetProxyEndpoint)
			return
		} else if !strings.HasSuffix(proxyingPath, "/") && sep.ProxyType != ProxyType_Root {
			potentialProxtEndpoint := sep.GetVirtualDirectoryHandlerFromRequestURI(proxyingPath + "/")
			if potentialProxtEndpoint != nil && !targetProxyEndpoint.Disabled {
				//Missing tailing slash. Redirect to target proxy endpoint
				http.Redirect(w, r, r.RequestURI+"/", http.StatusTemporaryRedirect)
				return
			}
		}

		//Fallback to handle by the host proxy forwarder
		h.hostRequest(w, r, sep)
		return
	}

	/*
		Root Router Handling
	*/
	//Clean up the request URI
	proxyingPath := strings.TrimSpace(r.RequestURI)
	if !strings.HasSuffix(proxyingPath, "/") {
		potentialProxtEndpoint := h.Parent.getTargetProxyEndpointFromRequestURI(proxyingPath + "/")
		if potentialProxtEndpoint != nil {
			//Missing tailing slash. Redirect to target proxy endpoint
			http.Redirect(w, r, r.RequestURI+"/", http.StatusTemporaryRedirect)
		} else {
			//Passthrough the request to root
			h.handleRootRouting(w, r)
		}
	} else {
		//No routing rules found.
		h.handleRootRouting(w, r)
	}
}

/*
handleRootRouting

This function handle root routing situations where there are no subdomain
, vdir or special routing rule matches the requested URI.

Once entered this routing segment, the root routing options will take over
for the routing logic.
*/
func (h *ProxyHandler) handleRootRouting(w http.ResponseWriter, r *http.Request) {

	domainOnly := r.Host
	if strings.Contains(r.Host, ":") {
		hostPath := strings.Split(r.Host, ":")
		domainOnly = hostPath[0]
	}

	//Get the proxy root config
	proot := h.Parent.Root
	switch proot.DefaultSiteOption {
	case DefaultSite_InternalStaticWebServer:
		fallthrough
	case DefaultSite_ReverseProxy:
		//They both share the same behavior

		//Check if any virtual directory rules matches
		proxyingPath := strings.TrimSpace(r.RequestURI)
		targetProxyEndpoint := proot.GetVirtualDirectoryHandlerFromRequestURI(proxyingPath)
		if targetProxyEndpoint != nil && !targetProxyEndpoint.Disabled {
			//Virtual directory routing rule found. Route via vdir mode
			h.vdirRequest(w, r, targetProxyEndpoint)
			return
		} else if !strings.HasSuffix(proxyingPath, "/") && proot.ProxyType != ProxyType_Root {
			potentialProxtEndpoint := proot.GetVirtualDirectoryHandlerFromRequestURI(proxyingPath + "/")
			if potentialProxtEndpoint != nil && !targetProxyEndpoint.Disabled {
				//Missing tailing slash. Redirect to target proxy endpoint
				http.Redirect(w, r, r.RequestURI+"/", http.StatusTemporaryRedirect)
				return
			}
		}

		//No vdir match. Route via root router
		h.hostRequest(w, r, h.Parent.Root)
	case DefaultSite_Redirect:
		redirectTarget := strings.TrimSpace(proot.DefaultSiteValue)
		if redirectTarget == "" {
			redirectTarget = "about:blank"
		}

		//Check if it is an infinite loopback redirect
		parsedURL, err := url.Parse(proot.DefaultSiteValue)
		if err != nil {
			//Error when parsing target. Send to root
			h.hostRequest(w, r, h.Parent.Root)
			return
		}
		hostname := parsedURL.Hostname()
		if hostname == domainOnly {
			h.logRequest(r, false, 500, "root-redirect", domainOnly)
			http.Error(w, "Loopback redirects due to invalid settings", 500)
			return
		}

		h.logRequest(r, false, 307, "root-redirect", domainOnly)
		http.Redirect(w, r, redirectTarget, http.StatusTemporaryRedirect)
	case DefaultSite_NotFoundPage:
		http.NotFound(w, r)
	}
}

// Handle access routing logic. Return true if the request is handled or blocked by the access control logic
// if the return value is false, you can continue process the response writer
func (h *ProxyHandler) handleAccessRouting(w http.ResponseWriter, r *http.Request) bool {
	//Check if this ip is in blacklist
	clientIpAddr := geodb.GetRequesterIP(r)
	if h.Parent.Option.GeodbStore.IsBlacklisted(clientIpAddr) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		template, err := os.ReadFile(filepath.Join(h.Parent.Option.WebDirectory, "templates/blacklist.html"))
		if err != nil {
			w.Write(page_forbidden)
		} else {
			w.Write(template)
		}
		h.logRequest(r, false, 403, "blacklist", "")
		return true
	}

	//Check if this ip is in whitelist
	if !h.Parent.Option.GeodbStore.IsWhitelisted(clientIpAddr) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		template, err := os.ReadFile(filepath.Join(h.Parent.Option.WebDirectory, "templates/whitelist.html"))
		if err != nil {
			w.Write(page_forbidden)
		} else {
			w.Write(template)
		}
		h.logRequest(r, false, 403, "whitelist", "")
		return true
	}

	return false
}
