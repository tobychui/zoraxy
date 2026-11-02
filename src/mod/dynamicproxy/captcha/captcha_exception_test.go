package captcha

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckExceptionPathRules(t *testing.T) {
	rules := []*ExceptionRule{{
		RuleType:   ExceptionTypePaths,
		PathPrefix: "/public",
	}}

	cases := []struct {
		name   string
		target string
		want   bool
	}{
		{"exact", "/public", true},
		{"subpath", "/public/logo.png", true},
		{"subpath_with_query", "/public/logo.png?v=2", true},
		{"resolves_into_prefix", "/admin/../public/logo.png", true},

		{"dot_segments", "/public/../admin/secret.txt", false},
		{"encoded_dot_segments", "/public/%2e%2e/admin/secret.txt", false},
		{"sibling_prefix_hyphen", "/public-internal/secret.txt", false},
		{"sibling_prefix_substring", "/publicsecret", false},
		{"unrelated", "/admin/secret.txt", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, tc.target, nil)
			if got := CheckException(r, rules, "192.0.2.1"); got != tc.want {
				t.Errorf("CheckException(%q) = %v, expected %v", tc.target, got, tc.want)
			}
		})
	}
}

func TestCheckExceptionUnusablePrefix(t *testing.T) {
	for _, prefix := range []string{"", "  ", "/"} {
		rules := []*ExceptionRule{{RuleType: ExceptionTypePaths, PathPrefix: prefix}}
		r := httptest.NewRequest(http.MethodGet, "/admin/secret.txt", nil)
		if CheckException(r, rules, "192.0.2.1") {
			t.Errorf("prefix %q exempted the request, expected no match", prefix)
		}
	}
}

func TestCheckExceptionNilSafety(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/public", nil)
	if CheckException(r, nil, "192.0.2.1") {
		t.Error("nil rule slice should not match")
	}
	if CheckException(r, []*ExceptionRule{nil}, "192.0.2.1") {
		t.Error("nil rule entry should not match")
	}
}

func TestCheckExceptionCIDRRules(t *testing.T) {
	cases := []struct {
		name     string
		rule     string
		clientIP string
		want     bool
	}{
		{"cidr_inside", "10.0.0.0/8", "10.1.2.3", true},
		{"cidr_outside", "10.0.0.0/8", "203.0.113.5", false},
		{"single_ip_match", "192.0.2.10", "192.0.2.10", true},
		{"single_ip_mismatch", "192.0.2.10", "192.0.2.11", false},
		{"single_ip_with_spaces", " 192.0.2.10 ", "192.0.2.10", true},
		{"ipv6_cidr_inside", "2001:db8::/32", "2001:db8::1", true},
		{"ipv6_equivalent_notation", "2001:db8::1", "2001:db8:0:0:0:0:0:1", true},
		{"ipv4_mapped_ipv6", "10.0.0.0/8", "::ffff:10.1.2.3", true},
		{"empty_client_ip", "10.0.0.0/8", "", false},
		{"invalid_client_ip", "10.0.0.0/8", "10.1.2.3, 203.0.113.5", false},
		{"invalid_rule", "not-an-ip", "10.1.2.3", false},
		{"empty_rule", "", "10.1.2.3", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rules := []*ExceptionRule{{RuleType: ExceptionTypeCIDR, CIDR: tc.rule}}
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if got := CheckException(r, rules, tc.clientIP); got != tc.want {
				t.Errorf("CheckException(rule=%q, clientIP=%q) = %v, expected %v", tc.rule, tc.clientIP, got, tc.want)
			}
		})
	}
}

// CIDR exceptions must only be matched against the caller resolved client IP.
// Forwarding headers on the request are attacker controlled unless the caller
// decided to trust them, so they must never be read here (#1321)
func TestCheckExceptionIgnoresForwardingHeaders(t *testing.T) {
	rules := []*ExceptionRule{{RuleType: ExceptionTypeCIDR, CIDR: "10.0.0.0/8"}}
	for _, headerName := range []string{"X-Real-Ip", "CF-Connecting-IP", "Fastly-Client-IP", "X-Forwarded-For"} {
		t.Run(headerName, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = "203.0.113.5:44321"
			r.Header.Set(headerName, "10.1.2.3")
			if CheckException(r, rules, "203.0.113.5") {
				t.Errorf("%s header satisfied a CIDR exception", headerName)
			}
		})
	}
}

// ProtectedPathPrefixes is an inclusion list, so it must resolve the path too:
// a request that resolves into the protected subtree has to be enforced.
func TestShouldEnforcePathResolvesPath(t *testing.T) {
	cfg := &Config{ProtectedPathPrefixes: []string{"/api"}}

	cases := []struct {
		name   string
		target string
		want   bool
	}{
		{"direct", "/api/admin", true},
		{"resolves_into_prefix", "/x/../api/admin", true},
		{"encoded_resolves_into_prefix", "/x/%2e%2e/api/admin", true},
		{"resolves_out_of_prefix", "/api/../public", false},
		{"sibling_prefix_hyphen", "/api-v2/admin", false},
		{"unrelated", "/public/logo.png", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShouldEnforcePath(tc.target, cfg); got != tc.want {
				t.Errorf("ShouldEnforcePath(%q) = %v, expected %v", tc.target, got, tc.want)
			}
		})
	}
}

// The root prefix keeps its broad meaning here, the opposite of how an
// exception rule treats it. This is the regression guard for that asymmetry.
func TestShouldEnforcePathRootPrefixProtectsEverything(t *testing.T) {
	cfg := &Config{ProtectedPathPrefixes: []string{"/"}}
	for _, target := range []string{"/", "/anything", "/deep/nested/path", "/x/../y"} {
		if !ShouldEnforcePath(target, cfg) {
			t.Errorf("ShouldEnforcePath(%q) with root prefix = false, expected true", target)
		}
	}
}

func TestShouldEnforcePathEdgeCases(t *testing.T) {
	if !ShouldEnforcePath("/anything", &Config{ProtectedPathPrefixes: []string{}}) {
		t.Error("an empty prefix list should protect all paths")
	}
	if !ShouldEnforcePath("/anything", nil) {
		t.Error("a nil config should protect all paths")
	}
	if ShouldEnforcePath("/anything", &Config{ProtectedPathPrefixes: []string{"  "}}) {
		t.Error("a whitespace-only prefix should not protect anything")
	}
}
