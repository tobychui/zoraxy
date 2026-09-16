package dynamicproxy

import (
	"errors"
	"net/http"
	"regexp"

	"imuslab.com/zoraxy/mod/auth"
	"imuslab.com/zoraxy/mod/netutils"
	"imuslab.com/zoraxy/mod/pathmatch"
)

/*
	authProviders.go

	This script handle authentication providers
*/

/*
Central Authentication Provider Router

This function will route the request to the correct authentication provider
if the return value is true, do not continue to the next handler

handleAuthProviderRouting takes in 4 parameters:
- sep: the ProxyEndpoint object
- w: the http.ResponseWriter object
- r: the http.Request object
- h: the ProxyHandler object

and return a boolean indicate if the request is written to http.ResponseWriter
- true: the request is handled, do not write to http.ResponseWriter
- false: the request is not handled (usually means auth ok), continue to the next handler
*/
func handleAuthProviderRouting(sep *ProxyEndpoint, w http.ResponseWriter, r *http.Request, h *ProxyHandler) bool {
	requestHostname := r.Host

	switch sep.AuthenticationProvider.AuthMethod {
	case AuthMethodBasic:
		err := h.handleBasicAuthRouting(w, r, sep)
		if err != nil {
			h.Parent.Option.Logger.LogHTTPRequest(r, "host-http", 401, requestHostname, "")
			return true
		}
	case AuthMethodForward:
		err := h.handleForwardAuth(w, r)
		if err != nil {
			h.Parent.Option.Logger.LogHTTPRequest(r, "host-http", 401, requestHostname, "")
			return true
		}
	case AuthMethodOauth2:
		err := h.handleOAuth2Auth(w, r)
		if err != nil {
			h.Parent.Option.Logger.LogHTTPRequest(r, "host-http", 401, requestHostname, "")
			return true
		}
	case AuthMethodZorxAuth:
		err := h.handleZorxAuth(w, r, sep)
		if err != nil {
			h.Parent.Option.Logger.LogHTTPRequest(r, "host-http", 401, requestHostname, "")
			return true
		}
	}

	//No authentication provider, do not need to handle
	return false
}

/* Basic Auth */
func (h *ProxyHandler) handleBasicAuthRouting(w http.ResponseWriter, r *http.Request, pe *ProxyEndpoint) error {
	//Wrapper for oop style
	return handleBasicAuth(w, r, pe)
}

// basicAuthExceptionMatched reports whether the request is exempt from basic auth
// by one of the configured exception rules.
func basicAuthExceptionMatched(pe *ProxyEndpoint, r *http.Request) bool {
	if pe == nil || pe.AuthenticationProvider == nil {
		return false
	}
	requestTarget := pathmatch.RequestTarget(r)
	for _, exceptionRule := range pe.AuthenticationProvider.BasicAuthExceptionRules {
		if exceptionRule == nil {
			continue
		}
		exceptionType := exceptionRule.RuleType
		switch exceptionType {
		case AuthExceptionType_Paths:
			//Path matching is case sensitive unless the rule opts out of it
			matched := pathmatch.RequestPathWithinPrefix(requestTarget, exceptionRule.PathPrefix)
			if exceptionRule.CaseInsensitive {
				matched = pathmatch.RequestPathWithinPrefixFold(requestTarget, exceptionRule.PathPrefix)
			}
			if matched {
				return true
			}
		case AuthExceptionType_CIDR:
			// By default, use the untrusted (RemoteAddr-only) IP to prevent header-spoofing bypass.
			// Only trust proxy headers (X-Real-Ip, CF-Connecting-IP, etc.) when the rule
			// was explicitly configured with UseTrustedProxy = true.
			var requesterIp string
			if exceptionRule.UseTrustedProxy {
				requesterIp = netutils.GetRequesterIP(r)
			} else {
				requesterIp = netutils.GetRequesterIPUntrusted(r)
			}
			if requesterIp != "" {
				if requesterIp == exceptionRule.CIDR {
					// This IP is excluded from basic auth
					return true
				}

				wildcardMatch := netutils.MatchIpWildcard(requesterIp, exceptionRule.CIDR)
				if wildcardMatch {
					// This IP is excluded from basic auth
					return true
				}

				cidrMatch := netutils.MatchIpCIDR(requesterIp, exceptionRule.CIDR)
				if cidrMatch {
					// This IP is excluded from basic auth
					return true
				}
			}
		default:
			//Unknown exception type, skip this rule
			continue
		}
	}
	return false
}

// Handle basic auth logic
// do not write to http.ResponseWriter if err return is not nil (already handled by this function)
func handleBasicAuth(w http.ResponseWriter, r *http.Request, pe *ProxyEndpoint) error {
	if basicAuthExceptionMatched(pe, r) {
		return nil
	}

	u, p, ok := r.BasicAuth()
	if !ok {
		w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
		w.WriteHeader(401)
		w.Write([]byte("401 - Unauthorized"))
		return errors.New("unauthorized")
	}

	//Check for the credentials to see if there is one matching
	hashedPassword := auth.Hash(p)
	matchingFound := false
	for _, cred := range pe.AuthenticationProvider.BasicAuthCredentials {
		if u == cred.Username && hashedPassword == cred.PasswordHash {
			matchingFound = true

			//Set the X-Remote-User header
			r.Header.Set("X-Remote-User", u)
			break
		}
	}

	if !matchingFound {
		w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
		w.WriteHeader(401)
		w.Write([]byte("401 - Unauthorized"))
		return errors.New("unauthorized")
	}

	return nil
}

/* Forward Auth */
func (h *ProxyHandler) handleForwardAuth(w http.ResponseWriter, r *http.Request) error {
	// Skip forward auth for SSO-ignored paths. These paths are served WITHOUT authentication
	// (e.g. an auth provider callback subpath that must reach its own handler), so matching is
	// hardened against path traversal / boundary tricks (see forward.IsIgnoredPath).
	if h.Parent.Option.ForwardAuthRouter.IsIgnoredPath(r.RequestURI) {
		return nil
	}
	return h.Parent.Option.ForwardAuthRouter.HandleAuthProviderRouting(w, r)
}

/* OAuth2 Auth */
func (h *ProxyHandler) handleOAuth2Auth(w http.ResponseWriter, r *http.Request) error {
	return h.Parent.Option.OAuth2Router.HandleOAuth2Auth(w, r)
}

/* ZorxAuth */
func (h *ProxyHandler) handleZorxAuth(w http.ResponseWriter, r *http.Request, sep *ProxyEndpoint) error {
	// Check ZorxAuth exception rules before applying authentication
	if sep != nil && sep.AuthenticationProvider != nil {
		if zorxAuthExceptionMatched(sep.AuthenticationProvider.ZorxAuthExceptionRules, r) {
			return nil
		}
	}
	return h.Parent.Option.ZorxAuthAgentRouter.HandleAuthRouting(w, r)
}

// zorxAuthExceptionMatched reports whether the request is exempt from ZorxAuth by
// one of the configured exception rules. It takes the rule slice directly so it
// can be exercised without a configured SSO agent.
func zorxAuthExceptionMatched(rules []*ZorxAuthExceptionRule, r *http.Request) bool {
	requestTarget := pathmatch.RequestTarget(r)
	normalizedPath := pathmatch.CleanRequestPath(requestTarget)
	for _, rule := range rules {
		if rule == nil {
			continue
		}
		switch rule.RuleType {
		case AuthExceptionType_Paths:
			if rule.IsRegex {
				matched, err := regexp.MatchString(rule.PathPattern, normalizedPath)
				if err == nil && matched {
					return true
				}
			} else {
				if pathmatch.RequestPathWithinPrefix(requestTarget, rule.PathPattern) {
					return true
				}
			}
		case AuthExceptionType_CIDR:
			var requesterIp string
			if rule.UseTrustedProxy {
				requesterIp = netutils.GetRequesterIP(r)
			} else {
				requesterIp = netutils.GetRequesterIPUntrusted(r)
			}
			if requesterIp != "" {
				if requesterIp == rule.CIDR {
					return true
				}
				if netutils.MatchIpWildcard(requesterIp, rule.CIDR) {
					return true
				}
				if netutils.MatchIpCIDR(requesterIp, rule.CIDR) {
					return true
				}
			}
		}
	}
	return false
}
