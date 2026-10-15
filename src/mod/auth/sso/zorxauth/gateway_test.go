package zorxauth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"imuslab.com/zoraxy/mod/database"
)

// newTestGateway creates an AuthRouter backed by a temporary database with the
// auth gateway enabled and a single user that is allowed to access all hosts.
func newTestGateway(t *testing.T) (*AuthRouter, *GatewayServer) {
	t.Helper()
	db, err := database.NewDatabase(filepath.Join(t.TempDir(), "zorxauth_test.db"), database.GetRecommendedBackendType())
	if err != nil {
		t.Fatalf("unable to create test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.NewTable(DB_USERS_TABLE)

	options := getDefaultOptions()
	options.EnableAuthGateway = true
	ar := &AuthRouter{
		Database: db,
		Options:  options,
	}
	if err := db.Write(DB_USERS_TABLE, ar.userDbKey("alice"), &User{ID: "alice-id", Username: "alice"}); err != nil {
		t.Fatalf("unable to write test user: %v", err)
	}
	return ar, NewGatewayServer(ar)
}

func fastPathRedirect(t *testing.T, ar *AuthRouter, gs *GatewayServer, rememberMe bool) *url.URL {
	t.Helper()
	token := "gateway-session-token"
	ar.gatewaySessionStore.Store(token, &GatewaySession{
		Username:   "alice",
		Expiry:     time.Now().Add(time.Hour),
		RememberMe: rememberMe,
	})

	req := httptest.NewRequest(http.MethodGet, "/?redirect="+url.QueryEscape("https://app-b.example.com/"), nil)
	req.AddCookie(&http.Cookie{Name: ar.Options.CookieName, Value: token})
	rec := httptest.NewRecorder()
	gs.handleAuthPage(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusSeeOther, rec.Code, rec.Body.String())
	}
	location, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatalf("invalid Location header %q: %v", rec.Header().Get("Location"), err)
	}
	if location.Host != "app-b.example.com" || location.Query().Get("session_id") == "" {
		t.Fatalf("unexpected session set redirect: %s", location.String())
	}
	return location
}

func TestHandleAuthPageFastPathCarriesRememberMe(t *testing.T) {
	ar, gs := newTestGateway(t)
	location := fastPathRedirect(t, ar, gs, true)
	if got := location.Query().Get("remember_me"); got != "true" {
		t.Fatalf("expected remember_me=true in session set redirect, got %q (%s)", got, location.String())
	}
}

// storedGatewaySession returns the gateway session currently stored for the
// token used by fastPathRedirect.
func storedGatewaySession(t *testing.T, ar *AuthRouter) *GatewaySession {
	t.Helper()
	sessionData, exists := ar.gatewaySessionStore.Load("gateway-session-token")
	if !exists {
		t.Fatalf("gateway session was not found in the session store")
	}
	gatewaySession, ok := sessionData.(*GatewaySession)
	if !ok {
		t.Fatalf("unexpected gateway session type %T", sessionData)
	}
	return gatewaySession
}

func TestHandleAuthPageFastPathRenewsRememberMeSessionWithRememberMeDuration(t *testing.T) {
	ar, gs := newTestGateway(t)
	before := time.Now()
	fastPathRedirect(t, ar, gs, true)

	session := storedGatewaySession(t, ar)
	minExpiry := before.Add(time.Duration(ar.Options.CookieDurationRememberMe) * time.Second)
	if session.Expiry.Before(minExpiry) {
		t.Fatalf("expected remembered gateway session to be renewed for at least %ds, got expiry %s (now %s)",
			ar.Options.CookieDurationRememberMe, session.Expiry, before)
	}
	if !session.RememberMe {
		t.Fatalf("expected gateway session to keep RememberMe after renewal")
	}
}

func TestHandleAuthPageFastPathRenewsPlainSessionWithCookieDuration(t *testing.T) {
	ar, gs := newTestGateway(t)
	before := time.Now()
	fastPathRedirect(t, ar, gs, false)

	session := storedGatewaySession(t, ar)
	minExpiry := before.Add(time.Duration(ar.Options.CookieDuration) * time.Second)
	maxExpiry := time.Now().Add(time.Duration(ar.Options.CookieDuration) * time.Second)
	if session.Expiry.Before(minExpiry) || session.Expiry.After(maxExpiry) {
		t.Fatalf("expected plain gateway session to be renewed for %ds, got expiry %s (now %s)",
			ar.Options.CookieDuration, session.Expiry, before)
	}
}

func TestHandleAuthPageFastPathWithoutRememberMe(t *testing.T) {
	ar, gs := newTestGateway(t)
	location := fastPathRedirect(t, ar, gs, false)
	if _, present := location.Query()["remember_me"]; present {
		t.Fatalf("did not expect remember_me in session set redirect, got %s", location.String())
	}
}
