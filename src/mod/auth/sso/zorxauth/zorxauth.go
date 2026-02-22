package zorxauth

import (
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/info/logger"
)

/*

	ZorxAuth SSO Authentication Module

	This module is designed to replace forward auth providers like Authelia/Authentik with a lightweight SSO authentication solution
	that only works in Zoraxy environment. It is ideal for lazy homelab sysadmins who don't want to setup a dedicated authentication
	server just for a few services running in their homelab.

 	SSORedirectURL is the URL of the SSO authentication endpoint, e.g. auth.example.com.
	SSOSessionSetURL is the URL of the SSO session set endpoint, e.g. mysite.example.com/.wellknown/zorxauth/session/set,
	which will be called by the SSO endpoint after successful authentication to set the session ID in the AuthAgent.

	The AuthAgent will redirect unauthenticated requests to this URL with a "redirect" query parameter
	containing the original requested URL.
	e.g. auth.example.com?redirect=http://mysite.example.com/protected/resource

	After successful authentication, the user will be redirected back to the original domain (mysite.example.com) to
	the SSOSessionSetURL with the session ID and original redirect URL as query parameters, e.g.
	-> mysite.example.com/.wellknown/zorxauth/session/set?session_id=abc123&redirect=http://mysite.example.com/protected/resource

	The zorxauth module will capture this endpoint and set the cookie in the user's browser, allowing them to access protected resources without needing to authenticate again until the session expires.
	Lastly, the SSO endpoint will redirect the user back to the original URL specified in the "redirect" query parameter, e.g.
	-> mysite.example.com/protected/resource
*/

// NewAuthRouter creates a new AuthRouter instance
func NewAuthRouter(db *database.Database, log *logger.Logger) *AuthRouter {
	// Create a new table for zorxauth settings if it doesn't exist
	db.NewTable(DB_NAME)
	db.NewTable(DB_USERS_TABLE)
	db.NewTable(DB_BROWSER_SESSIONS_TABLE)
	db.NewTable(DB_GATEWAY_SESSIONS_TABLE)

	if !db.KeyExists(DB_NAME, "options") {
		//Write default options to database
		defaultOptions := getDefaultOptions()
		db.Write(DB_NAME, "options", defaultOptions)
	}

	//Load options from database
	options := AuthRouterOptions{}
	db.Read(DB_NAME, "options", &options)

	authRouter := &AuthRouter{
		Logger:             log,
		Database:           db,
		Options:            &options,
		rateLimitResetStop: make(chan bool, 1),
	}

	// Load browser sessions from database
	authRouter.loadBrowserSessions()

	// Load gateway sessions from database
	authRouter.loadGatewaySessions()

	// Start the per-minute login attempt counter reset ticker
	go authRouter.startLoginRateLimitTicker()

	//Start the authentication gateway if enabled
	if options.EnableAuthGateway {
		gatewayServer := NewGatewayServer(authRouter)
		if err := gatewayServer.Start(); err != nil {
			if log != nil {
				log.PrintAndLog("zorxauth", "Failed to start gateway server: "+err.Error(), err)
			}
		}

		authRouter.gatewayServer = gatewayServer
	}

	return authRouter
}

// GenerateValidationCodeForSession generates a one-time validation code for the given session ID and stores the mapping in sessionIdStore.
// this is not the cookie session ID, for cookie session ID, see generateSessionToken() instead.
func (ar *AuthRouter) generateValidationCodeForSession(sessionId string) string {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		// Fallback to time-derived code when entropy source fails
		buf = []byte(time.Now().Format("20060102150405.000000000"))
	}

	validationCode := hex.EncodeToString(buf)
	ar.sessionIdStore.Store(validationCode, sessionId)

	// Validation code is short-lived and one-time use.
	time.AfterFunc(30*time.Second, func() {
		ar.sessionIdStore.Delete(validationCode)
	})

	return validationCode
}

// startLoginRateLimitTicker resets per-IP login attempt counters every minute.
func (ar *AuthRouter) startLoginRateLimitTicker() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ar.rateLimitResetStop:
			return
		case <-ticker.C:
			ar.loginAttemptCounter.Range(func(key, _ interface{}) bool {
				ar.loginAttemptCounter.Delete(key)
				return true
			})
		}
	}
}

// getLoginAttemptCount returns the number of login attempts made by ip in the current minute window.
func (ar *AuthRouter) getLoginAttemptCount(ip string) int64 {
	v, ok := ar.loginAttemptCounter.Load(ip)
	if !ok {
		return 0
	}
	return atomic.LoadInt64(v.(*int64))
}

// incrementLoginAttempt records a login attempt (successful or not) from ip.
func (ar *AuthRouter) incrementLoginAttempt(ip string) {
	v, _ := ar.loginAttemptCounter.LoadOrStore(ip, new(int64))
	atomic.AddInt64(v.(*int64), 1)
}

// incrementLoginFailure records a consecutive login failure for ip and returns the new failure count.
func (ar *AuthRouter) incrementLoginFailure(ip string) int64 {
	v, _ := ar.loginFailureCounter.LoadOrStore(ip, new(int64))
	return atomic.AddInt64(v.(*int64), 1)
}

// loadBrowserSessions loads existing browser sessions from database into memory
func (ar *AuthRouter) loadBrowserSessions() {
	// Get all keys from the browser sessions table
	entries, err := ar.Database.ListTable(DB_BROWSER_SESSIONS_TABLE)
	if err != nil {
		if ar.Logger != nil {
			ar.Logger.PrintAndLog("zorxauth", "Failed to list browser sessions: "+err.Error(), err)
		}
		return
	}

	now := time.Now()
	loadedCount := 0
	expiredCount := 0

	for _, entry := range entries {
		var session BrowserSession
		key := string(entry[0])
		err := ar.Database.Read(DB_BROWSER_SESSIONS_TABLE, key, &session)
		if err != nil {
			if ar.Logger != nil {
				ar.Logger.PrintAndLog("zorxauth", "Failed to read browser session "+key+": "+err.Error(), err)
			}
			continue
		}

		// Skip expired sessions
		if now.After(session.Expiry) {
			expiredCount++
			ar.Database.Delete(DB_BROWSER_SESSIONS_TABLE, key)
			continue
		}

		// Extract session ID from key (remove prefix)
		sessionID := strings.TrimPrefix(key, DB_BROWSER_SESSION_KEY_PREFIX)

		// Store in memory
		ar.cookieIdStore.Store(sessionID, &session)
		loadedCount++

		// Set up cleanup timer for when session expires
		timeUntilExpiry := time.Until(session.Expiry)
		if timeUntilExpiry > 0 {
			sessionIDCopy := sessionID
			time.AfterFunc(timeUntilExpiry, func() {
				ar.cookieIdStore.Delete(sessionIDCopy)
			})
		}
	}

	if ar.Logger != nil {
		ar.Logger.PrintAndLog("zorxauth", "Loaded "+strconv.Itoa(loadedCount)+" browser sessions from database ("+strconv.Itoa(expiredCount)+" expired sessions removed)", nil)
	}
}

// loadGatewaySessions loads existing gateway sessions from database into memory
func (ar *AuthRouter) loadGatewaySessions() {
	// Get all keys from the gateway sessions table
	entries, err := ar.Database.ListTable(DB_GATEWAY_SESSIONS_TABLE)
	if err != nil {
		if ar.Logger != nil {
			ar.Logger.PrintAndLog("zorxauth", "Failed to list gateway sessions: "+err.Error(), err)
		}
		return
	}

	now := time.Now()
	loadedCount := 0
	expiredCount := 0

	for _, entry := range entries {
		var session GatewaySession
		key := string(entry[0])
		err := ar.Database.Read(DB_GATEWAY_SESSIONS_TABLE, key, &session)
		if err != nil {
			if ar.Logger != nil {
				ar.Logger.PrintAndLog("zorxauth", "Failed to read gateway session "+key+": "+err.Error(), err)
			}
			continue
		}

		// Skip expired sessions
		if now.After(session.Expiry) {
			expiredCount++
			ar.Database.Delete(DB_GATEWAY_SESSIONS_TABLE, key)
			continue
		}

		// Extract session ID from key (remove prefix)
		sessionID := strings.TrimPrefix(key, DB_GATEWAY_SESSION_KEY_PREFIX)

		// Store in memory
		ar.gatewaySessionStore.Store(sessionID, &session)
		loadedCount++

		// Set up cleanup timer for when session expires
		timeUntilExpiry := time.Until(session.Expiry)
		if timeUntilExpiry > 0 {
			sessionIDCopy := sessionID
			time.AfterFunc(timeUntilExpiry, func() {
				ar.gatewaySessionStore.Delete(sessionIDCopy)
			})
		}
	}

	if ar.Logger != nil {
		ar.Logger.PrintAndLog("zorxauth", "Loaded "+strconv.Itoa(loadedCount)+" gateway sessions from database ("+strconv.Itoa(expiredCount)+" expired sessions removed)", nil)
	}
}

// Close saves all browser sessions to database and performs cleanup
func (ar *AuthRouter) Close() {
	// Stop the login rate limit ticker
	if ar.rateLimitResetStop != nil {
		select {
		case ar.rateLimitResetStop <- true:
		default:
		}
	}

	// Stop the gateway server if running
	if ar.gatewayServer != nil {
		ar.gatewayServer.Stop()
	}

	// Save all browser sessions to database
	ar.cookieIdStore.Range(func(key, value interface{}) bool {
		sessionID, ok := key.(string)
		if !ok {
			return true
		}

		browserSession, ok := value.(*BrowserSession)
		if !ok {
			return true
		}

		// Skip expired sessions
		if time.Now().After(browserSession.Expiry) {
			return true
		}

		// Save to database
		dbKey := DB_BROWSER_SESSION_KEY_PREFIX + sessionID
		err := ar.Database.Write(DB_BROWSER_SESSIONS_TABLE, dbKey, browserSession)
		if err != nil && ar.Logger != nil {
			ar.Logger.PrintAndLog("zorxauth", "Failed to save browser session: "+err.Error(), err)
		}

		return true
	})

	// Save all gateway sessions to database
	ar.gatewaySessionStore.Range(func(key, value interface{}) bool {
		sessionID, ok := key.(string)
		if !ok {
			return true
		}

		gatewaySession, ok := value.(*GatewaySession)
		if !ok {
			return true
		}

		// Skip expired sessions
		if time.Now().After(gatewaySession.Expiry) {
			return true
		}

		// Save to database
		dbKey := DB_GATEWAY_SESSION_KEY_PREFIX + sessionID
		err := ar.Database.Write(DB_GATEWAY_SESSIONS_TABLE, dbKey, gatewaySession)
		if err != nil && ar.Logger != nil {
			ar.Logger.PrintAndLog("zorxauth", "Failed to save gateway session: "+err.Error(), err)
		}

		return true
	})

	if ar.Logger != nil {
		ar.Logger.PrintAndLog("zorxauth", "Browser and gateway sessions saved to database", nil)
	}
}
