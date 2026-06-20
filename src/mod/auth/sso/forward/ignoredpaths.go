package forward

import (
	"net/url"
	"path"
	"strings"
)

/*
	ignoredpaths.go

	Implements "Ignored Paths" for forward auth: a list of request path prefixes that bypass
	the forward auth check entirely. Requests whose path falls within one of these prefixes are
	served WITHOUT authentication.

	The primary use case is auth-provider callback subpaths that are served on the protected
	application's own domain and therefore must not be gated by the auth check, e.g. Authentik's
	/outpost.goauthentik.io in single-application mode (see issue #895). It is intentionally
	generic: any path the operator explicitly wants public can be listed.
*/

// requestPathWithinPrefix reports whether the given request URI falls *within* the supplied
// path prefix using normalized path boundaries. It is hardened against:
//   - boundary tricks    e.g. "/outpost.goauthentik.io.evil" must NOT match "/outpost.goauthentik.io"
//   - path traversal     e.g. "/outpost.goauthentik.io/../admin" must NOT match (resolves to "/admin")
//   - encoded traversal  e.g. "/outpost.goauthentik.io/%2e%2e/admin"
//
// This matters because a false positive would skip authentication for a path the operator
// did not intend to expose.
func requestPathWithinPrefix(requestURI string, prefix string) bool {
	requestPath := requestURI
	if u, err := url.ParseRequestURI(requestURI); err == nil {
		//Use the decoded path only, dropping any query string / fragment
		requestPath = u.Path
	}
	requestPath = path.Clean("/" + requestPath)
	cleanedPrefix := path.Clean("/" + prefix)
	return requestPath == cleanedPrefix || strings.HasPrefix(requestPath, cleanedPrefix+"/")
}

// IsIgnoredPath reports whether the given request URI should bypass forward auth because it
// falls within one of the configured ignored path prefixes. Empty / whitespace-only entries
// are skipped so a stray comma cannot accidentally disable auth for the whole site.
func (ar *AuthRouter) IsIgnoredPath(requestURI string) bool {
	if ar == nil {
		return false
	}
	for _, prefix := range ar.options.IgnoredPaths {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			continue
		}
		if requestPathWithinPrefix(requestURI, prefix) {
			return true
		}
	}
	return false
}
