package dynamicproxy

import (
	_ "embed"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
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

var (
	//go:embed tld.json
	rawTldMap []byte
)

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
			if h.handleAccessRouting(w, r) {
				return
			}
		}
		matchedRoutingRule.Route(w, r)
		return
	}

	/*
		General Access Check
	*/

	if h.handleAccessRouting(w, r) {
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
			if err := h.handleBasicAuthRouting(w, r, targetProxyEndpoint); err != nil {
				return
			}
		}
		h.proxyRequest(w, r, targetProxyEndpoint)
	} else if !strings.HasSuffix(proxyingPath, "/") {
		if potentialProxtEndpoint := h.Parent.getTargetProxyEndpointFromRequestURI(fmt.Sprintf("%s/", proxyingPath)); potentialProxtEndpoint != nil {
			//Missing tailing slash. Redirect to target proxy endpoint
			http.Redirect(w, r, fmt.Sprintf("%s/", r.RequestURI), http.StatusTemporaryRedirect)
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

	if h.Parent.RootRoutingOptions.EnableRedirectForUnsetRules {
		//Route to custom domain
		if h.Parent.RootRoutingOptions.UnsetRuleRedirectTarget == "" {
			//Not set. Redirect to first level of domain redirectable
			fld, err := h.getTopLevelRedirectableDomain(domainOnly)
			if err != nil {
				//Redirect to proxy root
				h.proxyRequest(w, r, h.Parent.Root)
			} else {
				log.Printf("[Router] Redirecting request from %s to %s", domainOnly, fld)
				h.logRequest(r, false, 307, "root-redirect", domainOnly)
				http.Redirect(w, r, fld, http.StatusTemporaryRedirect)
			}
			return
		} else if h.isTopLevelRedirectableDomain(domainOnly) {
			//This is requesting a top level private domain that should be serving root
			h.proxyRequest(w, r, h.Parent.Root)
		} else {
			//Validate the redirection target URL
			parsedURL, err := url.Parse(h.Parent.RootRoutingOptions.UnsetRuleRedirectTarget)
			if err != nil {
				//Error when parsing target. Send to root
				h.proxyRequest(w, r, h.Parent.Root)
				return
			}
			hostname := parsedURL.Hostname()
			if domainOnly != hostname {
				//Redirect to target
				h.logRequest(r, false, 307, "root-redirect", domainOnly)
				http.Redirect(w, r, h.Parent.RootRoutingOptions.UnsetRuleRedirectTarget, http.StatusTemporaryRedirect)
				return
			} else {
				//Loopback request due to bad settings (Shd leave it empty)
				//Forward it to root proxy
				h.proxyRequest(w, r, h.Parent.Root)
			}
		}
	} else {
		//Route to root
		h.proxyRequest(w, r, h.Parent.Root)
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
		template, err := os.ReadFile("./web/forbidden.html")
		if err != nil {
			w.Write([]byte("403 - Forbidden"))
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
		template, err := os.ReadFile("./web/forbidden.html")
		if err != nil {
			w.Write([]byte("403 - Forbidden"))
		} else {
			w.Write(template)
		}
		h.logRequest(r, false, 403, "whitelist", "")
		return true
	}

	return false
}

// Return if the given host is already topped (e.g. example.com or example.co.uk) instead of
// a host with subdomain (e.g. test.example.com)
func (h *ProxyHandler) isTopLevelRedirectableDomain(requestHost string) bool {
	parts := strings.Split(requestHost, ".")
	if len(parts) > 2 {
		//Cases where strange tld is used like .co.uk or .com.hk
		_, ok := h.Parent.tldMap[strings.Join(parts[1:], ".")]
		if ok {
			//Already topped
			return true
		}
	} else {
		//Already topped
		return true
	}

	return false
}

// GetTopLevelRedirectableDomain returns the toppest level of domain
// that is redirectable. E.g. a.b.c.example.co.uk will return example.co.uk
func (h *ProxyHandler) getTopLevelRedirectableDomain(unsetSubdomainHost string) (string, error) {
	parts := strings.Split(unsetSubdomainHost, ".")
	if h.isTopLevelRedirectableDomain(unsetSubdomainHost) {
		//Already topped
		return "", errors.New("already at top level domain")
	}

	for i := 0; i < len(parts); i++ {
		possibleTld := parts[i:]
		_, ok := h.Parent.tldMap[strings.Join(possibleTld, ".")]
		if ok {
			//This is tld length
			tld := strings.Join(parts[i-1:], ".")
			return "//" + tld, nil
		}
	}

	return "", errors.New("unsupported top level domain given")
}
