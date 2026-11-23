package zorxauth

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
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
		ar.removeBrowserSession(cookie.Value)
		return false
	}

	return true
}

// RequestIsAuthenticatedInSSO reports whether the request carries a valid SSO gateway
// session and returns the current username of its owner
func (ar *AuthRouter) RequestIsAuthenticatedInSSO(w http.ResponseWriter, r *http.Request) (bool, string) {
	user := ar.getSSOSessionUser(r)
	if user == nil {
		return false, ""
	}
	return true, user.Username
}

// getSSOSessionUser returns the user signed in to the SSO gateway, or nil if the
// request has no valid gateway session
func (ar *AuthRouter) getSSOSessionUser(r *http.Request) *User {
	//Check if cookie exists
	cookie, err := r.Cookie(ar.Options.CookieName)
	if err != nil {
		return nil
	}

	sessionData, exists := ar.gatewaySessionStore.Load(cookie.Value)
	if !exists {
		return nil
	}

	gatewaySession, ok := sessionData.(*GatewaySession)
	if !ok {
		return nil
	}

	// Check if session has expired
	if time.Now().After(gatewaySession.Expiry) {
		// Delete the expired gateway session
		ar.removeGatewaySession(cookie.Value)

		// When the SSO portal session expires, all site-specific sessions of this user should also be invalidated
		ar.deleteBrowserSessionsOfUser(gatewaySession.UserID)
		return nil
	}

	user, err := ar.resolveSessionUser(gatewaySession.Username, gatewaySession.UserID)
	if err != nil {
		ar.removeGatewaySession(cookie.Value)
		return nil
	}

	if user.Username != gatewaySession.Username {
		// User was renamed, refresh the lookup hint so later requests take the fast path
		ar.replaceGatewaySession(cookie.Value, gatewaySession, &GatewaySession{
			Username: user.Username,
			UserID:   user.ID,
			Expiry:   gatewaySession.Expiry,
		})
	}

	return user
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
		redirectURL = "/" // default to home page if redirect URL is missing
	}

	// One-time use validation code
	handoffObj, exists := ar.sessionIdStore.LoadAndDelete(sessionID)
	if !exists {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write(invalidHTML)
		return errors.New("validation session not found or expired")
	}

	handoff, ok := handoffObj.(*ValidationCodeSession)
	if !ok || handoff == nil {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write(invalidHTML)
		return errors.New("validation session is invalid")
	}

	user, err := ar.resolveSessionUser(handoff.Username, handoff.UserID)
	if err != nil {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write(invalidHTML)
		return errors.New("validation session is invalid: " + err.Error())
	}

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
		Username: user.Username,
		UserID:   user.ID,
		Expiry:   expiryTime,
	}
	ar.storeBrowserSession(browserSessionID, browserSession)

	// Set up cleanup timer for when session expires
	if cookieDuration > 0 {
		time.AfterFunc(time.Duration(cookieDuration)*time.Second, func() {
			ar.removeBrowserSession(browserSessionID)
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

// buildLoginRedirectURL returns the absolute SSO login URL for an unauthenticated request.
// Host-only SSORedirectURL values are normalized so they are never treated as relative paths.
func (ar *AuthRouter) buildLoginRedirectURL(r *http.Request) (string, error) {
	if ar.Options.SSORedirectURL == "" {
		return "", errors.New("SSO Redirect URL is not set")
	}

	scheme := requestScheme(r)
	ssoBase, err := ensureAbsoluteHTTPURL(ar.Options.SSORedirectURL, scheme)
	if err != nil {
		return "", errors.New("SSO Redirect URL is invalid: " + err.Error())
	}
	if ssoRedirectWouldLoop(ssoBase, r) {
		return "", errors.New("SSO Redirect URL resolves to the same host as the protected request")
	}

	originalURL := buildOriginalRequestURL(r)
	return buildSSOAuthRedirect(ssoBase, originalURL)
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
		redirectURL, err := ar.buildLoginRedirectURL(r)
		if err != nil {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(invalidHTML)
			return err
		}

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

		redirectURL, buildErr := ar.buildLoginRedirectURL(r)
		if buildErr != nil {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(invalidHTML)
			return buildErr
		}

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
