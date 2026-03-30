package zorxauth

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"imuslab.com/zoraxy/mod/netutils"
)

//go:embed auth.html
var authPageHTML []byte

//go:embed noaccess.html
var noAccessHTML []byte

//go:embed logout.html
var logoutHTML []byte

//go:embed favicon.png
var faviconPNG []byte

// GatewayServer represents the authentication gateway HTTP server
type GatewayServer struct {
	server *http.Server
	router *AuthRouter
	mu     sync.RWMutex
	mux    *http.ServeMux
}

// NewGatewayServer creates a new gateway server instance
func NewGatewayServer(router *AuthRouter) *GatewayServer {
	mux := http.NewServeMux()
	gs := &GatewayServer{
		router: router,
		mux:    mux,
	}

	// Register routes
	mux.HandleFunc("/", gs.handleAuthPage)
	mux.HandleFunc("/noaccess", gs.handleNoAccessPage)
	mux.HandleFunc("/favicon.png", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		w.Write(faviconPNG)
	})
	mux.HandleFunc("/login", gs.handleLogin)
	mux.HandleFunc("/logout", gs.handleLogout)

	return gs
}

// Start starts the gateway server on the configured port
func (gs *GatewayServer) Start() error {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	if gs.server != nil {
		return fmt.Errorf("gateway server is already running")
	}

	if !gs.router.Options.EnableAuthGateway {
		return fmt.Errorf("authentication gateway is disabled")
	}

	port := gs.router.Options.GatewayPort
	if port == 0 {
		port = 5489
	}

	gs.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      gs.mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		if gs.router.Logger != nil {
			gs.router.Logger.PrintAndLog("zorxauth", fmt.Sprintf("Starting authentication gateway on port %d", port), nil)
		}

		if err := gs.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			if gs.router.Logger != nil {
				gs.router.Logger.PrintAndLog("zorxauth", fmt.Sprintf("Gateway server error: %v", err), err)
			}
		}
	}()

	return nil
}

// Stop stops the gateway server
func (gs *GatewayServer) Stop() error {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	if gs.server == nil {
		return fmt.Errorf("gateway server is not running")
	}

	if gs.router.Logger != nil {
		gs.router.Logger.PrintAndLog("zorxauth", "Stopping authentication gateway", nil)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := gs.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("failed to shutdown gateway server: %w", err)
	}

	gs.server = nil
	return nil
}

// Restart restarts the gateway server
func (gs *GatewayServer) Restart() error {
	if gs.router.Logger != nil {
		gs.router.Logger.PrintAndLog("zorxauth", "Restarting authentication gateway", nil)
	}

	// Stop the server if it's running
	gs.mu.RLock()
	isRunning := gs.server != nil
	gs.mu.RUnlock()

	if isRunning {
		if err := gs.Stop(); err != nil {
			return fmt.Errorf("failed to stop gateway during restart: %w", err)
		}
		// Give it a moment to fully stop
		time.Sleep(100 * time.Millisecond)
	}

	// Start the server
	return gs.Start()
}

// IsRunning returns whether the gateway server is currently running
func (gs *GatewayServer) IsRunning() bool {
	gs.mu.RLock()
	defer gs.mu.RUnlock()
	return gs.server != nil
}

/*
	Authentication Gateway Handlers
	These handlers serve the authentication pages and process login/logout requests.
*/

// handleAuthPage serves the authentication/login page
func (gs *GatewayServer) handleAuthPage(w http.ResponseWriter, r *http.Request) {
	// Check if gateway is enabled
	if gs.ServeGatewayDisabled(w, r) {
		return
	}

	// Check if the user already has a valid session cookie in the SSO portal
	// if yes, grant access without showing the login page and directly redirect to the original
	// request domain set session URL with a one-time validation code
	authenticated, username := gs.router.RequestIsAuthenticatedInSSO(w, r)
	if authenticated {
		redirectURL := r.URL.Query().Get("redirect")
		if redirectURL == "" {
			redirectURL = gs.router.Options.FallbackRedirectURL
			if redirectURL == "" {
				//Invalid settings, fallback to about:blank to avoid open redirect vulnerability
				redirectURL = "about:blank"
			}
		}

		host := ""
		parsedRedirectURL, err := url.Parse(redirectURL)
		if err == nil {
			host = parsedRedirectURL.Hostname()
		} else {
			if gs.router.Logger != nil {
				gs.router.Logger.PrintAndLog("zorxauth", fmt.Sprintf("Error parsing redirect URL: %v", err), err)
			}
		}

		if gs.router.ValidateUserAccessToHost(username, host) {
			// Renew the gateway session expiry time since the user is actively using it
			cookie, cookieErr := r.Cookie(gs.router.Options.CookieName)
			if cookieErr == nil && cookie.Value != "" {
				// Load the existing gateway session
				if sessionData, exists := gs.router.gatewaySessionStore.Load(cookie.Value); exists {
					if gatewaySession, ok := sessionData.(*GatewaySession); ok {
						// Extend the session expiry time
						newExpiry := time.Now().Add(time.Duration(gs.router.Options.CookieDuration) * time.Second)
						gatewaySession.Expiry = newExpiry
						gs.router.gatewaySessionStore.Store(cookie.Value, gatewaySession)
					}
				}
			}

			sessionId := gs.router.generateValidationCodeForSession(username)
			// Parse the redirect target so we can build the session-set URL on the target host.
			parsedTarget, parseErr := url.Parse(redirectURL)
			if parseErr == nil && parsedTarget.Host != "" {
				host := parsedTarget.Hostname()
				port := parsedTarget.Port()
				targetProtocol := parsedTarget.Scheme
				if targetProtocol == "" {
					targetProtocol = "http"
				}
				hostWithPort := host
				if port != "" {
					hostWithPort = host + ":" + port
				}
				sessionSetURL := fmt.Sprintf("%s://%s/%s?session_id=%s&redirect=%s",
					targetProtocol, hostWithPort,
					strings.TrimPrefix(gs.router.Options.SSOSessionSetURL, "/"),
					sessionId, url.QueryEscape(redirectURL))
				http.Redirect(w, r, sessionSetURL, http.StatusSeeOther)
				return
			}
		} else {
			// User doesn't have access to this host — send them to the no-access page
			// so they can choose to logout and switch to an account that has access.
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write(noAccessHTML)
			return
		}
	}

	// Serve the authentication page
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(authPageHTML)
}

// handleLogin processes login requests
func (gs *GatewayServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	if gs.ServeGatewayDisabled(w, r) {
		return
	}

	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Method not allowed",
		})
		return
	}

	clientIP := netutils.GetRequesterIPUntrusted(r)

	// --- Rate limiting: enforce per-minute attempt ceiling per IP ---
	if gs.router.Options.EnableRateLimit && gs.router.Options.RateLimitPerIp > 0 {
		if gs.router.getLoginAttemptCount(clientIP) >= int64(gs.router.Options.RateLimitPerIp) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"success": false,
				"error":   "Too many login attempts. Please try again later.",
			})
			return
		}
	}
	gs.router.incrementLoginAttempt(clientIP)

	// Extract credentials
	username := r.FormValue("username")
	password := r.FormValue("password")

	if !gs.router.ValidateUsername(username, password) {
		// --- Exponential backoff on failed credential check ---
		if gs.router.Options.UseExpotentialBackoff {
			failures := gs.router.incrementLoginFailure(clientIP)
			// delay = 100ms * 2^(failures-1), capped at 30 s
			delay := 100 * time.Millisecond * (1 << uint(failures-1))
			const maxDelay = 30 * time.Second
			if delay > maxDelay || delay < 0 { // guard against int overflow on large failure counts
				delay = maxDelay
			}
			time.Sleep(delay)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Invalid username or password",
		})
		return
	}

	// Successful login: clear the consecutive-failure counter for this IP
	gs.router.loginFailureCounter.Delete(clientIP)

	redirectTarget := r.FormValue("redirect")
	if redirectTarget == "" {
		redirectTarget = gs.router.Options.FallbackRedirectURL
		if redirectTarget == "" {
			//Invalid settings, fallback to about:blank to avoid open redirect vulnerability
			redirectTarget = "about:blank"
		}
	}

	parsedTarget, err := url.Parse(redirectTarget)
	if err != nil || parsedTarget.Host == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Invalid redirect target",
		})
		return
	}

	host := parsedTarget.Hostname()
	port := parsedTarget.Port()
	targetProtocol := parsedTarget.Scheme
	if targetProtocol == "" {
		targetProtocol = "http"
	}

	u, err := gs.router.getUserByUsernameOrEmail(username)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Internal server error",
		})
		return
	}

	if !gs.router.ValidateUserAccessToHost(u.Username, host) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Access to the requested host is not allowed for this user",
		})
		return
	}

	//User can access this host
	rememberMe := false
	if r.FormValue("remember_me") == "on" || strings.EqualFold(r.FormValue("remember_me"), "true") {
		rememberMe = true
	}
	if r.FormValue("rememberMe") == "on" || strings.EqualFold(r.FormValue("rememberMe"), "true") {
		rememberMe = true
	}

	//Generate a cookie for the authentication gateway domain with Domain
	// this way next time this user is redirected to the gateway for authentication,
	// the cookie will be included in the request and the gateway can recognize the user and skip the login step
	cookieDuration := gs.router.Options.CookieDuration
	if rememberMe {
		cookieDuration = gs.router.Options.CookieDurationRememberMe
	}
	isSecure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")

	// Generate a session token and store it in the gateway session store
	sessionToken := gs.router.generateSessionToken()
	expiryTime := time.Now().Add(time.Duration(cookieDuration) * time.Second)
	gatewaySession := &GatewaySession{
		Username: u.Username,
		Expiry:   expiryTime,
	}
	gs.router.gatewaySessionStore.Store(sessionToken, gatewaySession)

	// Set the gateway cookie
	http.SetCookie(w, &http.Cookie{
		Name:     gs.router.Options.CookieName,
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   cookieDuration,
	})

	// Schedule automatic removal from the store when the cookie expires
	time.AfterFunc(time.Duration(cookieDuration)*time.Second, func() {
		gs.router.gatewaySessionStore.Delete(sessionToken)
	})

	sessionId := gs.router.generateValidationCodeForSession(u.Username)
	hostWithPort := host
	if port != "" {
		hostWithPort = host + ":" + port
	}
	sessionSetURL := fmt.Sprintf("%s://%s/%s", targetProtocol, hostWithPort, strings.TrimPrefix(gs.router.Options.SSOSessionSetURL, "/"))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":        true,
		"sessionId":      sessionId,
		"redirectTarget": redirectTarget,
		"sessionSetURL":  sessionSetURL,
		"rememberMe":     rememberMe,
	})
}

// handleNoAccessPage serves the access-denied page when an authenticated user
// does not have permission for the requested host.
func (gs *GatewayServer) handleNoAccessPage(w http.ResponseWriter, r *http.Request) {
	if gs.ServeGatewayDisabled(w, r) {
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	w.Write(noAccessHTML)
}

// handleLogout processes logout requests.
// GET: Serves the logout page with user info and logout button
// POST: Invalidates all sessions (gateway and browser sessions) and clears cookies
func (gs *GatewayServer) handleLogout(w http.ResponseWriter, r *http.Request) {
	if gs.ServeGatewayDisabled(w, r) {
		return
	}

	// Handle GET request - serve logout page
	if r.Method == http.MethodGet {
		// Get current username for template replacement
		authenticated, username := gs.router.RequestIsAuthenticatedInSSO(w, r)
		if !authenticated {
			// If user is not authenticated show error
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized"))
			return
		}

		// Replace template placeholder with actual username
		pageContent := strings.Replace(string(logoutHTML), "{{username}}", username, -1)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(pageContent))
		return
	}

	// Handle POST request - perform logout
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get the current user's username from their session
	authenticated, username := gs.router.RequestIsAuthenticatedInSSO(w, r)
	if !authenticated || username == "" {
		// No valid session to logout from
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "No active session",
		})
		return
	}

	// Get the session cookie for the current browser
	cookie, err := r.Cookie(gs.router.Options.CookieName)
	if err == nil && cookie.Value != "" {
		// Remove from gateway session store
		gs.router.gatewaySessionStore.Delete(cookie.Value)
	}

	// Iterate through all browser sessions and remove all sessions for this username
	// This ensures the user is logged out from ALL devices/browsers
	gs.router.cookieIdStore.Range(func(key, value interface{}) bool {
		sessionID, ok := key.(string)
		if !ok {
			return true // continue iteration
		}

		browserSession, ok := value.(*BrowserSession)
		if !ok {
			return true // continue iteration
		}

		// If this session belongs to the current user, delete it
		if browserSession.Username == username {
			gs.router.cookieIdStore.Delete(sessionID)
		}

		return true // continue iteration
	})

	// Expire the cookie in the current browser
	isSecure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	http.SetCookie(w, &http.Cookie{
		Name:     gs.router.Options.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

func (gs *GatewayServer) ServeGatewayDisabled(w http.ResponseWriter, r *http.Request) bool {
	if !gs.router.Options.EnableAuthGateway {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Authentication gateway is disabled"))
		return true
	}
	return false
}
