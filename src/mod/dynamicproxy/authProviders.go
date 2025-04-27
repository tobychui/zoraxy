package dynamicproxy

import (
	"errors"
	"net/http"
	"strings"

	"imuslab.com/zoraxy/mod/auth"
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
	if sep.AuthenticationProvider.AuthMethod == AuthMethodBasic {
		err := h.handleBasicAuthRouting(w, r, sep)
		if err != nil {
			h.Parent.Option.Logger.LogHTTPRequest(r, "host-http", 401, requestHostname, "")
			return true
		}
	} else if sep.AuthenticationProvider.AuthMethod == AuthMethodAuthelia {
		err := h.handleAutheliaAuth(w, r)
		if err != nil {
			h.Parent.Option.Logger.LogHTTPRequest(r, "host-http", 401, requestHostname, "")
			return true
		}
	} else if sep.AuthenticationProvider.AuthMethod == AuthMethodAuthentik {
		err := h.handleAuthentikAuth(w, r)
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
			if strings.HasPrefix(r.RequestURI, exceptionRule.PathPrefix) {
				//This path is excluded from basic auth
				return nil
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

/* Authelia */

// Handle authelia auth routing
func (h *ProxyHandler) handleAutheliaAuth(w http.ResponseWriter, r *http.Request) error {
	return h.Parent.Option.AutheliaRouter.HandleAutheliaAuth(w, r)
}

func (h *ProxyHandler) handleAuthentikAuth(w http.ResponseWriter, r *http.Request) error {
	return h.Parent.Option.AuthentikRouter.HandleAuthentikAuth(w, r)
}
