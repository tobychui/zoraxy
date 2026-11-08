package oauth2

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	xoauth2 "golang.org/x/oauth2"
)

func requireSessionCookie(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/private" {
		t.Fatalf("callback failed: %d %s", w.Code, w.Body.String())
	}
	var session *http.Cookie
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == "z-token" && cookie.MaxAge >= 0 {
			t.Fatal("callback exposed the provider token to the browser")
		}
		if cookie.Name == sessionCookieName {
			session = cookie
		}
	}
	if session == nil || len(session.Value) != 43 || !session.HttpOnly || !session.Secure ||
		session.Path != "/" || session.Domain != "" || session.SameSite != http.SameSiteLaxMode ||
		!session.Expires.After(time.Now()) || session.Expires.After(time.Now().Add(time.Hour)) {
		t.Fatalf("invalid session cookie: %+v", session)
	}
	return session
}

type sessionTestProvider struct {
	server *httptest.Server
	calls  atomic.Int32
	claims atomic.Value
	status atomic.Int32
}

func newSessionTestProvider(t *testing.T) (*OAuth2Router, *sessionTestProvider) {
	t.Helper()
	p := &sessionTestProvider{}
	p.claims.Store(`{"id":42,"preferred_username":"alice","groups":["admins"]}`)
	p.status.Store(http.StatusOK)
	p.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/token" {
			_, _ = w.Write([]byte(`{"access_token":"provider-token","token_type":"Bearer","expires_in":3600}`))
			return
		}
		p.calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer provider-token" {
			http.Error(w, "unexpected token", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(int(p.status.Load()))
		_, _ = w.Write([]byte(p.claims.Load().(string)))
	}))
	t.Cleanup(p.server.Close)
	router := newSettingsTestRouter(t)
	router.options.OAuth2ClientId = "client"
	router.options.OAuth2ServerURL = p.server.URL + "/authorize"
	router.options.OAuth2TokenURL = p.server.URL + "/token"
	router.options.OAuth2UserInfoUrl = p.server.URL + "/userinfo"
	router.options.OAuth2Scopes = "profile"
	return router, p
}

func sessionTestLogin(t *testing.T, router *OAuth2Router) *http.Cookie {
	t.Helper()
	start := httptest.NewRecorder()
	_ = router.HandleOAuth2Auth(start, httptest.NewRequest(http.MethodGet, "https://app.test/private", nil), nil)
	authURL, err := url.Parse(start.Header().Get("Location"))
	if err != nil || start.Code != http.StatusFound {
		t.Fatalf("starting login: %d %v", start.Code, err)
	}
	r := httptest.NewRequest(http.MethodGet, "https://app.test/internal/oauth2?code=valid&state="+url.QueryEscape(authURL.Query().Get("state")), nil)
	for _, cookie := range start.Result().Cookies() {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	_ = router.HandleOAuth2Auth(w, r, nil)
	return requireSessionCookie(t, w)
}

func TestSuppliedBearerTokenCannotAuthenticate(t *testing.T) {
	for _, oidc := range []bool{false, true} {
		router, provider := newSessionTestProvider(t)
		if oidc {
			router.options.OAuth2Scopes = "openid"
			router.options.OAuth2WellKnownUrl = provider.server.URL + "/discovery"
		}
		for _, name := range []string{"z-token", sessionCookieName} {
			r := httptest.NewRequest(http.MethodGet, "https://app.test/private", nil)
			r.AddCookie(&http.Cookie{Name: name, Value: "provider-token"})
			w := httptest.NewRecorder()
			if err := router.HandleOAuth2Auth(w, r, nil); err == nil || w.Code != http.StatusFound ||
				HasVerifiedIdentityHeaders(r) || r.Header.Get("Remote-User") != "" || provider.calls.Load() != 0 {
				t.Fatalf("supplied %s authenticated or reached UserInfo (OIDC=%v)", name, oidc)
			}
		}
	}
}

func TestSessionIdentityGroupsAndUpstreamCookies(t *testing.T) {
	router, provider := newSessionTestProvider(t)
	cookie := sessionTestLogin(t, router)
	for _, websocket := range []bool{false, true} {
		for _, removeToken := range []bool{false, true} {
			r := httptest.NewRequest(http.MethodGet, "https://app.test/private", nil)
			r.AddCookie(cookie)
			r.AddCookie(&http.Cookie{Name: "z-token", Value: "injected-token"})
			r.AddCookie(&http.Cookie{Name: "application", Value: "keep"})
			if websocket {
				r.Header.Set("Upgrade", "websocket")
			}
			policy := &AccessPolicy{ForwardUserInfo: true, ForwardWebSocketIdentity: true, RemoveTokenCookie: removeToken, AllowedGroups: []string{"admins"}}
			if err := router.HandleOAuth2Auth(httptest.NewRecorder(), r, policy); err != nil {
				t.Fatal(err)
			}
			if !HasVerifiedIdentityHeaders(r) || r.Header.Get("Remote-User") != "alice" {
				t.Fatal("session did not establish verified identity")
			}
			if _, err := r.Cookie(sessionCookieName); err == nil {
				t.Fatal("session credential leaked upstream")
			}
			token, err := r.Cookie("z-token")
			if (err == nil) == removeToken || (!removeToken && token.Value != "provider-token") || strings.Contains(r.Header.Get("Cookie"), "injected-token") {
				t.Fatal("upstream token policy used client-supplied credentials")
			}
			if _, err := r.Cookie("application"); err != nil {
				t.Fatal("application cookie was removed")
			}
		}
	}
	provider.claims.Store(`{"id":42,"groups":["guests"]}`)
	r := httptest.NewRequest(http.MethodGet, "https://app.test/private", nil)
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	if err := router.HandleOAuth2Auth(w, r, &AccessPolicy{AllowedGroups: []string{"admins"}}); err == nil || w.Code != http.StatusForbidden {
		t.Fatal("session cached obsolete group membership")
	}
	provider.status.Store(http.StatusUnauthorized)
	r = httptest.NewRequest(http.MethodGet, "https://app.test/private", nil)
	r.AddCookie(cookie)
	if err := router.HandleOAuth2Auth(httptest.NewRecorder(), r, nil); err == nil || HasVerifiedIdentityHeaders(r) {
		t.Fatal("session accepted a revoked provider token")
	}
}

func TestSessionInvalidation(t *testing.T) {
	for _, kind := range []string{"unknown", "tampered", "expired", "host", "scheme", "settings", "delete", "restart"} {
		t.Run(kind, func(t *testing.T) {
			router, provider := newSessionTestProvider(t)
			cookie := sessionTestLogin(t, router)
			target := "https://app.test/private"
			switch kind {
			case "unknown":
				cookie.Value = xoauth2.GenerateVerifier()
			case "tampered":
				cookie.Value += "x"
			case "expired":
				session := router.sessions.Get(cookie.Value).Value()
				session.expires = time.Now().Add(-time.Second)
				router.sessions.Set(cookie.Value, session, time.Hour)
			case "host":
				target = "https://other.test/private"
			case "scheme":
				target = "http://app.test/private"
			case "settings":
				saveTestSettings(t, router, provider.server.URL+"/userinfo", "admins")
			case "delete":
				router.HandleSetOAuth2Settings(httptest.NewRecorder(), httptest.NewRequest(http.MethodDelete, "/settings", nil))
				saveTestSettings(t, router, provider.server.URL+"/userinfo", "admins")
			case "restart":
				router = newSettingsTestRouter(t)
				saveTestSettings(t, router, provider.server.URL+"/userinfo", "admins")
			}
			before := provider.calls.Load()
			r := httptest.NewRequest(http.MethodGet, target, nil)
			r.AddCookie(cookie)
			w := httptest.NewRecorder()
			if err := router.HandleOAuth2Auth(w, r, nil); err == nil || w.Code != http.StatusFound || HasVerifiedIdentityHeaders(r) || provider.calls.Load() != before {
				t.Fatal("invalid session authenticated or reached UserInfo")
			}
		})
	}
}

func TestSessionAbsoluteExpiryAndStaleCallback(t *testing.T) {
	router, _ := newSessionTestProvider(t)
	r := httptest.NewRequest(http.MethodGet, "https://app.test/internal/oauth2", nil)
	expires := time.Now().Add(5 * time.Minute)
	cookie, err := router.createSession(r, &xoauth2.Token{AccessToken: "token", Expiry: expires}, "alice", 0)
	if err != nil || !cookie.Expires.Equal(expires) {
		t.Fatalf("session exceeded token expiry: %v", err)
	}
	r.AddCookie(cookie)
	before := router.sessions.Get(cookie.Value).ExpiresAt()
	for range 10 {
		if _, valid := router.sessionForRequest(r, 0); !valid {
			t.Fatal("session expired prematurely")
		}
	}
	if !router.sessions.Get(cookie.Value).ExpiresAt().Equal(before) {
		t.Fatal("traffic extended session lifetime")
	}
	saveTestSettings(t, router, "https://issuer.test/userinfo", "admins")
	if _, err := router.createSession(r, &xoauth2.Token{AccessToken: "token"}, "alice", 0); err == nil || router.sessions.Len() != 0 {
		t.Fatal("callback from old settings created a session")
	}
}
