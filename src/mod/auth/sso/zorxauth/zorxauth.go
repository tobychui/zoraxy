package zorxauth

import (
	"crypto/rand"
	"encoding/hex"
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
