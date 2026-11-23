package dynamicproxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"imuslab.com/zoraxy/mod/auth/sso/zorxauth"
)

func newZorxAuthTestHandler() *ProxyHandler {
	return &ProxyHandler{Parent: &Router{Option: &RouterOption{ZorxAuthAgentRouter: &zorxauth.AuthRouter{}}}}
}

func newZorxAuthEndpoint(method AuthMethod, rules ...*ZorxAuthExceptionRule) *ProxyEndpoint {
	return &ProxyEndpoint{
		RootOrMatchingDomain: "app.local",
		AuthenticationProvider: &AuthenticationProvider{
			AuthMethod:             method,
			ZorxAuthExceptionRules: rules,
		},
	}
}

func spoofedIdentityRequest(target string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "http://app.local"+target, nil)
	r.Header.Set("Remote-User", "admin")
	r.Header.Set("Remote-User-Id", "spoofed")
	r.Header.Set("Remote-Groups", "admins")
	r.Header["Remote_user_id"] = []string{"spoofed"}
	r.Header.Set("X-Unrelated", "keep")
	return r
}

// An exempt path must reach the upstream anonymous, not with client supplied identity headers
func TestZorxAuthExemptRouteStripsIdentityHeaders(t *testing.T) {
	h := newZorxAuthTestHandler()
	sep := newZorxAuthEndpoint(AuthMethodZorxAuth, &ZorxAuthExceptionRule{RuleType: AuthExceptionType_Paths, PathPattern: "/public/"})
	r := spoofedIdentityRequest("/public/index.html")

	if err := h.handleZorxAuth(httptest.NewRecorder(), r, sep); err != nil {
		t.Fatalf("exempt request should pass, got %v", err)
	}
	for _, name := range []string{"Remote-User", "Remote-User-Id", "Remote-Groups", "Remote_user_id"} {
		if got := r.Header.Values(name); len(got) != 0 {
			t.Errorf("%s should be stripped on an exempt route, got %q", name, got)
		}
	}
	if r.Header.Get("X-Unrelated") != "keep" {
		t.Errorf("unrelated header removed")
	}
}

// Endpoints that do not use Zoraxy Auth are left alone, an upstream proxy may set these headers
func TestStripZorxAuthIdentityHeadersOnlyForZorxAuthEndpoints(t *testing.T) {
	h := newZorxAuthTestHandler()

	r := spoofedIdentityRequest("/")
	h.stripZorxAuthIdentityHeaders(newZorxAuthEndpoint(AuthMethodNone), r)
	if r.Header.Get("Remote-User") != "admin" {
		t.Errorf("headers of a non Zoraxy Auth endpoint should be untouched")
	}

	r = spoofedIdentityRequest("/")
	h.stripZorxAuthIdentityHeaders(newZorxAuthEndpoint(AuthMethodZorxAuth), r)
	if r.Header.Get("Remote-User") != "" || r.Header.Get("Remote-User-Id") != "" {
		t.Errorf("identity headers should be stripped for a Zoraxy Auth endpoint")
	}
}
