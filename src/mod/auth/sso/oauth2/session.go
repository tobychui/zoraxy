package oauth2

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

const sessionCookieName = "z-oauth-session"

// Sessions are process-local and expire absolutely, even under continuous
// traffic. Restarting Zoraxy or changing global OAuth2 settings requires login.
// Only a successfully verified callback may create one. Provider tokens never
// come from the browser and refresh tokens are deliberately not retained.
type oauth2Session struct {
	accessToken string
	tokenType   string
	subject     string
	host        string
	secure      bool
	generation  uint64
	expires     time.Time
}

func (ar *OAuth2Router) createSession(r *http.Request, token *oauth2.Token, subject string, generation uint64) (*http.Cookie, error) {
	ar.settingsMu.RLock()
	defer ar.settingsMu.RUnlock()
	if generation != ar.settingsGeneration || !token.Valid() {
		return nil, errors.New("OAuth2 login expired or configuration changed")
	}
	expires := time.Now().Add(time.Hour)
	if !token.Expiry.IsZero() && token.Expiry.Before(expires) {
		expires = token.Expiry
	}
	// GenerateVerifier uses 32 cryptographically random bytes (256 bits).
	id := oauth2.GenerateVerifier()
	session := oauth2Session{
		accessToken: token.AccessToken, tokenType: token.TokenType, subject: subject,
		host: strings.ToLower(r.Host), secure: r.TLS != nil, generation: generation, expires: expires,
	}
	// No maintenance goroutine is needed: reads reject expired entries and each
	// login prunes them. The capacity limit also bounds abandoned sessions.
	ar.sessions.DeleteExpired()
	ar.sessions.Set(id, session, time.Until(expires))
	if previous, err := r.Cookie(sessionCookieName); err == nil {
		ar.sessions.Delete(previous.Value)
	}
	return &http.Cookie{
		Name: sessionCookieName, Value: id, Path: "/", Expires: expires,
		HttpOnly: true, Secure: session.secure, SameSite: http.SameSiteLaxMode,
	}, nil
}

func (ar *OAuth2Router) sessionForRequest(r *http.Request, generation uint64) (oauth2Session, bool) {
	ar.settingsMu.RLock()
	defer ar.settingsMu.RUnlock()
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || generation != ar.settingsGeneration {
		return oauth2Session{}, false
	}
	item := ar.sessions.Get(cookie.Value)
	if item == nil {
		return oauth2Session{}, false
	}
	session := item.Value()
	return session, session.generation == generation && time.Now().Before(session.expires) &&
		session.host == strings.ToLower(r.Host) && session.secure == (r.TLS != nil)
}
