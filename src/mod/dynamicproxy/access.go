package dynamicproxy

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"imuslab.com/zoraxy/mod/access"
	"imuslab.com/zoraxy/mod/eventsystem"
	"imuslab.com/zoraxy/mod/netutils"
	"imuslab.com/zoraxy/mod/plugins/zoraxy_plugin/events"
)

// Handle access check (blacklist / whitelist), return true if request is handled (aka blocked)
// if the return value is false, you can continue process the response writer
func (h *ProxyHandler) handleAccessRouting(ruleID string, w http.ResponseWriter, r *http.Request, sep *ProxyEndpoint) bool {
	accessRule, err := h.Parent.Option.AccessController.GetAccessRuleByID(ruleID)
	if err != nil {
		//Unable to load access rule. Target rule not found?
		h.Parent.Option.Logger.PrintAndLog("proxy-access", "Unable to load access rule: "+ruleID, err)
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("500 - Internal Server Error"))
		return true
	}

	isBlocked, blockedReason := accessRequestBlocked(accessRule, h.Parent.Option.WebDirectory, w, r)
	if isBlocked {
		h.Parent.logRequest(r, false, 403, blockedReason, r.Host, "", sep)
		return true
	}

	if isBlocked, blockedReason = accessRequestBlockedByEndpointWhitelist(accessRule, h.Parent.Option.WebDirectory, w, r, sep); isBlocked {
		h.Parent.logRequest(r, false, 403, blockedReason, r.Host, "", sep)
		return true
	}

	return false
}

// Return boolean, return true if access is blocked
// For string, it will return the blocked reason (if any)
func accessRequestBlocked(accessRule *access.AccessRule, templateDirectory string, w http.ResponseWriter, r *http.Request) (bool, string) {
	//Check if this ip is in blacklist
	clientIpAddr := accessRule.GetClientIP(r)
	if accessRule.IsBlacklisted(clientIpAddr) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		template, err := os.ReadFile(filepath.Join(templateDirectory, "templates/blacklist.html"))
		if err != nil {
			w.Write(page_forbidden)
		} else {
			w.Write(template)
		}

		// Emit blacklisted IP blocked event
		// Get the comment for this IP
		comment, err := accessRule.GetBlacklistedIPComment(clientIpAddr)
		if err != nil {
			comment = "blacklisted"
		}
		eventsystem.Publisher.Emit(
			&events.BlacklistedIPBlockedEvent{
				IP:           clientIpAddr,
				Comment:      comment,
				RequestedURL: r.URL.String(),
				Hostname:     r.Host,
				UserAgent:    r.Header.Get("User-Agent"),
				Method:       r.Method,
			},
		)

		return true, "blacklist"
	}

	//Check if this ip is in whitelist
	if !accessRule.IsWhitelisted(clientIpAddr) {
		writeWhitelistResponse(templateDirectory, w)
		return true, "whitelist"
	}

	//Not blocked.
	return false, ""
}

func accessRequestBlockedByEndpointWhitelist(accessRule *access.AccessRule, templateDirectory string, w http.ResponseWriter, r *http.Request, sep *ProxyEndpoint) (bool, string) {
	if sep == nil || !sep.RequireIPWhitelist || len(sep.IPWhitelistSubnets) == 0 {
		return false, ""
	}

	clientIpAddr := accessRule.GetClientIP(r)
	if !ipAllowedByWhitelist(clientIpAddr, sep.IPWhitelistSubnets) {
		writeWhitelistResponse(templateDirectory, w)
		return true, "ip-whitelist"
	}

	return false, ""
}

func ipAllowedByWhitelist(clientIP string, whitelist []string) bool {
	clientIP = strings.TrimSpace(clientIP)
	if clientIP == "" {
		return false
	}

	clientAddr := net.ParseIP(clientIP)
	for _, entry := range whitelist {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		if strings.Contains(entry, "/") {
			if netutils.MatchIpCIDR(clientIP, entry) {
				return true
			}
			continue
		}

		entryAddr := net.ParseIP(entry)
		if clientAddr != nil && entryAddr != nil && clientAddr.Equal(entryAddr) {
			return true
		}
	}

	return false
}

func writeWhitelistResponse(templateDirectory string, w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	template, err := os.ReadFile(filepath.Join(templateDirectory, "templates/whitelist.html"))
	if err != nil {
		w.Write(page_forbidden)
		return
	}
	w.Write(template)
}

// GetClientIPForEndpoint resolves the real client IP of a request using the
// access rule bounded to the given proxy endpoint. Header supplied addresses
// (X-Real-IP, CF-Connecting-IP, Fastly-Client-IP, X-Forwarded-For) are only
// honored when the access rule opted in to trusting proxy headers
func (router *Router) GetClientIPForEndpoint(r *http.Request, pe *ProxyEndpoint) string {
	ruleID := "default"
	if pe != nil && pe.AccessFilterUUID != "" {
		ruleID = pe.AccessFilterUUID
	}

	if router.Option != nil && router.Option.AccessController != nil {
		accessRule, err := router.Option.AccessController.GetAccessRuleByID(ruleID)
		if err == nil {
			if clientIP := accessRule.GetClientIP(r); clientIP != "" {
				return clientIP
			}
		}
	}

	//Access rule not available. Fallback to the connection address only
	return netutils.GetRequesterIPUntrusted(r)
}
