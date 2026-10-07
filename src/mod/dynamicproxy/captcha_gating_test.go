package dynamicproxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"imuslab.com/zoraxy/mod/dynamicproxy/captcha"
)

var spoofableClientIPHeaders = []string{"X-Real-Ip", "CF-Connecting-IP", "Fastly-Client-IP", "X-Forwarded-For"}

// newTestCaptchaRouter builds a test router with a CAPTCHA session store, using
// the same access controller setup as the rate limit tests
func newTestCaptchaRouter(t *testing.T, trustProxyHeaders bool, trustedProxies ...string) *Router {
	t.Helper()
	router := newTestRateLimitRouter(t, trustProxyHeaders, trustedProxies...)
	router.captchaSessionStore = captcha.NewSessionStore()
	t.Cleanup(router.captchaSessionStore.Close)
	return router
}

func newTestCaptchaEndpoint(exceptionCIDR string) *ProxyEndpoint {
	config := &captcha.Config{
		Provider:  captcha.ProviderCloudflare,
		SiteKey:   "test-site-key",
		SecretKey: "test-secret-key",
	}
	if exceptionCIDR != "" {
		config.ExceptionRules = []*captcha.ExceptionRule{{
			RuleType: captcha.ExceptionTypeCIDR,
			CIDR:     exceptionCIDR,
		}}
	}
	return &ProxyEndpoint{
		RootOrMatchingDomain: "host.example.com",
		RequireCaptcha:       true,
		CaptchaConfig:        config,
	}
}

// A direct client must not be able to skip the challenge by claiming an
// exempted address in a forwarding header (#1321)
func TestHandleCaptchaGating_SpoofedHeaderDoesNotMatchCIDRException(t *testing.T) {
	for _, headerName := range spoofableClientIPHeaders {
		t.Run(headerName, func(t *testing.T) {
			router := newTestCaptchaRouter(t, false)
			w := httptest.NewRecorder()
			r := newTestRequest("203.0.113.5:44321", map[string]string{headerName: "10.1.2.3"})

			handled, challenged := router.handleCaptchaGating(w, r, newTestCaptchaEndpoint("10.0.0.0/8"))
			if !handled || !challenged {
				t.Fatalf("spoofed %s header skipped the CAPTCHA challenge (handled=%v, challenged=%v)", headerName, handled, challenged)
			}
			if w.Code != http.StatusForbidden {
				t.Errorf("expected status 403 for the challenge page, got %d", w.Code)
			}
		})
	}
}

// Trusting proxy headers on the access rule is not enough. The direct peer
// must also be in the trusted proxy list.
func TestHandleCaptchaGating_UntrustedPeerHeaderDoesNotMatchCIDRException(t *testing.T) {
	for _, headerName := range spoofableClientIPHeaders {
		t.Run(headerName, func(t *testing.T) {
			router := newTestCaptchaRouter(t, true, "104.16.0.0/13")
			r := newTestRequest("203.0.113.5:44321", map[string]string{headerName: "10.1.2.3"})

			if _, challenged := router.handleCaptchaGating(httptest.NewRecorder(), r, newTestCaptchaEndpoint("10.0.0.0/8")); !challenged {
				t.Fatalf("%s header from an untrusted peer skipped the CAPTCHA challenge", headerName)
			}
		})
	}
}

// CDN fronted deployments that opted in to trusted proxy headers keep their
// IP exceptions working
func TestHandleCaptchaGating_TrustedProxyHeaderMatchesCIDRException(t *testing.T) {
	router := newTestCaptchaRouter(t, true, "104.16.0.0/13")
	r := newTestRequest("104.16.5.1:443", map[string]string{"CF-Connecting-IP": "10.1.2.3"})

	handled, challenged := router.handleCaptchaGating(httptest.NewRecorder(), r, newTestCaptchaEndpoint("10.0.0.0/8"))
	if handled || challenged {
		t.Fatalf("client behind a trusted proxy in an exempted range was challenged (handled=%v, challenged=%v)", handled, challenged)
	}
}

func TestHandleCaptchaGating_DirectPeerMatchesCIDRException(t *testing.T) {
	router := newTestCaptchaRouter(t, false)
	r := newTestRequest("10.1.2.3:44321", nil)

	handled, challenged := router.handleCaptchaGating(httptest.NewRecorder(), r, newTestCaptchaEndpoint("10.0.0.0/8"))
	if handled || challenged {
		t.Fatalf("direct peer in an exempted range was challenged (handled=%v, challenged=%v)", handled, challenged)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// stubCaptchaProvider replaces the default HTTP client transport for the
// duration of the test and returns a pointer to the form values that were
// sent to the CAPTCHA provider's verification API
func stubCaptchaProvider(t *testing.T) *url.Values {
	t.Helper()
	sent := &url.Values{}
	originalTransport := http.DefaultClient.Transport
	http.DefaultClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		*sent, _ = url.ParseQuery(string(body))
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"success":true}`)),
			Request:    r,
		}, nil
	})
	t.Cleanup(func() { http.DefaultClient.Transport = originalTransport })
	return sent
}

func newTestCaptchaVerifyRequest(remoteAddr string, headers map[string]string) *http.Request {
	form := url.Values{"cf-turnstile-response": {"test-token"}}
	r := httptest.NewRequest(http.MethodPost, "https://host.example.com"+captcha.VerifyPath, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.RemoteAddr = remoteAddr
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

// The remoteip reported to the CAPTCHA provider must be the resolved client IP,
// not an address chosen by the client in a forwarding header (#1321)
func TestHandleCaptchaGating_VerificationSendsResolvedRemoteIP(t *testing.T) {
	for _, headerName := range spoofableClientIPHeaders {
		t.Run(headerName, func(t *testing.T) {
			sent := stubCaptchaProvider(t)
			router := newTestCaptchaRouter(t, false)
			w := httptest.NewRecorder()
			r := newTestCaptchaVerifyRequest("203.0.113.5:44321", map[string]string{headerName: "10.1.2.3"})

			if handled, _ := router.handleCaptchaGating(w, r, newTestCaptchaEndpoint("")); !handled {
				t.Fatal("verification request was not handled")
			}
			if w.Code != http.StatusOK {
				t.Fatalf("expected verification to succeed with status 200, got %d", w.Code)
			}
			if got := sent.Get("remoteip"); got != "203.0.113.5" {
				t.Errorf("remoteip sent to provider = %q, expected the connection address 203.0.113.5", got)
			}
		})
	}
}

func TestHandleCaptchaGating_VerificationSendsTrustedProxyClientIP(t *testing.T) {
	sent := stubCaptchaProvider(t)
	router := newTestCaptchaRouter(t, true, "104.16.0.0/13")
	r := newTestCaptchaVerifyRequest("104.16.5.1:443", map[string]string{"CF-Connecting-IP": "198.51.100.7"})

	if handled, _ := router.handleCaptchaGating(httptest.NewRecorder(), r, newTestCaptchaEndpoint("")); !handled {
		t.Fatal("verification request was not handled")
	}
	if got := sent.Get("remoteip"); got != "198.51.100.7" {
		t.Errorf("remoteip sent to provider = %q, expected the trusted proxy client IP 198.51.100.7", got)
	}
}
