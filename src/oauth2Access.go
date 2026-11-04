package main

import (
    "encoding/json"
    "net"
    "net/http"
    "strings"

    oauthaccess "imuslab.com/zoraxy/mod/auth/sso/oauth2"
    "imuslab.com/zoraxy/mod/dynamicproxy"
    "imuslab.com/zoraxy/mod/utils"
)

type oauth2RouteSettings struct {
    AllowedGroups                  string                             `json:"allowedGroups"`
    ExceptionRules                []*dynamicproxy.BasicAuthExceptionRule `json:"exceptionRules"`
    ForwardUserInfo               bool                               `json:"forwardUserInfo"`
    RemoveUntrustedIdentityHeaders bool                              `json:"removeUntrustedIdentityHeaders"`
    ForwardWebSocketIdentity      bool                               `json:"forwardWebSocketIdentity"`
    IncludeTokenCookie            bool                               `json:"includeTokenCookie"`
    ForwardedScopes               string                             `json:"forwardedScopes"`
}

// HandleProxyOAuth2Settings edits only the OAuth2 configuration for one host.
// Existing authentication fields and the other provider configurations are preserved.
func HandleProxyOAuth2Settings(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet && r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
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
    if proxy.AuthenticationProvider == nil {
        proxy.AuthenticationProvider = dynamicproxy.GetDefaultAuthenticationProvider()
    }
    provider := proxy.AuthenticationProvider
    if r.Method == http.MethodGet {
        scopes := provider.OAuth2ForwardedScopes
        if scopes == nil {
            scopes = oauthaccess.ScopesForClaims(provider.OAuth2ForwardedClaims)
        }
        rules := provider.OAuth2ExceptionRules
        if rules == nil {
            rules = []*dynamicproxy.BasicAuthExceptionRule{}
        }
        response := oauth2RouteSettings{
            AllowedGroups: strings.Join(provider.OAuth2AllowedGroups, ", "),
            ExceptionRules: rules,
            ForwardUserInfo: !provider.OAuth2DisableIdentityForwarding,
            RemoveUntrustedIdentityHeaders: !provider.OAuth2AllowUntrustedIdentityHeaders || !provider.OAuth2DisableIdentityForwarding,
            ForwardWebSocketIdentity: provider.OAuth2ForwardWebSocketIdentity == nil || *provider.OAuth2ForwardWebSocketIdentity,
            IncludeTokenCookie: provider.OAuth2DisableTokenCookieRemoval,
            ForwardedScopes: strings.Join(scopes, ", "),
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
    scopes, err := oauthaccess.ParseForwardedScopes(input.ForwardedScopes)
    if err != nil {
        utils.SendErrorResponse(w, err.Error())
        return
    }

    provider.OAuth2AllowedGroups = oauthaccess.SplitAllowedGroups(input.AllowedGroups)
    provider.OAuth2ExceptionRules = input.ExceptionRules
    provider.OAuth2DisableIdentityForwarding = !input.ForwardUserInfo
    provider.OAuth2AllowUntrustedIdentityHeaders = !input.RemoveUntrustedIdentityHeaders && !input.ForwardUserInfo
    provider.OAuth2ForwardWebSocketIdentity = &input.ForwardWebSocketIdentity
    provider.OAuth2DisableTokenCookieRemoval = input.IncludeTokenCookie
    provider.OAuth2ForwardedScopes = scopes
    proxy.UpdateToRuntime()
    SaveReverseProxyConfig(proxy)
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
