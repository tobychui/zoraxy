package dynamicproxy

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"imuslab.com/zoraxy/mod/access"
	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/info/logger"
)

// newTestRateLimitRouter builds a minimal Router wired with a real access
// controller, so the rate limiter resolves client IPs the same way it does
// in production.
func newTestRateLimitRouter(t *testing.T, trustProxyHeaders bool, trustedProxies ...string) *Router {
	t.Helper()
	tmpDir := t.TempDir()

	sysdb, err := database.NewDatabase(filepath.Join(tmpDir, "sysdb"), database.GetRecommendedBackendType())
	if err != nil {
		t.Fatalf("unable to create test database: %v", err)
	}
	t.Cleanup(func() { sysdb.Close() })

	testLogger, err := logger.NewFmtLogger()
	if err != nil {
		t.Fatalf("unable to create test logger: %v", err)
	}

	accessController, err := access.NewAccessController(&access.Options{
		Logger:             *testLogger,
		ConfigFolder:       filepath.Join(tmpDir, "access"),
		TrustedProxiesFile: filepath.Join(tmpDir, "trusted_proxies.json"),
		Database:           sysdb,
	})
	if err != nil {
		t.Fatalf("unable to create access controller: %v", err)
	}

	defaultRule, err := accessController.GetAccessRuleByID("default")
	if err != nil {
		t.Fatalf("unable to load default access rule: %v", err)
	}
	defaultRule.TrustProxyHeadersOnly = trustProxyHeaders
	for _, proxy := range trustedProxies {
		accessController.AddTrustedProxy(proxy, "test")
	}

	return &Router{
		Option:           &RouterOption{AccessController: accessController},
		rateLimitCounter: RequestCountPerIpTable{},
	}
}

func newTestRequest(remoteAddr string, headers map[string]string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "https://host.example.com/", nil)
	r.RemoteAddr = remoteAddr
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

// A header supplied client IP carries no port. The limiter must still count it
// instead of silently failing open. Regression test for the CDN bypass where
// net.SplitHostPort() on a bare address aborted the whole check.
func TestHandleRateLimit_BareHeaderIPStillCounts(t *testing.T) {
	for _, headerName := range []string{"CF-Connecting-IP", "Fastly-Client-IP", "X-Real-Ip", "X-Forwarded-For"} {
		t.Run(headerName, func(t *testing.T) {
			router := newTestRateLimitRouter(t, true, "104.16.0.0/13")
			pe := &ProxyEndpoint{RequireRateLimit: true, RateLimit: 3}

			blocked := 0
			for i := 0; i < 10; i++ {
				w := httptest.NewRecorder()
				r := newTestRequest("104.16.5.1:443", map[string]string{headerName: "203.0.113.99"})
				if err := router.handleRateLimit(w, r, pe); err != nil {
					blocked++
					if w.Code != 429 {
						t.Errorf("expected status 429 on block, got %d", w.Code)
					}
				}
			}

			if blocked == 0 {
				t.Fatalf("%s: rate limit never fired, limiter failed open", headerName)
			}
			if got := router.rateLimitCounter.GetCount("203.0.113.99"); got != 10 {
				t.Errorf("%s: expected 10 requests counted against 203.0.113.99, got %d", headerName, got)
			}
		})
	}
}

// Every visitor behind the CDN must get their own counter rather than sharing
// the edge address, otherwise one busy client 429s everyone else.
func TestHandleRateLimit_CountsPerRealClient(t *testing.T) {
	router := newTestRateLimitRouter(t, true, "104.16.0.0/13")
	pe := &ProxyEndpoint{RequireRateLimit: true, RateLimit: 3}

	//Exceeded() blocks once the counter reaches the limit, so a limit of 3
	//lets 2 requests through before the third is rejected
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		r := newTestRequest("104.16.5.1:443", map[string]string{"CF-Connecting-IP": "203.0.113.1"})
		if err := router.handleRateLimit(w, r, pe); err != nil {
			t.Fatalf("first client blocked too early at request %d", i)
		}
	}

	//A different client through the same edge node must not be affected
	w := httptest.NewRecorder()
	r := newTestRequest("104.16.5.1:443", map[string]string{"CF-Connecting-IP": "203.0.113.2"})
	if err := router.handleRateLimit(w, r, pe); err != nil {
		t.Fatalf("second client should not share the first client's counter")
	}
}

// A spoofed header from a peer that is not on the trusted proxy list must not
// let a client escape the limiter by rotating the header value.
func TestHandleRateLimit_SpoofedHeaderFromUntrustedPeerIgnored(t *testing.T) {
	router := newTestRateLimitRouter(t, true) //no trusted proxies configured
	pe := &ProxyEndpoint{RequireRateLimit: true, RateLimit: 3}

	blocked := 0
	for i := 0; i < 10; i++ {
		w := httptest.NewRecorder()
		//Every request claims a different origin IP
		r := newTestRequest("198.51.100.7:51000", map[string]string{
			"CF-Connecting-IP": "203.0.113." + string(rune('0'+i)),
		})
		if err := router.handleRateLimit(w, r, pe); err != nil {
			blocked++
		}
	}

	if blocked == 0 {
		t.Fatal("spoofed CF-Connecting-IP allowed a client to bypass the rate limit")
	}
	if got := router.rateLimitCounter.GetCount("198.51.100.7"); got != 10 {
		t.Errorf("expected all 10 requests counted against the real peer, got %d", got)
	}
}

// The pre-existing behaviour of counting the direct peer when no proxy header
// is involved must be unchanged.
func TestHandleRateLimit_DirectConnection(t *testing.T) {
	router := newTestRateLimitRouter(t, false)
	pe := &ProxyEndpoint{RequireRateLimit: true, RateLimit: 3}

	blocked := 0
	for i := 0; i < 10; i++ {
		w := httptest.NewRecorder()
		r := newTestRequest("198.51.100.20:44321", nil)
		if err := router.handleRateLimit(w, r, pe); err != nil {
			blocked++
		}
	}

	if blocked != 8 {
		t.Errorf("expected 8 of 10 requests blocked at limit 3, got %d", blocked)
	}
}
