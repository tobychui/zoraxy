package dynamicproxy

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"imuslab.com/zoraxy/mod/dynamicproxy/captcha"
	"imuslab.com/zoraxy/mod/dynamicproxy/loadbalance"
	"imuslab.com/zoraxy/mod/info/logger"
)

func newTestNonTLSRouter(t *testing.T, forceHTTPSRedirect bool) (*Router, *atomic.Int32, *loadbalance.Upstream) {
	t.Helper()

	var upstreamHits atomic.Int32
	upstreamSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHits.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("upstream ok"))
	}))
	t.Cleanup(upstreamSrv.Close)

	testLogger, err := logger.NewFmtLogger()
	if err != nil {
		t.Fatalf("unable to create test logger: %v", err)
	}

	lb := loadbalance.NewLoadBalancer(&loadbalance.Options{Logger: testLogger})
	t.Cleanup(lb.Close)

	upstreamAddr := strings.TrimPrefix(upstreamSrv.URL, "http://")
	upstream := &loadbalance.Upstream{
		OriginIpOrDomain: upstreamAddr,
		Weight:           1,
	}
	if err := upstream.StartProxy(""); err != nil {
		t.Fatalf("unable to initialize upstream proxy: %v", err)
	}

	router := newTestRateLimitRouter(t, false)
	router.Option.Port = 443
	router.Option.ForceHttpsRedirect = forceHTTPSRedirect
	router.Option.Logger = testLogger
	router.ProxyEndpoints = &sync.Map{}
	router.Root = &ProxyEndpoint{DefaultSiteOption: DefaultSite_InternalStaticWebServer}
	router.mux = &ProxyHandler{Parent: router}
	router.loadBalancer = lb

	return router, &upstreamHits, upstream
}

func addTestACMERoutingRule(t *testing.T, router *Router, enabled bool) {
	t.Helper()
	err := router.AddRoutingRules(&RoutingRule{
		ID:      "acme-autorenew",
		Enabled: enabled,
		MatchRule: func(r *http.Request) bool {
			parsedURL, err := url.ParseRequestURI(r.RequestURI)
			if err != nil {
				return false
			}
			return strings.HasPrefix(parsedURL.Path, "/.well-known/acme-challenge/")
		},
		RoutingHandler: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("acme-challenge-ok"))
		},
	})
	if err != nil {
		t.Fatalf("unable to register ACME routing rule: %v", err)
	}
}

func newPort80HTTPRequest(host, path string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "http://"+host+path, nil)
	r.RequestURI = r.URL.RequestURI()
	r.RemoteAddr = "198.51.100.10:54321"
	return r
}

func addTestEndpoint(router *Router, domain string, bypassGlobalTLS bool, upstream *loadbalance.Upstream) *ProxyEndpoint {
	ep := GetDefaultProxyEndpoint()
	ep.RootOrMatchingDomain = domain
	ep.BypassGlobalTLS = bypassGlobalTLS
	ep.ActiveOrigins = []*loadbalance.Upstream{upstream}
	ep.parent = router
	router.ProxyEndpoints.Store(domain, &ep)
	return &ep
}

// Regression test for #1342: hosts with BypassGlobalTLS ("Allow plain HTTP access")
// must still match special routing rules (like ACME HTTP-01 challenges) on port 80
// before rate limiting, CAPTCHA, authentication, or upstream proxying.
func TestHandlePort80Request_BypassGlobalTLS_RoutesSpecialRulesFirst(t *testing.T) {
	router, upstreamHits, upstream := newTestNonTLSRouter(t, true)
	addTestACMERoutingRule(t, router, true)

	ep := addTestEndpoint(router, "plain.example.com", true, upstream)
	ep.RequireRateLimit = true
	ep.RateLimit = 1
	ep.RequireCaptcha = true
	ep.CaptchaConfig = &captcha.Config{
		Provider:  captcha.ProviderCloudflare,
		SiteKey:   "test-site-key",
		SecretKey: "test-secret-key",
	}
	ep.AuthenticationProvider = &AuthenticationProvider{
		AuthMethod: AuthMethodBasic,
		BasicAuthCredentials: []*BasicAuthCredentials{
			{Username: "alice", PasswordHash: "unused"},
		},
	}

	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		r := newPort80HTTPRequest("plain.example.com:80", "/.well-known/acme-challenge/token-123")
		router.handlePort80Request(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("request %d: expected status %d, got %d (%s)", i+1, http.StatusOK, w.Code, w.Body.String())
		}
		if got := w.Body.String(); got != "acme-challenge-ok" {
			t.Fatalf("request %d: expected ACME handler body, got %q", i+1, got)
		}
	}

	if got := upstreamHits.Load(); got != 0 {
		t.Fatalf("expected 0 upstream hits for ACME challenge requests, got %d", got)
	}
}

func TestHandlePort80Request_BypassGlobalTLS_ForwardsNonMatchingAndDisabledRule(t *testing.T) {
	cases := []struct {
		name        string
		ruleEnabled bool
		path        string
	}{
		{
			name:        "non-matching path proxies to upstream",
			ruleEnabled: true,
			path:        "/hello",
		},
		{
			name:        "disabled routing rule does not intercept request",
			ruleEnabled: false,
			path:        "/.well-known/acme-challenge/token-123",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router, upstreamHits, upstream := newTestNonTLSRouter(t, true)
			addTestACMERoutingRule(t, router, tc.ruleEnabled)
			addTestEndpoint(router, "plain.example.com", true, upstream)

			w := httptest.NewRecorder()
			r := newPort80HTTPRequest("plain.example.com", tc.path)
			router.handlePort80Request(w, r)

			if w.Code != http.StatusOK {
				t.Fatalf("expected status %d, got %d (%s)", http.StatusOK, w.Code, w.Body.String())
			}
			if got := w.Body.String(); got != "upstream ok" {
				t.Fatalf("expected upstream body, got %q", got)
			}
			if got := upstreamHits.Load(); got != 1 {
				t.Fatalf("expected 1 upstream hit, got %d", got)
			}
		})
	}
}

func TestHandlePort80Request_NonBypassHost_PreservesExistingBehavior(t *testing.T) {
	t.Run("ForceHttpsRedirect redirects challenge to HTTPS", func(t *testing.T) {
		router, upstreamHits, upstream := newTestNonTLSRouter(t, true)
		addTestACMERoutingRule(t, router, true)
		addTestEndpoint(router, "tls-only.example.com", false, upstream)

		w := httptest.NewRecorder()
		r := newPort80HTTPRequest("tls-only.example.com", "/.well-known/acme-challenge/tok")
		router.handlePort80Request(w, r)

		if w.Code != http.StatusTemporaryRedirect {
			t.Fatalf("expected status %d, got %d", http.StatusTemporaryRedirect, w.Code)
		}
		wantLoc := "https://tls-only.example.com/.well-known/acme-challenge/tok"
		if got := w.Header().Get("Location"); got != wantLoc {
			t.Fatalf("expected Location %q, got %q", wantLoc, got)
		}
		if got := upstreamHits.Load(); got != 0 {
			t.Fatalf("expected 0 upstream hits, got %d", got)
		}
	})

	t.Run("without ForceHttpsRedirect rejects plain HTTP for non-bypass host", func(t *testing.T) {
		router, upstreamHits, upstream := newTestNonTLSRouter(t, false)
		addTestEndpoint(router, "tls-only.example.com", false, upstream)

		w := httptest.NewRecorder()
		r := newPort80HTTPRequest("tls-only.example.com", "/")
		router.handlePort80Request(w, r)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
		if got := upstreamHits.Load(); got != 0 {
			t.Fatalf("expected 0 upstream hits, got %d", got)
		}
	})
}

func TestHandleNonTLSRequest_RoutesSpecialRuleDirectly(t *testing.T) {
	router, upstreamHits, upstream := newTestNonTLSRouter(t, false)
	addTestACMERoutingRule(t, router, true)
	ep := addTestEndpoint(router, "secondary.example.com", true, upstream)

	wChallenge := httptest.NewRecorder()
	rChallenge := newPort80HTTPRequest("secondary.example.com:8080", "/.well-known/acme-challenge/secondary-tok")
	router.handleNonTLSRequest(wChallenge, rChallenge, ep)

	if wChallenge.Code != http.StatusOK || wChallenge.Body.String() != "acme-challenge-ok" {
		t.Fatalf("expected ACME handler response, got %d %q", wChallenge.Code, wChallenge.Body.String())
	}
	if got := upstreamHits.Load(); got != 0 {
		t.Fatalf("expected 0 upstream hits for ACME request, got %d", got)
	}

	wNormal := httptest.NewRecorder()
	rNormal := newPort80HTTPRequest("secondary.example.com:8080", "/normal")
	router.handleNonTLSRequest(wNormal, rNormal, ep)

	if wNormal.Code != http.StatusOK || wNormal.Body.String() != "upstream ok" {
		t.Fatalf("expected upstream response, got %d %q", wNormal.Code, wNormal.Body.String())
	}
	if got := upstreamHits.Load(); got != 1 {
		t.Fatalf("expected 1 upstream hit for normal request, got %d", got)
	}
}
