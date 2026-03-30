package zorxauth

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

//go:embed invalid.html
var invalidHTML []byte

func (ar *AuthRouter) RequestIsAuthenticatedInHost(w http.ResponseWriter, r *http.Request) bool {
	//Check if cookie exists
	cookie, err := r.Cookie(ar.Options.CookieName)
	if err != nil {
		return false
	}

	// Check if session exists in store
	sessionData, exists := ar.cookieIdStore.Load(cookie.Value)
	if !exists {
		return false
	}

	// Check if session has expired
	browserSession, ok := sessionData.(*BrowserSession)
	if !ok {
		return false
	}

	if time.Now().After(browserSession.Expiry) {
		ar.cookieIdStore.Delete(cookie.Value)
		return false
	}

	return true
}

func (ar *AuthRouter) RequestIsAuthenticatedInSSO(w http.ResponseWriter, r *http.Request) (bool, string) {
	//Check if cookie exists
	cookie, err := r.Cookie(ar.Options.CookieName)
	if err != nil {
		return false, ""
	}

	sessionData, exists := ar.gatewaySessionStore.Load(cookie.Value)
	if !exists {
		return false, ""
	}

	gatewaySession, ok := sessionData.(*GatewaySession)
	if !ok {
		return false, ""
	}

	// Check if session has expired
	if time.Now().After(gatewaySession.Expiry) {
		username := gatewaySession.Username

		// Delete the expired gateway session
		ar.gatewaySessionStore.Delete(cookie.Value)

		// Clear all browser sessions (cookieIdStore) for this user
		// When the SSO portal session expires, all site-specific sessions should also be invalidated
		ar.cookieIdStore.Range(func(key, value interface{}) bool {
			sessionID, ok := key.(string)
			if !ok {
				return true // continue iteration
			}

			browserSession, ok := value.(*BrowserSession)
			if !ok {
				return true // continue iteration
			}

			// If this browser session belongs to the expired user, delete it
			if browserSession.Username == username {
				ar.cookieIdStore.Delete(sessionID)
			}

			return true // continue iteration
		})

		return false, ""
	}

	return true, gatewaySession.Username
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

// handleSessionSetCallback handles the callback from SSO login with the validation code,
// sets the session cookie (for this domain), and redirects back to the original URL
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

	// Generate long living session cookie for browser with the username and expiry time
	browserSessionID := ar.generateSessionToken()
	rememberMe := strings.EqualFold(r.URL.Query().Get("remember_me"), "true") || r.URL.Query().Get("remember_me") == "on"
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

	// Create browser session with expiry time
	expiryTime := time.Now().Add(time.Duration(cookieDuration) * time.Second)
	browserSession := &BrowserSession{
		Username: username,
		Expiry:   expiryTime,
	}
	ar.cookieIdStore.Store(browserSessionID, browserSession)

	// Set up cleanup timer for when session expires
	if cookieDuration > 0 {
		time.AfterFunc(time.Duration(cookieDuration)*time.Second, func() {
			ar.cookieIdStore.Delete(browserSessionID)
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

// isAjaxRequest detects if the request is an AJAX/fetch request
func isAjaxRequest(r *http.Request) bool {
	// Check for X-Requested-With header (common in AJAX libraries)
	if r.Header.Get("X-Requested-With") == "XMLHttpRequest" {
		return true
	}

	// Check if Accept header prefers JSON over HTML
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") && !strings.Contains(accept, "text/html") {
		return true
	}

	// Check for Fetch API requests (Sec-Fetch-Mode header)
	if r.Header.Get("Sec-Fetch-Mode") == "cors" {
		return true
	}

	return false
}

// setAuthHeaders sets the authentication headers to be passed to the backend service after successful authentication
func (ar *AuthRouter) setAuthHeaders(r *http.Request, user *User) {
	r.Header.Set("X-Auth-Method", "zoraxy_sso")
	r.Header.Set("Remote-User", user.Username) //Authelia compatibility
	r.Header.Set("Remote-Name", user.Username) //Authelia compatibility
	if user.Email != "" {
		r.Header.Set("Remote-Email", user.Email) //Authelia compatibility
	} else {
		r.Header.Del("Remote-Email")
	}
}

// HandleAuthRouting handles the routing for ZorxAuth authentication provider.
// It checks if the request is authenticated in the current site, if not, it redirects to the SSO login page
func (ar *AuthRouter) HandleAuthRouting(w http.ResponseWriter, r *http.Request) error {
	if strings.TrimRight(r.URL.Path, "/") == strings.TrimRight(ar.Options.SSOSessionSetURL, "/") {
		// This is the callback endpoint for SSO session set so a cookie with the target domain can be set here.
		// After setting the cookie, it will redirect back to the original URL specified in the "redirect" query parameter.
		return ar.handleSessionSetCallback(w, r)
	}

	if !ar.RequestIsAuthenticatedInHost(w, r) {
		// Not authenticated in the current site
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
		originalURL := scheme + "://" + r.Host + r.RequestURI
		redirectURL := ar.Options.SSORedirectURL + "?redirect=" + url.QueryEscape(originalURL)

		// For AJAX requests, return 401 with JSON instead of redirecting
		// This avoids CORS issues with cross-origin redirects
		if isAjaxRequest(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":       "Session expired",
				"redirectUrl": redirectURL,
				"requireAuth": true,
			})
			return errors.New("Unauthenticated, AJAX request returned 401")
		}

		// For regular browser requests, redirect to SSO login page
		http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
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
		originalURL := scheme + "://" + r.Host + r.RequestURI
		redirectURL := ar.Options.SSORedirectURL + "?redirect=" + url.QueryEscape(originalURL)

		// For AJAX requests, return 401 with JSON instead of redirecting
		if isAjaxRequest(r) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":       "Session expired",
				"redirectUrl": redirectURL,
				"requireAuth": true,
			})
			return errors.New("Invalid session, AJAX request returned 401")
		}

		// For regular browser requests, redirect to SSO login page
		http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
		return errors.New("Invalid session, redirecting to SSO login page")
	}

	// Authenticated, set the auth headers and continue to proxy the request to the backend service
	ar.setAuthHeaders(r, user)
	return nil
}
