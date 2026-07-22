package dpcore_test

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
)

// sharedServerName returns the ServerName baked into the proxy's shared
// transport at construction time (empty if none / not an *http.Transport).
func sharedServerName(p *dpcore.ReverseProxy) string {
	tr, ok := p.Transport.(*http.Transport)
	if !ok || tr.TLSClientConfig == nil {
		return ""
	}
	return tr.TLSClientConfig.ServerName
}

// TestInitTimeSNI checks that a hostname upstream keeps a fixed shared SNI
// (so connection pooling is preserved), while an IP upstream leaves it empty
// because Go would drop an IP-literal SNI from the ClientHello (RFC 6066).
func TestInitTimeSNI(t *testing.T) {
	cases := []struct {
		name    string
		rawURL  string
		wantSNI string
	}{
		{"https hostname keeps fixed SNI", "https://backend.example.com:8443", "backend.example.com"},
		{"https IP has no shared SNI", "https://10.10.251.5:443", ""},
		{"http upstream has no TLS SNI", "http://backend.example.com", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := url.Parse(tc.rawURL)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			p := dpcore.NewDynamicProxyCore(u, "", &dpcore.DpcoreOptions{})
			if got := sharedServerName(p); got != tc.wantSNI {
				t.Fatalf("shared ServerName = %q, want %q", got, tc.wantSNI)
			}
		})
	}
}

// TestIPUpstreamUsesRequestHostSNI is the regression test for the v3.3.4 SNI
// change: an IP-addressed HTTPS upstream must still receive a valid SNI, derived
// from the (proxied) request host, so an SNI-routed backend can present the right
// cert. Before the fix, ServerName was the upstream IP, which Go omits entirely,
// leaving the backend with no SNI.
func TestIPUpstreamUsesRequestHostSNI(t *testing.T) {
	var (
		mu     sync.Mutex
		gotSNI string
	)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.TLS = &tls.Config{
		GetConfigForClient: func(chi *tls.ClientHelloInfo) (*tls.Config, error) {
			mu.Lock()
			gotSNI = chi.ServerName
			mu.Unlock()
			return nil, nil
		},
	}
	srv.StartTLS() // listens on a 127.0.0.1:<port> address (an IP upstream)
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server url: %v", err)
	}

	// Skip cert validation: the test cert won't match the SNI we assert on.
	p := dpcore.NewDynamicProxyCore(u, "", &dpcore.DpcoreOptions{IgnoreTLSVerification: true})

	const frontHost = "frontdomain.example.com"
	req := httptest.NewRequest(http.MethodGet, "http://"+frontHost+"/", nil)
	rw := httptest.NewRecorder()

	status, err := p.ProxyHTTP(rw, req, &dpcore.ResponseRewriteRuleSet{
		UseTLS:       true,
		ProxyDomain:  u.Host,
		OriginalHost: frontHost,
	})
	if err != nil {
		t.Fatalf("ProxyHTTP returned error: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}

	mu.Lock()
	defer mu.Unlock()
	if gotSNI != frontHost {
		t.Fatalf("backend received SNI %q, want %q", gotSNI, frontHost)
	}
}
