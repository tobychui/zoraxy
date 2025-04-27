package dynamicproxy

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

/*
	Server.go

	Main server for dynamic proxy core

	Routing Handler Priority (High to Low)
	- Special Routing Rule (e.g. acme)
	- Redirectable
	- Subdomain Routing
		- Access Router
			- Blacklist
			- Whitelist
		- Rate Limitor
		- SSO Auth
		- Basic Auth
		- Plugin Router
		- Vitrual Directory Proxy
		- Subdomain Proxy
	- Root router (default site router)
*/

func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	/*
		Special Routing Rules, bypass most of the limitations
	*/
	//Check if there are external routing rule (rr) matches.
	//If yes, route them via external rr
	matchedRoutingRule := h.Parent.GetMatchingRoutingRule(r)
	if matchedRoutingRule != nil {
		//Matching routing rule found. Let the sub-router handle it
		matchedRoutingRule.Route(w, r)
		return
	}

	/*
		Redirection Routing
	*/
	//Check if this is a redirection url
	if h.Parent.Option.RedirectRuleTable.IsRedirectable(r) {
		statusCode := h.Parent.Option.RedirectRuleTable.HandleRedirect(w, r)
		h.Parent.logRequest(r, statusCode != 500, statusCode, "redirect", r.Host, "")
		return
	}

	/*
		Host Routing
	*/
	//Extract request host to see if any proxy rule is matched
	domainOnly := r.Host
	if strings.Contains(r.Host, ":") {
		hostPath := strings.Split(r.Host, ":")
		domainOnly = hostPath[0]
	}
	sep := h.Parent.getProxyEndpointFromHostname(domainOnly)
	if sep != nil && !sep.Disabled {
		//Matching proxy rule found
		//Access Check (blacklist / whitelist)
		ruleID := sep.AccessFilterUUID
		if sep.AccessFilterUUID == "" {
			//Use default rule
			ruleID = "default"
		}
		if h.handleAccessRouting(ruleID, w, r) {
			//Request handled by subroute
			return
		}

		// Rate Limit
		if sep.RequireRateLimit {
			err := h.handleRateLimitRouting(w, r, sep)
			if err != nil {
				h.Parent.Option.Logger.LogHTTPRequest(r, "host", 307, r.Host, "")
				return
			}
		}

		//Validate auth (basic auth or SSO auth)
		respWritten := handleAuthProviderRouting(sep, w, r, h)
		if respWritten {
			//Request handled by subroute
			return
		}

		//Plugin routing

		if h.Parent.Option.PluginManager != nil && h.Parent.Option.PluginManager.HandleRoute(w, r, sep.Tags) {
			//Request handled by subroute
			return
		}

		//Check if any virtual directory rules matches
		proxyingPath := strings.TrimSpace(r.RequestURI)
		targetProxyEndpoint := sep.GetVirtualDirectoryHandlerFromRequestURI(proxyingPath)
		if targetProxyEndpoint != nil && !targetProxyEndpoint.Disabled {
			//Virtual directory routing rule found. Route via vdir mode
			h.vdirRequest(w, r, targetProxyEndpoint)
			return
		} else if !strings.HasSuffix(proxyingPath, "/") && sep.ProxyType != ProxyTypeRoot {
			potentialProxtEndpoint := sep.GetVirtualDirectoryHandlerFromRequestURI(proxyingPath + "/")
			if potentialProxtEndpoint != nil && !potentialProxtEndpoint.Disabled {
				//Missing tailing slash. Redirect to target proxy endpoint
				http.Redirect(w, r, r.RequestURI+"/", http.StatusTemporaryRedirect)
				h.Parent.Option.Logger.LogHTTPRequest(r, "redirect", 307, r.Host, "")
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

	//Root access control based on default rule
	blocked := h.handleAccessRouting("default", w, r)
	if blocked {
		return
	}

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

This function handle root routing (aka default sites) situations where there are no subdomain
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
		} else if !strings.HasSuffix(proxyingPath, "/") && proot.ProxyType != ProxyTypeRoot {
			potentialProxtEndpoint := proot.GetVirtualDirectoryHandlerFromRequestURI(proxyingPath + "/")
			if potentialProxtEndpoint != nil && !targetProxyEndpoint.Disabled {
				//Missing tailing slash. Redirect to target proxy endpoint
				http.Redirect(w, r, r.RequestURI+"/", http.StatusTemporaryRedirect)
				return
			}
		}

		//Do not log default site requests to avoid flooding the logs
		//h.Parent.logRequest(r, false, 307, "root", domainOnly, "")

		//No vdir match. Route via root router
		h.hostRequest(w, r, h.Parent.Root)
	case DefaultSite_Redirect:
		redirectTarget := strings.TrimSpace(proot.DefaultSiteValue)
		if redirectTarget == "" {
			redirectTarget = "about:blank"
		}

		//Check if the default site values start with http or https
		if !strings.HasPrefix(redirectTarget, "http://") && !strings.HasPrefix(redirectTarget, "https://") {
			redirectTarget = "http://" + redirectTarget
		}

		//Check if it is an infinite loopback redirect
		parsedURL, err := url.Parse(redirectTarget)
		if err != nil {
			//Error when parsing target. Send to root
			h.hostRequest(w, r, h.Parent.Root)
			return
		}
		hostname := parsedURL.Hostname()
		if hostname == domainOnly {
			h.Parent.logRequest(r, false, 500, "root-redirect", domainOnly, "")
			http.Error(w, "Loopback redirects due to invalid settings", 500)
			return
		}

		h.Parent.logRequest(r, false, 307, "root-redirect", domainOnly, "")
		http.Redirect(w, r, redirectTarget, http.StatusTemporaryRedirect)
	case DefaultSite_NotFoundPage:
		//Serve the not found page, use template if exists
		h.serve404PageWithTemplate(w, r)
	case DefaultSite_NoResponse:
		//No response. Just close the connection
		h.Parent.logRequest(r, false, 444, "root-no_resp", domainOnly, "")
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		conn, _, err := hijacker.Hijack()
		if err != nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		conn.Close()
	case DefaultSite_TeaPot:
		//I'm a teapot
		h.Parent.logRequest(r, false, 418, "root-teapot", domainOnly, "")
		http.Error(w, "I'm a teapot", http.StatusTeapot)
	default:
		//Unknown routing option. Send empty response
		h.Parent.logRequest(r, false, 544, "root-unknown", domainOnly, "")
		http.Error(w, "544 - No Route Defined", 544)
	}
}

// Serve 404 page with template if exists
func (h *ProxyHandler) serve404PageWithTemplate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	template, err := os.ReadFile(filepath.Join(h.Parent.Option.WebDirectory, "templates/notfound.html"))
	if err != nil {
		w.Write(page_hosterror)
	} else {
		w.Write(template)
	}
}
