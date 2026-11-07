package dynamicproxy

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"imuslab.com/zoraxy/mod/auth/sso/oauth2"
	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
	"imuslab.com/zoraxy/mod/dynamicproxy/redirection"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/mtls"
	"imuslab.com/zoraxy/mod/tlscert"
)

func TestServeHTTPMTLSBypassDoesNotForwardSpoofedIdentity(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if websocket.IsWebSocketUpgrade(r) {
			upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				t.Error(err)
				return
			}
			defer conn.Close()
			if err := conn.WriteJSON(r.Header); err != nil {
				t.Error(err)
			}
			return
		}
		_ = json.NewEncoder(w).Encode(r.Header)
	}))
	defer upstream.Close()
	for _, copyAll := range []bool{false, true} {
		name := "default headers"
		if copyAll {
			name = "custom headers enabled"
		}
		t.Run(name, func(t *testing.T) {
			router := newTestRateLimitRouter(t, false)
			router.Option.Logger, _ = logger.NewFmtLogger()
			router.Option.RedirectRuleTable = &redirection.RuleTable{}
			router.Option.MTLSManager = &mtls.Manager{}
			router.ProxyEndpoints = &sync.Map{}
			endpoint := GetDefaultProxyEndpoint()
			endpoint.RootOrMatchingDomain = "app.test"
			endpoint.DisableLogging = true
			endpoint.EnableWebsocketCustomHeaders = copyAll
			endpoint.AuthenticationProvider.AuthMethod = AuthMethodOauth2
			endpoint.TlsOptions = &tlscert.HostSpecificTlsBehavior{MTLSMode: mtls.ModeRequired, MTLSBypassAuth: true}
			endpoint.VirtualDirectories = []*VirtualDirectoryEndpoint{{MatchingPath: "/app", Domain: strings.TrimPrefix(upstream.URL, "http://")}}
			if _, err := router.PrepareProxyRoute(&endpoint); err != nil {
				t.Fatal(err)
			}
			if err := router.AddProxyRouteToRuntime(&endpoint); err != nil {
				t.Fatal(err)
			}
			handler := &ProxyHandler{Parent: router}
			frontend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r.Host = "app.test"
				// Model the state produced by a successful TLS handshake. This
				// test covers routing after verification, not certificate validation.
				cert := &x509.Certificate{SerialNumber: big.NewInt(1)}
				r.TLS = &tls.ConnectionState{ServerName: "app.test", PeerCertificates: []*x509.Certificate{cert}, VerifiedChains: [][]*x509.Certificate{{cert}}}
				handler.ServeHTTP(w, r)
			}))
			defer frontend.Close()
			spoofed := http.Header{}
			for _, key := range oauth2.IdentityHeaderNames {
				spoofed.Set(key, "spoofed-admin")
			}
			spoofed.Set("Cookie", "z-token=secret; app-session=keep")
			check := func(headers http.Header) {
				t.Helper()
				for _, key := range oauth2.IdentityHeaderNames {
					if value := headers.Get(key); value != "" {
						t.Errorf("upstream received %s: %s", key, value)
					}
				}
				if strings.Contains(headers.Get("Cookie"), "z-token=") {
					t.Error("upstream received OAuth2 token cookie")
				}
			}
			req, _ := http.NewRequest(http.MethodGet, frontend.URL+"/app/private", nil)
			req.Header = spoofed.Clone()
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("HTTP upstream status: %d", resp.StatusCode)
			}
			var headers http.Header
			if err := json.NewDecoder(resp.Body).Decode(&headers); err != nil {
				t.Fatal(err)
			}
			check(headers)
			if headers.Get("X-Client-Cert-Verified") != "true" {
				t.Fatal("request did not pass mTLS verification")
			}
			conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(frontend.URL, "http")+"/app/ws", spoofed.Clone())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			if err := conn.ReadJSON(&headers); err != nil {
				t.Fatal(err)
			}
			check(headers)
		})
	}
}

func TestOAuth2MTLSBypassProtectsIdentity(t *testing.T) {
	for _, tc := range []struct {
		name                                                      string
		disableForwarding, allowUntrusted, keepToken, wantHeaders bool
	}{
		{name: "defaults"},
		{name: "forwarding overrides untrusted opt-in", allowUntrusted: true},
		{name: "forwarding disabled", disableForwarding: true},
		{name: "explicit untrusted opt-in", disableForwarding: true, allowUntrusted: true, wantHeaders: true},
		{name: "explicit token opt-in", keepToken: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, websocket := range []bool{false, true} {
				provider := &AuthenticationProvider{
					AuthMethod:                          AuthMethodOauth2,
					OAuth2DisableIdentityForwarding:     tc.disableForwarding,
					OAuth2AllowUntrustedIdentityHeaders: tc.allowUntrusted,
					OAuth2DisableTokenCookieRemoval:     tc.keepToken,
				}
				r := httptest.NewRequest(http.MethodGet, "https://app.test/private", nil)
				for _, name := range oauth2.IdentityHeaderNames {
					r.Header.Set(name, "spoofed-admin")
				}
				if websocket {
					r.Header.Set("Upgrade", "websocket")
				}
				r.Header.Set("X-Client-Cert-Verified", "true")
				r.AddCookie(&http.Cookie{Name: "z-token", Value: "secret"})
				r.AddCookie(&http.Cookie{Name: "z-oauth-state", Value: "state"})
				r.AddCookie(&http.Cookie{Name: "app-session", Value: "keep"})
				// This is the auth routing entry point used by ServeHTTP after a
				// verified mTLS certificate with BypassAuth enabled.
				if handleAuthProviderRouting(&ProxyEndpoint{AuthenticationProvider: provider}, httptest.NewRecorder(), r, nil, true) {
					t.Fatal("verified mTLS bypass should proceed to the upstream")
				}
				for _, name := range oauth2.IdentityHeaderNames {
					if got := r.Header.Get(name) != ""; got != tc.wantHeaders {
						t.Errorf("%s preserved=%v, want %v", name, got, tc.wantHeaders)
					}
				}
				if shouldForwardOAuth2WebSocketIdentity(r, provider) || oauth2.HasVerifiedIdentityHeaders(r) {
					t.Fatal("mTLS bypass must not establish a verified OAuth2 identity")
				}
				if _, err := r.Cookie("z-token"); (err == nil) != tc.keepToken {
					t.Error("token cookie did not follow the route policy")
				}
				if _, err := r.Cookie("z-oauth-state"); err == nil {
					t.Error("OAuth2 state leaked to the upstream")
				}
				if _, err := r.Cookie("app-session"); err != nil || r.Header.Get("X-Client-Cert-Verified") != "true" {
					t.Error("unrelated cookies or certificate evidence were removed")
				}
			}
		})
	}
}

func TestOAuth2ReplacementRebindsVirtualDirectories(t *testing.T) {
	original := &ProxyEndpoint{AuthenticationProvider: &AuthenticationProvider{
		AuthMethod: AuthMethodOauth2, OAuth2DisableIdentityForwarding: true,
	}}
	runtimeProxy := &dpcore.ReverseProxy{}
	vdir := &VirtualDirectoryEndpoint{MatchingPath: "/app", Domain: "backend.test", proxy: runtimeProxy, parent: original}
	original.VirtualDirectories = []*VirtualDirectoryEndpoint{vdir}
	provider := *original.AuthenticationProvider
	provider.OAuth2DisableIdentityForwarding = false
	replacement := original.CloneWithAuthenticationProvider(&provider)
	cloned := replacement.GetVirtualDirectoryRuleByMatchingPath("/app")
	if cloned == vdir || cloned.parent != replacement || cloned.proxy != runtimeProxy {
		t.Fatal("virtual directory must be copied, rebound, and retain its runtime proxy")
	}
	if vdir.parent != original || original.VirtualDirectories[0] != vdir || !vdir.parent.AuthenticationProvider.OAuth2DisableIdentityForwarding {
		t.Fatal("original virtual directory or its policy was mutated")
	}

	// Obtain real request-scoped evidence through the OAuth2 handler, then check
	// that the replacement vdir uses its new policy for WebSocket forwarding.
	userInfo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"sub":"alice","preferred_username":"alice"}`))
	}))
	defer userInfo.Close()
	db, err := database.NewDatabase(filepath.Join(t.TempDir(), "oauth2.db"), database.GetRecommendedBackendType())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	log, _ := logger.NewFmtLogger()
	options := &oauth2.OAuth2RouterOptions{
		Database: db, Logger: log, OAuth2ServerURL: "https://issuer.test/auth",
		OAuth2TokenURL: "https://issuer.test/token", OAuth2UserInfoUrl: userInfo.URL, OAuth2Scopes: "profile",
	}
	authRouter := oauth2.NewOAuth2Router(options)
	defer options.OAuth2ConfigCache.Stop()
	handler := &ProxyHandler{Parent: &Router{Option: &RouterOption{OAuth2Router: authRouter}}}
	r := httptest.NewRequest(http.MethodGet, "https://app.test/app/ws", nil)
	r.Header.Set("Upgrade", "websocket")
	r.Header.Set("Remote-User", "spoofed")
	r.AddCookie(&http.Cookie{Name: "z-token", Value: "valid"})
	if err := handler.handleOAuth2Auth(httptest.NewRecorder(), r, replacement); err != nil {
		t.Fatal(err)
	}
	if r.Header.Get("Remote-User") != "alice" || !shouldForwardOAuth2WebSocketIdentity(r, cloned.parent.AuthenticationProvider) {
		t.Fatal("replacement vdir did not enable forwarding the authenticated identity")
	}
	if shouldForwardOAuth2WebSocketIdentity(r, vdir.parent.AuthenticationProvider) {
		t.Fatal("original disabled policy changed")
	}
	provider.OAuth2ForwardWebSocketIdentity = new(false)
	if shouldForwardOAuth2WebSocketIdentity(r, &provider) {
		t.Fatal("WebSocket forwarding opt-out ignored")
	}
}
