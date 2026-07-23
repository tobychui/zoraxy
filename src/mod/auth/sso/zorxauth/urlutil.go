package zorxauth

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// requestScheme returns http or https for the incoming request, honoring X-Forwarded-Proto.
func requestScheme(r *http.Request) string {
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		return "https"
	}
	return "http"
}

// ensureAbsoluteHTTPURL makes sure raw is an absolute http(s) URL.
// Host-only values such as "auth.example.com" (common when users omit the
// protocol) are rewritten with fallbackScheme so http.Redirect does not treat
// them as a relative path on the protected host (see #1108).
func ensureAbsoluteHTTPURL(raw, fallbackScheme string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("empty URL")
	}
	if strings.HasPrefix(raw, "/") {
		return "", errors.New("URL must include a host")
	}
	if fallbackScheme != "http" && fallbackScheme != "https" {
		fallbackScheme = "https"
	}

	if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
		raw = fallbackScheme + "://" + strings.TrimLeft(raw, "/")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("URL scheme must be http or https")
	}
	if u.Host == "" {
		return "", errors.New("URL missing host")
	}

	// Keep path if present; drop a lone trailing slash so "?redirect=" appends cleanly
	// when callers still concatenate. Prefer buildSSOAuthRedirect for new call sites.
	if u.Path == "/" {
		u.Path = ""
	}
	return u.String(), nil
}

// buildSSOAuthRedirect builds the SSO login URL with a redirect query parameter
// pointing at the original protected resource.
func buildSSOAuthRedirect(ssoBase, originalURL string) (string, error) {
	u, err := url.Parse(ssoBase)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", errors.New("SSO redirect URL must be absolute")
	}
	q := u.Query()
	q.Set("redirect", originalURL)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// buildOriginalRequestURL reconstructs the absolute URL the user originally requested.
// Nested "redirect" query values from a prior broken relative SSO hop are unwrapped
// so a single successful login lands on the real resource instead of a nested URL.
func buildOriginalRequestURL(r *http.Request) string {
	scheme := requestScheme(r)
	candidate := scheme + "://" + r.Host + r.URL.RequestURI()
	return unwrapNestedRedirectTarget(candidate, r.Host)
}

// unwrapNestedRedirectTarget walks redirect= query values that still point at the
// same host (the #1108 relative-redirect loop shape) and returns the innermost URL.
// It only unwraps when the path looks like a host token (e.g. "/za.lan"), so a
// legitimate app query such as "/oauth/callback?redirect=..." is left alone.
func unwrapNestedRedirectTarget(candidate, host string) string {
	const maxDepth = 8
	for i := 0; i < maxDepth; i++ {
		u, err := url.Parse(candidate)
		if err != nil {
			return candidate
		}
		if !strings.EqualFold(u.Hostname(), hostnameOnly(host)) {
			return candidate
		}

		path := strings.Trim(u.Path, "/")
		// Relative SSO loop uses the SSO hostname as a single path segment.
		if path == "" || strings.Contains(path, "/") || !strings.Contains(path, ".") {
			return candidate
		}

		redirectParam := u.Query().Get("redirect")
		if redirectParam == "" {
			return candidate
		}
		next, err := url.Parse(redirectParam)
		if err != nil || next.Scheme == "" || next.Host == "" {
			return candidate
		}
		if !strings.EqualFold(next.Hostname(), hostnameOnly(host)) {
			return candidate
		}
		candidate = redirectParam
	}
	return candidate
}

func hostnameOnly(hostport string) string {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return hostport
	}
	return host
}

// ssoRedirectWouldLoop reports whether sending the browser to ssoBase would stay
// on the same host as the protected request (misconfiguration / relative path).
func ssoRedirectWouldLoop(ssoBase string, r *http.Request) bool {
	u, err := url.Parse(ssoBase)
	if err != nil || u.Host == "" {
		return true
	}
	return strings.EqualFold(u.Hostname(), hostnameOnly(r.Host))
}
