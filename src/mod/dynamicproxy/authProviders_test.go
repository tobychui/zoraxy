package dynamicproxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// newBasicAuthEndpoint builds a minimal basic auth endpoint carrying the given exception rules.
func newBasicAuthEndpoint(rules ...*BasicAuthExceptionRule) *ProxyEndpoint {
	return &ProxyEndpoint{
		RootOrMatchingDomain: "test.local",
		AuthenticationProvider: &AuthenticationProvider{
			AuthMethod:              AuthMethodBasic,
			BasicAuthCredentials:    []*BasicAuthCredentials{{Username: "admin", PasswordHash: "deadbeef"}},
			BasicAuthExceptionRules: rules,
		},
	}
}

// pathRule returns a path prefix exception rule with the given case matching mode.
func pathRule(prefix string, caseInsensitive bool) *BasicAuthExceptionRule {
	return &BasicAuthExceptionRule{
		RuleType:        AuthExceptionType_Paths,
		PathPrefix:      prefix,
		CaseInsensitive: caseInsensitive,
	}
}

// TestBasicAuthExceptionMatchedPathPrefix covers the path prefix exclusion matching rules.
func TestBasicAuthExceptionMatchedPathPrefix(t *testing.T) {
	cases := []struct {
		name   string
		rule   *BasicAuthExceptionRule
		target string
		want   bool
	}{
		// Regression for the report that a /public/ exclusion still challenged the bare /public.
		{"trailing_slash_rule_bare_path", pathRule("/public/", false), "/public", true},
		{"trailing_slash_rule_exact", pathRule("/public/", false), "/public/", true},
		{"trailing_slash_rule_subpath", pathRule("/public/", false), "/public/logo.png", true},
		{"trailing_slash_rule_query", pathRule("/public/", false), "/public/logo.png?v=2", true},
		{"no_slash_rule_bare_path", pathRule("/public", false), "/public", true},
		{"no_slash_rule_subpath", pathRule("/public", false), "/public/logo.png", true},
		{"prefix_without_leading_slash", pathRule("public/", false), "/public/logo.png", true},

		// A prefix must match on a segment boundary, never as a bare string prefix.
		{"sibling_substring", pathRule("/public", false), "/publicsecret", false},
		{"sibling_hyphen", pathRule("/public", false), "/public-internal/secret.txt", false},
		{"traversal_out_of_prefix", pathRule("/public/", false), "/public/../admin/secret.txt", false},
		{"encoded_traversal", pathRule("/public/", false), "/public/%2e%2e/admin/secret.txt", false},
		{"unrelated_path", pathRule("/public/", false), "/admin", false},
		{"empty_prefix", pathRule("   ", false), "/anything", false},

		// Case matching is opt-in per rule and must not widen the prefix boundary.
		{"case_sensitive_by_default", pathRule("/public/", false), "/Public/logo.png", false},
		{"case_insensitive_subpath", pathRule("/public/", true), "/Public/logo.png", true},
		{"case_insensitive_bare_path", pathRule("/public/", true), "/PUBLIC", true},
		{"case_insensitive_mixed_rule", pathRule("/PuBlIc/", true), "/public/logo.png", true},
		{"case_insensitive_sibling", pathRule("/public", true), "/PublicSecret", false},
		{"case_insensitive_traversal", pathRule("/public/", true), "/Public/../admin", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pe := newBasicAuthEndpoint(tc.rule)
			r := httptest.NewRequest(http.MethodGet, tc.target, nil)
			if got := basicAuthExceptionMatched(pe, r); got != tc.want {
				t.Errorf("rule %q (caseInsensitive=%v) against %q = %v, expected %v",
					tc.rule.PathPrefix, tc.rule.CaseInsensitive, tc.target, got, tc.want)
			}
		})
	}
}

// TestBasicAuthExceptionMatchedCIDR verifies IP exclusions still match on the untrusted client IP.
func TestBasicAuthExceptionMatchedCIDR(t *testing.T) {
	cidrRule := &BasicAuthExceptionRule{RuleType: AuthExceptionType_CIDR, CIDR: "192.168.1.0/24"}
	pe := newBasicAuthEndpoint(cidrRule)

	inside := httptest.NewRequest(http.MethodGet, "/admin", nil)
	inside.RemoteAddr = "192.168.1.50:41234"
	if !basicAuthExceptionMatched(pe, inside) {
		t.Errorf("client inside the excluded CIDR should be exempt")
	}

	outside := httptest.NewRequest(http.MethodGet, "/admin", nil)
	outside.RemoteAddr = "10.0.0.5:41234"
	if basicAuthExceptionMatched(pe, outside) {
		t.Errorf("client outside the excluded CIDR must not be exempt")
	}

	// A spoofed header must be ignored unless the rule opted into trusting proxy headers.
	spoofed := httptest.NewRequest(http.MethodGet, "/admin", nil)
	spoofed.RemoteAddr = "10.0.0.5:41234"
	spoofed.Header.Set("X-Real-Ip", "192.168.1.50")
	if basicAuthExceptionMatched(pe, spoofed) {
		t.Errorf("proxy headers must not grant an exemption when UseTrustedProxy is false")
	}
}

// TestBasicAuthExceptionMatchedGuards checks the nil endpoint, provider and rule entries are skipped.
func TestBasicAuthExceptionMatchedGuards(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/public/logo.png", nil)

	if basicAuthExceptionMatched(nil, r) {
		t.Errorf("nil endpoint must not be exempt")
	}
	if basicAuthExceptionMatched(&ProxyEndpoint{}, r) {
		t.Errorf("endpoint without an authentication provider must not be exempt")
	}

	pe := newBasicAuthEndpoint(nil, pathRule("/public/", false))
	if !basicAuthExceptionMatched(pe, r) {
		t.Errorf("a nil rule entry must be skipped instead of hiding later rules")
	}
}

// TestHandleBasicAuthHonoursPathException reproduces the reported flow end to end without credentials.
func TestHandleBasicAuthHonoursPathException(t *testing.T) {
	cases := []struct {
		target     string
		wantExempt bool
		wantStatus int
	}{
		{target: "/public", wantExempt: true, wantStatus: http.StatusOK},
		{target: "/public/logo.png", wantExempt: true, wantStatus: http.StatusOK},
		{target: "/admin", wantExempt: false, wantStatus: http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			pe := newBasicAuthEndpoint(pathRule("/public/", false))
			r := httptest.NewRequest(http.MethodGet, tc.target, nil)
			w := httptest.NewRecorder()

			err := handleBasicAuth(w, r, pe)
			if tc.wantExempt && err != nil {
				t.Fatalf("%s should bypass basic auth, got error %v", tc.target, err)
			}
			if !tc.wantExempt && err == nil {
				t.Fatalf("%s should be challenged for credentials", tc.target)
			}
			if w.Code != tc.wantStatus {
				t.Errorf("%s responded %d, expected %d", tc.target, w.Code, tc.wantStatus)
			}
		})
	}
}
