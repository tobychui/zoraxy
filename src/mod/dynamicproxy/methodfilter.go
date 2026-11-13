package dynamicproxy

import (
	"errors"
	"net/http"
	"sort"
	"strings"

	"imuslab.com/zoraxy/mod/dynamicproxy/errorpage"
)

/*
	methodfilter.go

	Per-endpoint allow-list of HTTP methods.

	When an endpoint lists its allowed methods, a request made with
	any other method is answered with 405 Method Not Allowed and is
	never forwarded to the upstream or to a virtual directory. An
	endpoint with an empty list accepts every method.
*/

// ParseAllowedMethods converts a list of HTTP methods separated by comma or
// whitespace (e.g. "GET, HEAD, POST") into a sorted list without duplicates.
// Methods are converted to upper case. An empty input returns an empty list,
// which means all methods are allowed.
func ParseAllowedMethods(raw string) ([]string, error) {
	methods := []string{}
	seen := map[string]bool{}
	for _, method := range strings.FieldsFunc(raw, func(c rune) bool {
		return c == ',' || c == ' ' || c == '\t' || c == '\n' || c == '\r'
	}) {
		method = strings.ToUpper(method)
		if !isValidHTTPMethod(method) {
			return nil, errors.New("invalid HTTP method: " + method)
		}
		if !seen[method] {
			seen[method] = true
			methods = append(methods, method)
		}
	}
	sort.Strings(methods)
	return methods, nil
}

// isValidHTTPMethod checks if the given string is an upper case HTTP method
// token as defined in RFC 9110 section 5.6.2
func isValidHTTPMethod(method string) bool {
	if method == "" || len(method) > 32 {
		return false
	}
	for _, c := range method {
		switch {
		case c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case strings.ContainsRune("!#$%&'*+-.^_`|~", c):
		default:
			return false
		}
	}
	return true
}

// IsMethodAllowed checks if a request with the given method can be served by
// this endpoint. Methods are case-sensitive (RFC 9110 section 9.1).
func (ep *ProxyEndpoint) IsMethodAllowed(method string) bool {
	if len(ep.AllowedMethods) == 0 {
		//No allow-list set, all methods are allowed
		return true
	}
	for _, allowedMethod := range ep.AllowedMethods {
		if allowedMethod == method {
			return true
		}
	}
	return false
}

// handleMethodNotAllowed answers a request whose method is not in the
// allow-list of the endpoint
func (h *ProxyHandler) handleMethodNotAllowed(w http.ResponseWriter, r *http.Request, pe *ProxyEndpoint) {
	w.Header().Set("Allow", strings.Join(pe.AllowedMethods, ", "))
	h.Parent.errorPages().ServeErrorPage(w, errorpage.ActionMethodNotAllowed, http.StatusMethodNotAllowed)
	h.Parent.logRequest(r, false, http.StatusMethodNotAllowed, "method-blocked", r.Host, "blocked", pe)
}
