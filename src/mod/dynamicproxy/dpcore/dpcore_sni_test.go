package dpcore_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

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
		name       string
		rawURL     string
		serverName string //value of DpcoreOptions.UpstreamTLSServerName
		wantSNI    string
	}{
		{"https hostname keeps fixed SNI", "https://backend.example.com:8443", "", "backend.example.com"},
		{"https IP has no shared SNI", "https://10.10.251.5:443", "", ""},
		{"http upstream has no TLS SNI", "http://backend.example.com", "", ""},

		// An admin configured server name (the endpoint's "Overwrite Host Header"
		// value) overrides the name derived from the upstream address.
		{"override wins over hostname upstream", "https://adguardhome:443", "adguard.example.com", "adguard.example.com"},
		{"override gives IP upstream a fixed SNI", "https://10.10.251.5:443", "adguard.example.com", "adguard.example.com"},
		{"override port is stripped", "https://adguardhome:443", "adguard.example.com:443", "adguard.example.com"},
		{"override is trimmed", "https://adguardhome:443", "  adguard.example.com  ", "adguard.example.com"},
		{"IP override falls back to upstream address", "https://backend.example.com:8443", "10.10.251.5", "backend.example.com"},
		{"IP override on IP upstream still has no shared SNI", "https://10.10.251.5:443", "192.168.1.1", ""},
		{"override is ignored for http upstream", "http://adguardhome", "adguard.example.com", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := url.Parse(tc.rawURL)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			p := dpcore.NewDynamicProxyCore(u, "", &dpcore.DpcoreOptions{
				UpstreamTLSServerName: tc.serverName,
			})
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

// newTestCert issues a self-signed certificate valid for the given DNS name,
// together with a root pool that trusts it.
func newTestCert(t *testing.T, dnsName string) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: dnsName},
		DNSNames:              []string{dnsName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, pool
}

// TestUpstreamTLSServerNameOverride covers the #1088 follow up: an upstream addressed
// by a hostname its certificate does not cover (the reported case is a Docker service
// name such as adguardhome:443 serving a cert for adguard.example.com). Without an
// override the handshake must fail certificate verification; with the endpoint's
// "Overwrite Host Header" value supplied as UpstreamTLSServerName it must succeed with
// verification still enabled, and the backend must observe the overridden SNI.
func TestUpstreamTLSServerNameOverride(t *testing.T) {
	const certName = "adguard.example.com"

	var (
		mu     sync.Mutex
		gotSNI string
	)
	cert, pool := newTestCert(t, certName)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{cert},
		GetConfigForClient: func(chi *tls.ClientHelloInfo) (*tls.Config, error) {
			mu.Lock()
			gotSNI = chi.ServerName
			mu.Unlock()
			return nil, nil
		},
	}
	srv.StartTLS()
	defer srv.Close()

	_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "https://"))
	if err != nil {
		t.Fatalf("split server url: %v", err)
	}
	// "localhost" resolves to the test server but is not covered by the certificate,
	// standing in for the Docker service name in the report.
	u, err := url.Parse("https://localhost:" + port)
	if err != nil {
		t.Fatalf("parse upstream url: %v", err)
	}

	// newProxy builds a proxy for the upstream above and trusts the test CA, so the
	// only thing that can fail verification is a hostname mismatch.
	newProxy := func(serverName string) *dpcore.ReverseProxy {
		p := dpcore.NewDynamicProxyCore(u, "", &dpcore.DpcoreOptions{
			UpstreamTLSServerName: serverName,
		})
		tr, ok := p.Transport.(*http.Transport)
		if !ok || tr.TLSClientConfig == nil {
			t.Fatalf("proxy has no TLS client config")
		}
		tr.TLSClientConfig.RootCAs = pool
		return p
	}

	const frontHost = "frontdomain.example.com"
	proxyOnce := func(p *dpcore.ReverseProxy) (int, error) {
		req := httptest.NewRequest(http.MethodGet, "http://"+frontHost+"/", nil)
		return p.ProxyHTTP(httptest.NewRecorder(), req, &dpcore.ResponseRewriteRuleSet{
			UseTLS:       true,
			ProxyDomain:  u.Host,
			OriginalHost: frontHost,
		})
	}

	t.Run("without override verification fails", func(t *testing.T) {
		status, err := proxyOnce(newProxy(""))
		if err == nil {
			t.Fatalf("expected certificate verification to fail, got status %d", status)
		}
		if status != http.StatusBadGateway {
			t.Fatalf("status = %d, want %d", status, http.StatusBadGateway)
		}
		if !strings.Contains(err.Error(), "certificate is valid for") {
			t.Fatalf("expected a hostname mismatch error, got: %v", err)
		}
	})

	t.Run("override passes verification and sets SNI", func(t *testing.T) {
		mu.Lock()
		gotSNI = ""
		mu.Unlock()

		// The override must also be what the shared transport carries, so the SNI is
		// fixed at init time and no per-request transport clone is needed.
		p := newProxy(certName)
		if got := sharedServerName(p); got != certName {
			t.Fatalf("shared ServerName = %q, want %q", got, certName)
		}

		status, err := proxyOnce(p)
		if err != nil {
			t.Fatalf("ProxyHTTP returned error: %v", err)
		}
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200", status)
		}

		mu.Lock()
		defer mu.Unlock()
		if gotSNI != certName {
			t.Fatalf("backend received SNI %q, want %q", gotSNI, certName)
		}
	})
}
