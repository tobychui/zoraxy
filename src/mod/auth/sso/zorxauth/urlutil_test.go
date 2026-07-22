package zorxauth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestEnsureAbsoluteHTTPURL(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		scheme  string
		want    string
		wantErr bool
	}{
		{
			name:   "host only gets https",
			raw:    "za.lan",
			scheme: "https",
			want:   "https://za.lan",
		},
		{
			name:   "host only uses request scheme",
			raw:    "za.lan",
			scheme: "http",
			want:   "http://za.lan",
		},
		{
			name:   "already absolute https preserved",
			raw:    "https://auth.example.com",
			scheme: "http",
			want:   "https://auth.example.com",
		},
		{
			name:   "path on SSO host preserved",
			raw:    "https://auth.example.com/login",
			scheme: "https",
			want:   "https://auth.example.com/login",
		},
		{
			name:   "trims whitespace and lone slash",
			raw:    "  https://za.lan/  ",
			scheme: "https",
			want:   "https://za.lan",
		},
		{
			name:    "empty rejected",
			raw:     "   ",
			scheme:  "https",
			wantErr: true,
		},
		{
			name:    "relative path rejected",
			raw:     "/only-path",
			scheme:  "https",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ensureAbsoluteHTTPURL(tt.raw, tt.scheme)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildSSOAuthRedirect(t *testing.T) {
	got, err := buildSSOAuthRedirect("https://za.lan", "https://docs.local/docs")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if u.Scheme != "https" || u.Host != "za.lan" {
		t.Fatalf("unexpected base: %s", got)
	}
	if u.Query().Get("redirect") != "https://docs.local/docs" {
		t.Fatalf("redirect query = %q", u.Query().Get("redirect"))
	}

	// Relative / host-only must fail so callers normalize first.
	if _, err := buildSSOAuthRedirect("za.lan", "https://docs.local/"); err == nil {
		t.Fatal("expected error for non-absolute SSO base")
	}
}

func TestBuildOriginalRequestURLUnwrapsNestedRedirect(t *testing.T) {
	nested := "https://docs.local/za.lan?redirect=" + url.QueryEscape(
		"https://docs.local/za.lan?redirect="+url.QueryEscape("https://docs.local/"),
	)
	req := httptest.NewRequest(http.MethodGet, nested, nil)
	req.Host = "docs.local"
	req.Header.Set("X-Forwarded-Proto", "https")

	got := buildOriginalRequestURL(req)
	if got != "https://docs.local/" {
		t.Fatalf("got %q, want https://docs.local/", got)
	}
}

func TestBuildOriginalRequestURLKeepsAppRedirectQuery(t *testing.T) {
	raw := "https://docs.local/oauth/callback?redirect=" + url.QueryEscape("https://oauth.example.com/done")
	req := httptest.NewRequest(http.MethodGet, raw, nil)
	req.Host = "docs.local"
	req.Header.Set("X-Forwarded-Proto", "https")

	got := buildOriginalRequestURL(req)
	if got != raw {
		t.Fatalf("got %q, want original app URL %q", got, raw)
	}
}

func TestBuildLoginRedirectURL_HostOnlyConfig(t *testing.T) {
	ar := &AuthRouter{
		Options: &AuthRouterOptions{
			SSORedirectURL: "za.lan",
		},
	}
	req := httptest.NewRequest(http.MethodGet, "https://docs.local/readme", nil)
	req.Host = "docs.local"
	req.Header.Set("X-Forwarded-Proto", "https")

	got, err := ar.buildLoginRedirectURL(req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "https://za.lan?") {
		t.Fatalf("expected absolute SSO host, got %q", got)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("redirect") != "https://docs.local/readme" {
		t.Fatalf("redirect = %q", u.Query().Get("redirect"))
	}
}

func TestBuildLoginRedirectURL_RejectsSameHostLoop(t *testing.T) {
	ar := &AuthRouter{
		Options: &AuthRouterOptions{
			SSORedirectURL: "https://docs.local/sso",
		},
	}
	req := httptest.NewRequest(http.MethodGet, "https://docs.local/", nil)
	req.Host = "docs.local"
	req.Header.Set("X-Forwarded-Proto", "https")

	if _, err := ar.buildLoginRedirectURL(req); err == nil {
		t.Fatal("expected same-host loop error")
	}
}

func TestSSORedirectWouldLoop(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://docs.local/", nil)
	req.Host = "docs.local"
	if !ssoRedirectWouldLoop("https://docs.local/login", req) {
		t.Fatal("expected loop")
	}
	if ssoRedirectWouldLoop("https://za.lan", req) {
		t.Fatal("did not expect loop")
	}
}
