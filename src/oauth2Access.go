package main

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"

	oauthaccess "imuslab.com/zoraxy/mod/auth/sso/oauth2"
	"imuslab.com/zoraxy/mod/dynamicproxy"
	"imuslab.com/zoraxy/mod/utils"
)

var oauth2RouteSettingsMu sync.Mutex

type oauth2RouteSettings struct {
	AllowedGroups                  string                                 `json:"allowedGroups"`
	ExceptionRules                 []*dynamicproxy.BasicAuthExceptionRule `json:"exceptionRules"`
	ForwardUserInfo                bool                                   `json:"forwardUserInfo"`
	RemoveUntrustedIdentityHeaders bool                                   `json:"removeUntrustedIdentityHeaders"`
	ForwardWebSocketIdentity       bool                                   `json:"forwardWebSocketIdentity"`
	IncludeTokenCookie             bool                                   `json:"includeTokenCookie"`
}

// updateProxyOAuth2Settings persists and then publishes a complete immutable
// endpoint replacement. Runtime request handlers can therefore never observe a
// provider while its OAuth2 policy is being changed field by field.
func updateProxyOAuth2Settings(proxy *dynamicproxy.ProxyEndpoint, input oauth2RouteSettings, save func(*dynamicproxy.ProxyEndpoint) error, publish func(*dynamicproxy.ProxyEndpoint)) error {
	if proxy == nil || save == nil || publish == nil {
		return errors.New("invalid OAuth2 settings update")
	}

	replacement := *proxy
	provider := proxy.AuthenticationProvider
	if provider == nil {
		provider = dynamicproxy.GetDefaultAuthenticationProvider()
	}
	providerReplacement := *provider

	// The input was decoded into request-owned values, but copy the rules again
	// so neither caller nor UI payload can mutate the published policy later.
	rules := make([]*dynamicproxy.BasicAuthExceptionRule, len(input.ExceptionRules))
	for i, rule := range input.ExceptionRules {
		if rule != nil {
			ruleReplacement := *rule
			rules[i] = &ruleReplacement
		}
	}
	forwardWebSocketIdentity := input.ForwardWebSocketIdentity
	providerReplacement.OAuth2AllowedGroups = append([]string(nil), oauthaccess.SplitAllowedGroups(input.AllowedGroups)...)
	providerReplacement.OAuth2ExceptionRules = rules
	providerReplacement.OAuth2DisableIdentityForwarding = !input.ForwardUserInfo
	providerReplacement.OAuth2AllowUntrustedIdentityHeaders = !input.RemoveUntrustedIdentityHeaders && !input.ForwardUserInfo
	providerReplacement.OAuth2ForwardWebSocketIdentity = &forwardWebSocketIdentity
	providerReplacement.OAuth2DisableTokenCookieRemoval = input.IncludeTokenCookie
	replacement.AuthenticationProvider = &providerReplacement

	// Persist first. A failed write must not create a runtime-only policy that
	// silently disappears on restart.
	if err := save(&replacement); err != nil {
		return err
	}
	publish(&replacement)
	return nil
}

// HandleProxyOAuth2Settings edits only the OAuth2 configuration for one host.
// Existing authentication fields and the other provider configurations are preserved.
func HandleProxyOAuth2Settings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Method == http.MethodPost {
		// Serialize administrative writes so two editors cannot persist and
		// publish replacements derived from the same stale endpoint snapshot.
		oauth2RouteSettingsMu.Lock()
		defer oauth2RouteSettingsMu.Unlock()
	}
	ep := strings.TrimSpace(r.FormValue("ep"))
	if ep == "" {
		utils.SendErrorResponse(w, "Missing proxy endpoint")
		return
	}
	proxy, err := dynamicProxyRouter.LoadProxy(ep)
	if err != nil || proxy == nil {
		utils.SendErrorResponse(w, "Proxy endpoint not found")
		return
	}
	provider := proxy.AuthenticationProvider
	if provider == nil {
		provider = dynamicproxy.GetDefaultAuthenticationProvider()
	}
	if r.Method == http.MethodGet {
		rules := provider.OAuth2ExceptionRules
		if rules == nil {
			rules = []*dynamicproxy.BasicAuthExceptionRule{}
		}
		response := oauth2RouteSettings{
			AllowedGroups:                  strings.Join(provider.OAuth2AllowedGroups, ", "),
			ExceptionRules:                 rules,
			ForwardUserInfo:                !provider.OAuth2DisableIdentityForwarding,
			RemoveUntrustedIdentityHeaders: !provider.OAuth2AllowUntrustedIdentityHeaders || !provider.OAuth2DisableIdentityForwarding,
			ForwardWebSocketIdentity:       provider.OAuth2ForwardWebSocketIdentity == nil || *provider.OAuth2ForwardWebSocketIdentity,
			IncludeTokenCookie:             provider.OAuth2DisableTokenCookieRemoval,
		}
		data, _ := json.Marshal(response)
		utils.SendJSONResponse(w, string(data))
		return
	}

	var input oauth2RouteSettings
	raw := r.FormValue("settings")
	if raw == "" || len(raw) > 128*1024 {
		utils.SendErrorResponse(w, "Invalid OAuth2 settings")
		return
	}
	if err := json.Unmarshal([]byte(raw), &input); err != nil {
		utils.SendErrorResponse(w, "Invalid OAuth2 settings JSON")
		return
	}
	if len(input.ExceptionRules) > 100 {
		utils.SendErrorResponse(w, "Too many exception rules")
		return
	}
	for _, rule := range input.ExceptionRules {
		if rule == nil {
			utils.SendErrorResponse(w, "Invalid empty exception rule")
			return
		}
		switch rule.RuleType {
		case dynamicproxy.AuthExceptionType_Paths:
			rule.PathPrefix = strings.TrimSpace(rule.PathPrefix)
			if !strings.HasPrefix(rule.PathPrefix, "/") {
				utils.SendErrorResponse(w, "Exception path must begin with /")
				return
			}
			rule.CIDR = ""
		case dynamicproxy.AuthExceptionType_CIDR:
			rule.CIDR = strings.TrimSpace(rule.CIDR)
			if !validOAuth2ExceptionIP(rule.CIDR) {
				utils.SendErrorResponse(w, "Invalid exception IP or CIDR")
				return
			}
			rule.PathPrefix = ""
		default:
			utils.SendErrorResponse(w, "Unknown exception type")
			return
		}
	}
	if err := updateProxyOAuth2Settings(proxy, input, SaveReverseProxyConfig, func(replacement *dynamicproxy.ProxyEndpoint) {
		replacement.UpdateToRuntime()
	}); err != nil {
		utils.SendErrorResponse(w, "Failed to save OAuth2 settings")
		return
	}
	utils.SendOK(w)
}

func validOAuth2ExceptionIP(s string) bool {
	if net.ParseIP(s) != nil {
		return true
	}
	if _, _, err := net.ParseCIDR(s); err == nil {
		return true
	}
	parts := strings.Split(s, ".")
	if len(parts) != 4 || parts[3] != "*" {
		return false
	}
	for _, p := range parts[:3] {
		if p == "" || len(p) > 3 {
			return false
		}
		n := 0
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
			n = n*10 + int(c-'0')
		}
		if n > 255 {
			return false
		}
	}
	return true
}
