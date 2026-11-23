package zorxauth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"imuslab.com/zoraxy/mod/database"
)

// newTestAuthRouter builds an AuthRouter on a throwaway database without starting the gateway
func newTestAuthRouter(t *testing.T) *AuthRouter {
	t.Helper()
	db, err := database.NewDatabase(filepath.Join(t.TempDir(), "sysdb"), database.GetRecommendedBackendType())
	if err != nil {
		t.Fatalf("unable to create test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	db.NewTable(DB_USERS_TABLE)
	db.NewTable(DB_BROWSER_SESSIONS_TABLE)
	db.NewTable(DB_GATEWAY_SESSIONS_TABLE)
	return &AuthRouter{Database: db, Options: getDefaultOptions()}
}

// restartAuthRouter simulates a Zoraxy restart: a new AuthRouter on the same
// database that loads the persisted sessions, as NewAuthRouter does
func restartAuthRouter(old *AuthRouter) *AuthRouter {
	ar := &AuthRouter{Database: old.Database, Options: getDefaultOptions()}
	ar.loadBrowserSessions()
	ar.loadGatewaySessions()
	return ar
}

// sessionIDs lists the IDs held by a session store
func sessionIDs(store interface{ Range(func(any, any) bool) }) map[string]bool {
	ids := map[string]bool{}
	store.Range(func(key, _ any) bool {
		ids[key.(string)] = true
		return true
	})
	return ids
}

// loginSessions creates a browser and a gateway session for user through the
// same helpers the login handlers use
func loginSessions(ar *AuthRouter, user *User, browserID, gatewayID string) {
	expiry := time.Now().Add(time.Hour)
	ar.storeBrowserSession(browserID, &BrowserSession{Username: user.Username, UserID: user.ID, Expiry: expiry})
	ar.storeGatewaySession(gatewayID, &GatewaySession{Username: user.Username, UserID: user.ID, Expiry: expiry})
}

func TestLoggedOutSessionDoesNotReviveAfterGracefulRestart(t *testing.T) {
	run1 := newTestAuthRouter(t)
	alice := mustCreateUser(t, run1, "alice")
	bob := mustCreateUser(t, run1, "bob")
	loginSessions(run1, alice, "alice-b", "alice-g")
	loginSessions(run1, bob, "bob-b", "bob-g")
	run1.Close()

	// Second run: both users are restored, then alice logs out everywhere
	run2 := restartAuthRouter(run1)
	if !sessionIDs(&run2.cookieIdStore)["alice-b"] || !sessionIDs(&run2.gatewaySessionStore)["alice-g"] {
		t.Fatalf("live sessions were not restored after a graceful restart")
	}
	run2.removeGatewaySession("alice-g")
	run2.deleteBrowserSessionsOfUser(alice.ID)
	run2.Close()

	run3 := restartAuthRouter(run2)
	if got := sessionIDs(&run3.cookieIdStore); got["alice-b"] || !got["bob-b"] {
		t.Errorf("browser sessions after restart = %v, want only bob-b", got)
	}
	if got := sessionIDs(&run3.gatewaySessionStore); got["alice-g"] || !got["bob-g"] {
		t.Errorf("gateway sessions after restart = %v, want only bob-g", got)
	}
}

// Zoraxy can be killed before Close() runs (e.g. a stop timeout while mDNS shuts down)
func TestSessionsAreWrittenThroughWithoutClose(t *testing.T) {
	run1 := newTestAuthRouter(t)
	alice := mustCreateUser(t, run1, "alice")
	bob := mustCreateUser(t, run1, "bob")
	loginSessions(run1, alice, "alice-b", "alice-g")
	loginSessions(run1, bob, "bob-b", "bob-g")

	// alice logs out, then the process dies without Close()
	run1.removeGatewaySession("alice-g")
	run1.deleteBrowserSessionsOfUser(alice.ID)

	run2 := restartAuthRouter(run1)
	if got := sessionIDs(&run2.cookieIdStore); got["alice-b"] || !got["bob-b"] {
		t.Errorf("browser sessions after crash = %v, want only bob-b", got)
	}
	if got := sessionIDs(&run2.gatewaySessionStore); got["alice-g"] || !got["bob-g"] {
		t.Errorf("gateway sessions after crash = %v, want only bob-g", got)
	}
}

func TestDeletedUserSessionsAreRemovedFromDatabase(t *testing.T) {
	ar := newTestAuthRouter(t)
	alice := mustCreateUser(t, ar, "alice")
	loginSessions(ar, alice, "alice-b", "alice-g")

	ar.deleteAllSessionsOfUser(alice.ID)

	if ar.Database.KeyExists(DB_BROWSER_SESSIONS_TABLE, DB_BROWSER_SESSION_KEY_PREFIX+"alice-b") ||
		ar.Database.KeyExists(DB_GATEWAY_SESSIONS_TABLE, DB_GATEWAY_SESSION_KEY_PREFIX+"alice-g") {
		t.Errorf("sessions of a deleted user are still persisted")
	}
}

// A rename refresh must not bring back a session that was removed after it was read
func TestReplaceSessionDoesNotReviveRemovedSession(t *testing.T) {
	ar := newTestAuthRouter(t)
	alice := mustCreateUser(t, ar, "alice")
	loginSessions(ar, alice, "alice-b", "alice-g")
	old, _ := ar.cookieIdStore.Load("alice-b")

	ar.removeBrowserSession("alice-b")
	ar.replaceBrowserSession("alice-b", old.(*BrowserSession), &BrowserSession{Username: "alice2", UserID: alice.ID, Expiry: time.Now().Add(time.Hour)})

	if _, ok := ar.cookieIdStore.Load("alice-b"); ok {
		t.Errorf("replace revived a removed session in memory")
	}
	if ar.Database.KeyExists(DB_BROWSER_SESSIONS_TABLE, DB_BROWSER_SESSION_KEY_PREFIX+"alice-b") {
		t.Errorf("replace revived a removed session in the database")
	}
}

func mustCreateUser(t *testing.T, ar *AuthRouter, username string) *User {
	t.Helper()
	user := &User{Username: username, PasswordHash: hashPassword("secret")}
	if err := ar.saveUser(user, ""); err != nil {
		t.Fatalf("saveUser(%q): %v", username, err)
	}
	return user
}

// requestWithBrowserSession stores a browser session for user and returns a request carrying its cookie
func requestWithBrowserSession(ar *AuthRouter, sessionID string, session *BrowserSession) *http.Request {
	ar.cookieIdStore.Store(sessionID, session)
	r := httptest.NewRequest(http.MethodGet, "http://app.example.com/", nil)
	r.AddCookie(&http.Cookie{Name: ar.Options.CookieName, Value: sessionID})
	return r
}

func TestBrowserSessionFollowsRename(t *testing.T) {
	ar := newTestAuthRouter(t)
	user := mustCreateUser(t, ar, "alice")
	r := requestWithBrowserSession(ar, "s1", &BrowserSession{Username: "alice", UserID: user.ID, Expiry: time.Now().Add(time.Hour)})

	user.Username = "alice2"
	if err := ar.saveUser(user, "alice"); err != nil {
		t.Fatalf("rename: %v", err)
	}

	got, err := ar.GetUserFromRequest(httptest.NewRecorder(), r)
	if err != nil {
		t.Fatalf("session should survive a rename: %v", err)
	}
	if got.ID != user.ID || got.Username != "alice2" {
		t.Fatalf("got %s/%s, want %s/alice2", got.ID, got.Username, user.ID)
	}

	// The lookup hint is refreshed so later requests read the record directly
	stored, _ := ar.cookieIdStore.Load("s1")
	if s := stored.(*BrowserSession); s.Username != "alice2" || s.UserID != user.ID {
		t.Fatalf("session hint not refreshed: %+v", s)
	}
}

func TestSessionOfDeletedUserRejectedWhenUsernameReused(t *testing.T) {
	ar := newTestAuthRouter(t)
	oldUser := mustCreateUser(t, ar, "alice")
	// Simulates a session that outlived its user (e.g. restored from the session table)
	r := requestWithBrowserSession(ar, "s1", &BrowserSession{Username: "alice", UserID: oldUser.ID, Expiry: time.Now().Add(time.Hour)})

	if err := ar.Database.Delete(DB_USERS_TABLE, ar.userDbKey("alice")); err != nil {
		t.Fatalf("delete: %v", err)
	}
	newUser := mustCreateUser(t, ar, "alice")
	if newUser.ID == oldUser.ID {
		t.Fatalf("re-created user must get a new ID")
	}

	if got, err := ar.GetUserFromRequest(httptest.NewRecorder(), r); err == nil {
		t.Fatalf("stale session authenticated as %s/%s", got.ID, got.Username)
	}
	if _, exists := ar.cookieIdStore.Load("s1"); exists {
		t.Fatalf("rejected session should be removed from the store")
	}
}

func TestLegacySessionWithoutUserIDRejected(t *testing.T) {
	ar := newTestAuthRouter(t)
	mustCreateUser(t, ar, "alice")
	r := requestWithBrowserSession(ar, "s1", &BrowserSession{Username: "alice", Expiry: time.Now().Add(time.Hour)})

	if _, err := ar.GetUserFromRequest(httptest.NewRecorder(), r); err == nil {
		t.Fatalf("session without user ID must be rejected")
	}
}

func TestGatewaySessionFollowsRenameAndRejectsDeletedUser(t *testing.T) {
	ar := newTestAuthRouter(t)
	user := mustCreateUser(t, ar, "bob")
	ar.gatewaySessionStore.Store("g1", &GatewaySession{Username: "bob", UserID: user.ID, Expiry: time.Now().Add(time.Hour)})
	r := httptest.NewRequest(http.MethodGet, "http://auth.example.com/", nil)
	r.AddCookie(&http.Cookie{Name: ar.Options.CookieName, Value: "g1"})

	user.Username = "robert"
	if err := ar.saveUser(user, "bob"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	if ok, name := ar.RequestIsAuthenticatedInSSO(httptest.NewRecorder(), r); !ok || name != "robert" {
		t.Fatalf("got (%v, %q), want (true, robert)", ok, name)
	}

	ar.Database.Delete(DB_USERS_TABLE, ar.userDbKey("robert"))
	mustCreateUser(t, ar, "robert")
	if ok, name := ar.RequestIsAuthenticatedInSSO(httptest.NewRecorder(), r); ok {
		t.Fatalf("gateway session of a deleted user authenticated as %q", name)
	}
}

func TestSessionSetCallbackBindsBrowserSessionToUserID(t *testing.T) {
	ar := newTestAuthRouter(t)
	user := mustCreateUser(t, ar, "carol")
	code := ar.generateValidationCodeForUser(user)

	r := httptest.NewRequest(http.MethodGet, "http://app.example.com"+ar.Options.SSOSessionSetURL+"?session_id="+code+"&redirect=/", nil)
	w := httptest.NewRecorder()
	ar.handleSessionSetCallback(w, r)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("callback status %d, want %d", w.Code, http.StatusSeeOther)
	}

	var cookieValue string
	for _, c := range w.Result().Cookies() {
		if c.Name == ar.Options.CookieName {
			cookieValue = c.Value
		}
	}
	stored, ok := ar.cookieIdStore.Load(cookieValue)
	if !ok || stored.(*BrowserSession).UserID != user.ID {
		t.Fatalf("browser session not bound to user ID: %+v", stored)
	}

	// The validation code is one-time use
	w = httptest.NewRecorder()
	ar.handleSessionSetCallback(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("reused validation code status %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleUserDeleteRemovesAllSessionsOfUser(t *testing.T) {
	ar := newTestAuthRouter(t)
	alice := mustCreateUser(t, ar, "alice")
	bob := mustCreateUser(t, ar, "bob")
	expiry := time.Now().Add(time.Hour)
	ar.cookieIdStore.Store("a-browser", &BrowserSession{Username: "alice", UserID: alice.ID, Expiry: expiry})
	ar.gatewaySessionStore.Store("a-gateway", &GatewaySession{Username: "alice", UserID: alice.ID, Expiry: expiry})
	aliceCode := ar.generateValidationCodeForUser(alice)
	ar.cookieIdStore.Store("b-browser", &BrowserSession{Username: "bob", UserID: bob.ID, Expiry: expiry})

	form := url.Values{"username": {"alice"}}
	r := httptest.NewRequest(http.MethodPost, "/api/sso/zorxauth/users/delete", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ar.HandleUserDelete(httptest.NewRecorder(), r)

	for name, store := range map[string]interface{ Load(any) (any, bool) }{
		"a-browser": &ar.cookieIdStore, "a-gateway": &ar.gatewaySessionStore, aliceCode: &ar.sessionIdStore,
	} {
		if _, exists := store.Load(name); exists {
			t.Errorf("session %s of deleted user still present", name)
		}
	}
	if _, exists := ar.cookieIdStore.Load("b-browser"); !exists {
		t.Errorf("session of another user was removed")
	}
}

func TestMissingUserIDIsPersisted(t *testing.T) {
	ar := newTestAuthRouter(t)
	// A record written without an ID, as an older build could have stored it
	if err := ar.Database.Write(DB_USERS_TABLE, ar.userDbKey("dave"), &User{Username: "dave"}); err != nil {
		t.Fatalf("write: %v", err)
	}

	first, err := ar.getUserByUsername("dave")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	second, _ := ar.getUserByUsername("dave")
	if first.ID == "" || first.ID != second.ID {
		t.Fatalf("user ID not stable across reads: %q vs %q", first.ID, second.ID)
	}

	raw := &User{}
	ar.Database.Read(DB_USERS_TABLE, ar.userDbKey("dave"), raw)
	if raw.ID != first.ID {
		t.Fatalf("generated ID not persisted: stored %q, returned %q", raw.ID, first.ID)
	}
}

func TestPersistedSessionsWithoutUserIDAreDiscarded(t *testing.T) {
	ar := newTestAuthRouter(t)
	user := mustCreateUser(t, ar, "erin")
	expiry := time.Now().Add(time.Hour)

	ar.Database.Write(DB_BROWSER_SESSIONS_TABLE, DB_BROWSER_SESSION_KEY_PREFIX+"legacy", &BrowserSession{Username: "erin", Expiry: expiry})
	ar.Database.Write(DB_BROWSER_SESSIONS_TABLE, DB_BROWSER_SESSION_KEY_PREFIX+"bound", &BrowserSession{Username: "erin", UserID: user.ID, Expiry: expiry})
	ar.Database.Write(DB_GATEWAY_SESSIONS_TABLE, DB_GATEWAY_SESSION_KEY_PREFIX+"legacy", &GatewaySession{Username: "erin", Expiry: expiry})
	ar.Database.Write(DB_GATEWAY_SESSIONS_TABLE, DB_GATEWAY_SESSION_KEY_PREFIX+"bound", &GatewaySession{Username: "erin", UserID: user.ID, Expiry: expiry})

	ar.loadBrowserSessions()
	ar.loadGatewaySessions()

	if _, ok := ar.cookieIdStore.Load("legacy"); ok {
		t.Errorf("legacy browser session was loaded")
	}
	if _, ok := ar.gatewaySessionStore.Load("legacy"); ok {
		t.Errorf("legacy gateway session was loaded")
	}
	if _, ok := ar.cookieIdStore.Load("bound"); !ok {
		t.Errorf("bound browser session was not loaded")
	}
	if _, ok := ar.gatewaySessionStore.Load("bound"); !ok {
		t.Errorf("bound gateway session was not loaded")
	}
	if ar.Database.KeyExists(DB_BROWSER_SESSIONS_TABLE, DB_BROWSER_SESSION_KEY_PREFIX+"legacy") ||
		ar.Database.KeyExists(DB_GATEWAY_SESSIONS_TABLE, DB_GATEWAY_SESSION_KEY_PREFIX+"legacy") {
		t.Errorf("legacy sessions should be deleted from the database")
	}
}
