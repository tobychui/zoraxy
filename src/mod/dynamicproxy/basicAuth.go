package dynamicproxy

import (
	"errors"
	"net/http"

	"imuslab.com/zoraxy/mod/auth"
)

/*
	BasicAuth.go

	This file handles the basic auth on proxy endpoints
	if RequireBasicAuth is set to true
*/

func (h *ProxyHandler) handleBasicAuthRouting(w http.ResponseWriter, r *http.Request, pe *ProxyEndpoint) error {
	proxyType := "vdir-auth"
	if pe.ProxyType == ProxyType_Subdomain {
		proxyType = "subd-auth"
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
		h.logRequest(r, false, 401, proxyType, pe.Domain)
		w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
		w.WriteHeader(401)
		return errors.New("unauthorized")
	}

	return nil
}
