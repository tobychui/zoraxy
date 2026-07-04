package forward

import "testing"

// TestRequestPathWithinPrefix verifies the hardened path matching used by Ignored Paths. The
// boundary and path-traversal cases are security relevant: a false positive would skip
// authentication on a path the operator did not intend to expose.
func TestRequestPathWithinPrefix(t *testing.T) {
	const outpost = "/outpost.goauthentik.io"

	cases := []struct {
		name   string
		uri    string
		prefix string
		want   bool
	}{
		{"callback with query", "/outpost.goauthentik.io/callback?code=abc&state=xyz", outpost, true},
		{"exact", "/outpost.goauthentik.io", outpost, true},
		{"trailing slash", "/outpost.goauthentik.io/", outpost, true},
		{"start endpoint", "/outpost.goauthentik.io/start", outpost, true},
		{"sign_out endpoint", "/outpost.goauthentik.io/sign_out", outpost, true},
		{"boundary sibling", "/outpost.goauthentik.io.evil/x", outpost, false},
		{"prefix as substring", "/outpost.goauthentik.ioxyz", outpost, false},
		{"dot dot traversal", "/outpost.goauthentik.io/../admin", outpost, false},
		{"encoded traversal", "/outpost.goauthentik.io/%2e%2e/admin", outpost, false},
		{"nested traversal", "/outpost.goauthentik.io/foo/../../secret", outpost, false},
		{"unrelated path", "/api/v1/data", outpost, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := requestPathWithinPrefix(tc.uri, tc.prefix); got != tc.want {
				t.Errorf("requestPathWithinPrefix(%q, %q) = %v, want %v", tc.uri, tc.prefix, got, tc.want)
			}
		})
	}
}

// TestAuthRouterIsIgnoredPath verifies multi-prefix matching, that empty/whitespace entries
// are skipped, that traversal cannot escape a configured prefix, and that an empty list never
// bypasses authentication.
func TestAuthRouterIsIgnoredPath(t *testing.T) {
	ar := &AuthRouter{
		options: &AuthRouterOptions{
			IgnoredPaths: []string{"/outpost.goauthentik.io", "  ", "/healthz"},
		},
	}

	cases := []struct {
		name string
		uri  string
		want bool
	}{
		{"first prefix", "/outpost.goauthentik.io/callback?code=x", true},
		{"second prefix exact", "/healthz", true},
		{"second prefix subpath", "/healthz/live", true},
		{"not listed", "/admin", false},
		{"traversal escapes", "/outpost.goauthentik.io/../admin", false},
		{"boundary", "/healthzzz", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ar.IsIgnoredPath(tc.uri); got != tc.want {
				t.Errorf("IsIgnoredPath(%q) = %v, want %v", tc.uri, got, tc.want)
			}
		})
	}

	empty := &AuthRouter{options: &AuthRouterOptions{}}
	if empty.IsIgnoredPath("/outpost.goauthentik.io/callback") {
		t.Errorf("empty IgnoredPaths must never bypass authentication")
	}
}
