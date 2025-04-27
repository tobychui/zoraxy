package dynamicproxy

import (
	"net/http"
	"os"
	"path/filepath"

	"imuslab.com/zoraxy/mod/access"
	"imuslab.com/zoraxy/mod/netutils"
)

// Handle access check (blacklist / whitelist), return true if request is handled (aka blocked)
// if the return value is false, you can continue process the response writer
func (h *ProxyHandler) handleAccessRouting(ruleID string, w http.ResponseWriter, r *http.Request) bool {
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
		h.Parent.logRequest(r, false, 403, blockedReason, r.Host, "")
	}
	return isBlocked
}

// Return boolean, return true if access is blocked
// For string, it will return the blocked reason (if any)
func accessRequestBlocked(accessRule *access.AccessRule, templateDirectory string, w http.ResponseWriter, r *http.Request) (bool, string) {
	//Check if this ip is in blacklist
	clientIpAddr := netutils.GetRequesterIP(r)
	if accessRule.IsBlacklisted(clientIpAddr) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		template, err := os.ReadFile(filepath.Join(templateDirectory, "templates/blacklist.html"))
		if err != nil {
			w.Write(page_forbidden)
		} else {
			w.Write(template)
		}

		return true, "blacklist"
	}

	//Check if this ip is in whitelist
	if !accessRule.IsWhitelisted(clientIpAddr) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		template, err := os.ReadFile(filepath.Join(templateDirectory, "templates/whitelist.html"))
		if err != nil {
			w.Write(page_forbidden)
		} else {
			w.Write(template)
		}
		return true, "whitelist"
	}

	//Not blocked.
	return false, ""
}
