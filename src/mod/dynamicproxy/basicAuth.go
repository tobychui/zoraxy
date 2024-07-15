package dynamicproxy

import (
	"errors"
	"net/http"
	"strings"

	"imuslab.com/zoraxy/mod/auth"
)

/*
	BasicAuth.go

	This file handles the basic auth on proxy endpoints
	if RequireBasicAuth is set to true
*/

func (h *ProxyHandler) handleBasicAuthRouting(w http.ResponseWriter, r *http.Request, pe *ProxyEndpoint) error {
	err := handleBasicAuth(w, r, pe)
	if err != nil {
		h.Parent.logRequest(r, false, 401, "host", r.URL.Hostname())
	}
	return err
}

// Handle basic auth logic
// do not write to http.ResponseWriter if err return is not nil (already handled by this function)
func handleBasicAuth(w http.ResponseWriter, r *http.Request, pe *ProxyEndpoint) error {
	if len(pe.BasicAuthExceptionRules) > 0 {
		//Check if the current path matches the exception rules
		for _, exceptionRule := range pe.BasicAuthExceptionRules {
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
		return errors.New("unauthorized")
	}

	//Check for the credentials to see if there is one matching
	hashedPassword := auth.Hash(p)
	matchingFound := false
	for _, cred := range pe.BasicAuthCredentials {
		if u == cred.Username && hashedPassword == cred.PasswordHash {
			matchingFound = true
			break
		}
	}

	if !matchingFound {
		w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
		w.WriteHeader(401)
		return errors.New("unauthorized")
	}

	return nil
}
