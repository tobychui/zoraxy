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
			if got := CheckException(r, rules); got != tc.want {
				t.Errorf("CheckException(%q) = %v, expected %v", tc.target, got, tc.want)
			}
		})
	}
}

func TestCheckExceptionUnusablePrefix(t *testing.T) {
	for _, prefix := range []string{"", "  ", "/"} {
		rules := []*ExceptionRule{{RuleType: ExceptionTypePaths, PathPrefix: prefix}}
		r := httptest.NewRequest(http.MethodGet, "/admin/secret.txt", nil)
		if CheckException(r, rules) {
			t.Errorf("prefix %q exempted the request, expected no match", prefix)
		}
	}
}

func TestCheckExceptionNilSafety(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/public", nil)
	if CheckException(r, nil) {
		t.Error("nil rule slice should not match")
	}
	if CheckException(r, []*ExceptionRule{nil}) {
		t.Error("nil rule entry should not match")
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
