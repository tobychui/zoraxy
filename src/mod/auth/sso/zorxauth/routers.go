package zorxauth

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"
)

//go:embed invalid.html
var invalidHTML []byte

//go:embed auth.html
var authHTML []byte

func (ar *AuthRouter) RequestIsAuthenticatedInHost(w http.ResponseWriter, r *http.Request) bool {
	//Check if cookie exists
	cookie, err := r.Cookie(ar.Options.CookieName)
	if err != nil {
		return false
	}

	_, exists := ar.sessionIdStore.Load(cookie.Value)
	return exists
}

func (ar *AuthRouter) RequestIsAuthenticatedInSSO(w http.ResponseWriter, r *http.Request) (bool, string) {
	//Check if cookie exists
	cookie, err := r.Cookie(ar.Options.CookieName)
	if err != nil {
		return false, ""
	}

	username, exists := ar.gatewaySessionStore.Load(cookie.Value)
	if !exists {
		return false, ""
	}

	usernameStr, ok := username.(string)
	if !ok {
		return false, ""
	}

	return true, usernameStr
}

// generateSessionToken creates a new random session token
// for browser session cookie. It is different from the validation code
// for SSO callback. See generateValidationCodeForSession() for that instead.
func (ar *AuthRouter) generateSessionToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		fallback := time.Now().Format("20060102150405.000000000")
		return hex.EncodeToString([]byte(fallback))
	}
	return hex.EncodeToString(buf)
}

func (ar *AuthRouter) handleSessionSetCallback(w http.ResponseWriter, r *http.Request) error {
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusBadRequest)
		w.Write(invalidHTML)
		return errors.New("missing session_id in callback")
	}

	redirectURL := r.URL.Query().Get("redirect")
	if redirectURL == "" {
		redirectURL = ar.Options.FallbackRedirectURL
		if redirectURL == "" {
			redirectURL = "/"
		}
	}

	rememberMe := strings.EqualFold(r.URL.Query().Get("remember_me"), "true") || r.URL.Query().Get("remember_me") == "on"

	usernameObj, exists := ar.sessionIdStore.Load(sessionID)
	if !exists {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write(invalidHTML)
		return errors.New("validation session not found or expired")
	}

	username, ok := usernameObj.(string)
	if !ok || username == "" {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write(invalidHTML)
		return errors.New("validation session is invalid")
	}

	// One-time use validation code
	ar.sessionIdStore.Delete(sessionID)

	browserSessionID := ar.generateSessionToken()
	ar.sessionIdStore.Store(browserSessionID, username)

	cookieDuration := ar.Options.CookieDuration
	if rememberMe {
		cookieDuration = ar.Options.CookieDurationRememberMe
	}
	if cookieDuration <= 0 {
		if rememberMe {
			cookieDuration = getDefaultOptions().CookieDurationRememberMe
		} else {
			cookieDuration = getDefaultOptions().CookieDuration
		}
	}

	if cookieDuration > 0 {
		time.AfterFunc(time.Duration(cookieDuration)*time.Second, func() {
			ar.sessionIdStore.Delete(browserSessionID)
		})
	}

	isSecure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{
		Name:     ar.Options.CookieName,
		Value:    browserSessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   cookieDuration,
	})

	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
	return errors.New("session callback handled")
}

func (ar *AuthRouter) setAuthHeaders(r *http.Request, user *User) {
	r.Header.Set("X-Auth-Method", "zorxauth")
	r.Header.Set("Remote-User", user.Username)
	r.Header.Set("Remote-Name", user.Username)
	r.Header.Set("X-Remote-User", user.Username)

	if user.Email != "" {
		r.Header.Set("Remote-Email", user.Email)
		r.Header.Set("X-Remote-Email", user.Email)
	} else {
		r.Header.Del("Remote-Email")
		r.Header.Del("X-Remote-Email")
	}
}

// HandleAuthRouting handles the routing for ZorxAuth authentication provider. It checks if the request is authenticated in the current site, if not, it redirects to the SSO login page
func (ar *AuthRouter) HandleAuthRouting(w http.ResponseWriter, r *http.Request) error {
	if strings.TrimRight(r.URL.Path, "/") == strings.TrimRight(ar.Options.SSOSessionSetURL, "/") {
		return ar.handleSessionSetCallback(w, r)
	}

	if !ar.RequestIsAuthenticatedInHost(w, r) {
		if ar.Options.SSORedirectURL == "" {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(invalidHTML)
			return errors.New("SSO Redirect URL is not set")
		}
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		redirectURL := ar.Options.SSORedirectURL + "?redirect=" + scheme + "://" + r.Host + r.RequestURI
		http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)

		// Technically this is not an error but the outer router require an error return to determine if the request is handled locally or not
		return errors.New("Unauthenticated, redirecting to SSO login page")
	}

	user, err := ar.GetUserFromRequest(w, r)
	if err != nil {
		// Session exists as cookie but no longer valid in store.
		http.SetCookie(w, &http.Cookie{
			Name:     ar.Options.CookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			MaxAge:   -1,
		})

		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		redirectURL := ar.Options.SSORedirectURL + "?redirect=" + scheme + "://" + r.Host + r.RequestURI
		http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
		return errors.New("Invalid session, redirecting to SSO login page")
	}

	ar.setAuthHeaders(r, user)
	return nil
}
