package dynamicproxy

import (
	"errors"
	"net/http"
	"strings"

	"imuslab.com/zoraxy/mod/auth"
	ssooauth2 "imuslab.com/zoraxy/mod/auth/sso/oauth2"
	"imuslab.com/zoraxy/mod/netutils"
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
		err := h.handleOAuth2Auth(w, r, sep)
		if err != nil {
			statusCode := 401
			if errors.Is(err, ssooauth2.ErrForbidden) {
				statusCode = 403
			}
			h.Parent.Option.Logger.LogHTTPRequest(r, "host-http", statusCode, requestHostname, "")
			return true
		}
	case AuthMethodZorxAuth:
		err := h.handleZorxAuth(w, r)
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

// Handle basic auth logic
// do not write to http.ResponseWriter if err return is not nil (already handled by this function)
func handleBasicAuth(w http.ResponseWriter, r *http.Request, pe *ProxyEndpoint) error {
	if len(pe.AuthenticationProvider.BasicAuthExceptionRules) > 0 {
		//Check if the current path matches the exception rules
		for _, exceptionRule := range pe.AuthenticationProvider.BasicAuthExceptionRules {
			exceptionType := exceptionRule.RuleType
			switch exceptionType {
			case AuthExceptionType_Paths:
				if strings.HasPrefix(r.RequestURI, exceptionRule.PathPrefix) {
					//This path is excluded from basic auth
					return nil
				}
			case AuthExceptionType_CIDR:
				requesterIp := netutils.GetRequesterIP(r)
				if requesterIp != "" {
					if requesterIp == exceptionRule.CIDR {
						// This IP is excluded from basic auth
						return nil
					}

					wildcardMatch := netutils.MatchIpWildcard(requesterIp, exceptionRule.CIDR)
					if wildcardMatch {
						// This IP is excluded from basic auth
						return nil
					}

					cidrMatch := netutils.MatchIpCIDR(requesterIp, exceptionRule.CIDR)
					if cidrMatch {
						// This IP is excluded from basic auth
						return nil
					}
				}
			default:
				//Unknown exception type, skip this rule
				continue
			}
		}
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
	matchedGroups := []string{}
	for _, cred := range pe.AuthenticationProvider.BasicAuthCredentials {
		if u == cred.Username && hashedPassword == cred.PasswordHash {
			matchingFound = true

			//Set the X-Remote-User header
			r.Header.Set("X-Remote-User", u)
			break
		}
	}

	if !matchingFound {
		parentRouter := pe.parent
		if parentRouter != nil && parentRouter.Option != nil && parentRouter.Option.BasicAuthManager != nil {
			ok, groups := parentRouter.Option.BasicAuthManager.ValidateCredentials(u, p, pe.AuthenticationProvider.BasicAuthGroupIDs)
			if ok {
				matchingFound = true
				matchedGroups = groups
				r.Header.Set("X-Remote-User", u)
				if len(groups) > 0 {
					r.Header.Set("X-Remote-Groups", strings.Join(groups, ","))
				}
			}
		}
	}

	if !matchingFound {
		w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
		w.WriteHeader(401)
		w.Write([]byte("401 - Unauthorized"))
		return errors.New("unauthorized")
	}

	if len(matchedGroups) == 0 && len(pe.AuthenticationProvider.BasicAuthGroupIDs) > 0 {
		r.Header.Set("X-Remote-Groups", strings.Join(pe.AuthenticationProvider.BasicAuthGroupIDs, ","))
	}

	return nil
}

/* Forward Auth */

// Handle forward auth routing
func (h *ProxyHandler) handleForwardAuth(w http.ResponseWriter, r *http.Request) error {
	return h.Parent.Option.ForwardAuthRouter.HandleAuthProviderRouting(w, r)
}

func (h *ProxyHandler) handleOAuth2Auth(w http.ResponseWriter, r *http.Request, pe *ProxyEndpoint) error {
	tenantID := ""
	requiredClaims := []*ssooauth2.ClaimRequirement{}
	if pe != nil && pe.AuthenticationProvider != nil {
		tenantID = pe.AuthenticationProvider.OAuth2TenantID
		requiredClaims = pe.AuthenticationProvider.OAuth2RequiredClaims
	}
	return h.Parent.Option.OAuth2Router.HandleOAuth2Auth(w, r, tenantID, requiredClaims)
}

func (h *ProxyHandler) handleZorxAuth(w http.ResponseWriter, r *http.Request) error {
	return h.Parent.Option.ZorxAuthAgentRouter.HandleAuthRouting(w, r)
}
