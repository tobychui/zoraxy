package pathmatch

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestRequestPathWithinPrefix(t *testing.T) {
	const prefix = "/public"

	cases := []struct {
		name   string
		target string
		want   bool
	}{
		{"exact", "/public", true},
		{"trailing_slash", "/public/", true},
		{"subpath", "/public/logo.png", true},
		{"subpath_with_query", "/public/logo.png?v=2", true},
		{"single_dot_segment", "/public/./logo.png", true},
		{"double_slash", "//public/logo.png", true},
		{"resolves_into_prefix", "/admin/../public/logo.png", true},

		{"dot_segments", "/public/../admin/secret.txt", false},
		{"encoded_dot_segments", "/public/%2e%2e/admin/secret.txt", false},
		{"dot_segments_double_slash", "/public/..//admin/secret.txt", false},
		{"encoded_dot_segments_encoded_slash", "/public/%2e%2e%2fadmin", false},
		{"nested_dot_segments", "/public/a/../../admin", false},
		{"sibling_prefix_hyphen", "/public-internal/secret.txt", false},
		{"sibling_prefix_substring", "/publicsecret", false},
		{"different_case", "/Public/logo.png", false},
		{"unrelated", "/admin/secret.txt", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RequestPathWithinPrefix(tc.target, prefix); got != tc.want {
				t.Errorf("RequestPathWithinPrefix(%q, %q) = %v, expected %v", tc.target, prefix, got, tc.want)
			}
		})
	}
}

func TestRequestPathWithinPrefixPrefixForms(t *testing.T) {
	const target = "/public/logo.png"

	cases := []struct {
		name   string
		prefix string
		want   bool
	}{
		{"plain", "/public", true},
		{"trailing_slash", "/public/", true},
		{"no_leading_slash", "public", true},
		{"surrounding_space", "  /public  ", true},
		{"empty", "", false},
		{"whitespace_only", "   ", false},
		{"root", "/", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RequestPathWithinPrefix(target, tc.prefix); got != tc.want {
				t.Errorf("RequestPathWithinPrefix(%q, %q) = %v, expected %v", target, tc.prefix, got, tc.want)
			}
		})
	}
}

// An unusable prefix must never widen into a match, otherwise a stray blank rule
// would cover every request on the endpoint.
func TestRequestPathWithinPrefixRejectsUnusablePrefix(t *testing.T) {
	for _, prefix := range []string{"", " ", "\t", "\n", "   "} {
		if RequestPathWithinPrefix("/anything/at/all", prefix) {
			t.Errorf("prefix %q matched, expected no match", prefix)
		}
	}
}

func TestPrefixIsRoot(t *testing.T) {
	cases := map[string]bool{
		"/":        true,
		"":         false,
		"   ":      false,
		"/public":  false,
		"public":   false,
		"/.":       true,
		"/public/": false,
	}
	for prefix, want := range cases {
		if got := PrefixIsRoot(prefix); got != want {
			t.Errorf("PrefixIsRoot(%q) = %v, expected %v", prefix, got, want)
		}
	}
}

func TestCleanRequestPath(t *testing.T) {
	cases := map[string]string{
		"/a/b/../c":              "/a/c",
		"/a/%2e%2e/b":            "/b",
		"/a?x=1":                 "/a",
		"/a#frag":                "/a#frag",
		"":                       "/",
		"/":                      "/",
		"http://host/a/../b":     "/b",
		"/trailing/":             "/trailing",
		"/dots/./here":           "/dots/here",
		"/escape/../../../../up": "/up",
	}
	for in, want := range cases {
		if got := CleanRequestPath(in); got != want {
			t.Errorf("CleanRequestPath(%q) = %q, expected %q", in, got, want)
		}
	}
}

func TestRequestTarget(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/a?b=1", nil)
	if got := RequestTarget(r); got != "/a?b=1" {
		t.Errorf("RequestTarget = %q, expected /a?b=1", got)
	}

	u, _ := url.ParseRequestURI("/only/url")
	if got := RequestTarget(&http.Request{URL: u}); got != "/only/url" {
		t.Errorf("RequestTarget fallback = %q, expected /only/url", got)
	}

	if got := RequestTarget(nil); got != "/" {
		t.Errorf("RequestTarget(nil) = %q, expected /", got)
	}
	if got := RequestTarget(&http.Request{}); got != "/" {
		t.Errorf("RequestTarget(empty) = %q, expected /", got)
	}
}
