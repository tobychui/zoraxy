package zorxauth

import (
	"encoding/json"
	"net/http"

	"imuslab.com/zoraxy/mod/utils"
)

// HandleAuthProviderSettings handles the settings for ZorxAuth authentication provider API endpoints
func (ar *AuthRouter) HandleAuthProviderSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ar.handleSettingsGET(w, r)
	case http.MethodPost:
		ar.handleSettingsPOST(w, r)
	case http.MethodDelete:
		ar.handleSettingsDELETE(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSettingsGET returns the current ZorxAuth configuration
func (ar *AuthRouter) handleSettingsGET(w http.ResponseWriter, r *http.Request) {
	cookieDurationRememberMe := ar.Options.CookieDurationRememberMe
	if cookieDurationRememberMe == 0 {
		cookieDurationRememberMe = getDefaultOptions().CookieDurationRememberMe
	}

	js, _ := json.Marshal(map[string]interface{}{
		"ssoRedirectURL":           ar.Options.SSORedirectURL,
		"ssoSessionSetURL":         ar.Options.SSOSessionSetURL,
		"cookieName":               ar.Options.CookieName,
		"cookieDuration":           ar.Options.CookieDuration,
		"cookieDurationRememberMe": cookieDurationRememberMe,
		"enableRateLimit":          ar.Options.EnableRateLimit,
		"rateLimitPerIp":           ar.Options.RateLimitPerIp,
		"useExpotentialBackoff":    ar.Options.UseExpotentialBackoff,
	})

	utils.SendJSONResponse(w, string(js))
}

// handleSettingsPOST updates the ZorxAuth configuration
func (ar *AuthRouter) handleSettingsPOST(w http.ResponseWriter, r *http.Request) {
	// Parse parameters
	ssoRedirectURL, err := utils.PostPara(r, "ssoRedirectURL")
	if err != nil {
		utils.SendErrorResponse(w, "SSO Redirect URL not set")
		return
	}

	ssoSessionSetURL, err := utils.PostPara(r, "ssoSessionSetURL")
	if err != nil {
		//Use the default value if not provided
		ssoSessionSetURL = getDefaultOptions().SSOSessionSetURL
	}

	cookieName, _ := utils.PostPara(r, "cookieName")
	if cookieName == "" {
		cookieName = "zorxauth_session_id"
	}

	cookieDuration, _ := utils.PostInt(r, "cookieDuration")
	if cookieDuration == 0 {
		cookieDuration = 3600
	}

	cookieDurationRememberMe, _ := utils.PostInt(r, "cookieDurationRememberMe")
	if cookieDurationRememberMe == 0 {
		cookieDurationRememberMe = getDefaultOptions().CookieDurationRememberMe
	}

	// Rate limiting settings
	enableRateLimit, _ := utils.PostBool(r, "enableRateLimit")
	rateLimitPerIp, _ := utils.PostInt(r, "rateLimitPerIp")
	if rateLimitPerIp == 0 {
		rateLimitPerIp = 60
	}
	useExpotentialBackoff, _ := utils.PostBool(r, "useExpotentialBackoff")

	// Update runtime configuration
	ar.Options.SSORedirectURL = ssoRedirectURL
	ar.Options.SSOSessionSetURL = ssoSessionSetURL
	ar.Options.CookieName = cookieName
	ar.Options.CookieDuration = cookieDuration
	ar.Options.CookieDurationRememberMe = cookieDurationRememberMe
	ar.Options.EnableRateLimit = enableRateLimit
	ar.Options.RateLimitPerIp = rateLimitPerIp
	ar.Options.UseExpotentialBackoff = useExpotentialBackoff

	// Save to database
	err = ar.Database.Write(DB_NAME, "options", ar.Options)
	if err != nil {
		utils.SendErrorResponse(w, "Failed to save settings")
		return
	}

	utils.SendOK(w)
}

// handleSettingsDELETE resets the ZorxAuth configuration to defaults
func (ar *AuthRouter) handleSettingsDELETE(w http.ResponseWriter, r *http.Request) {
	// Reset to defaults
	defaultOpts := getDefaultOptions()
	ar.Options.SSORedirectURL = defaultOpts.SSORedirectURL
	ar.Options.SSOSessionSetURL = defaultOpts.SSOSessionSetURL
	ar.Options.CookieName = defaultOpts.CookieName
	ar.Options.CookieDuration = defaultOpts.CookieDuration
	ar.Options.CookieDurationRememberMe = defaultOpts.CookieDurationRememberMe
	ar.Options.EnableRateLimit = defaultOpts.EnableRateLimit
	ar.Options.RateLimitPerIp = defaultOpts.RateLimitPerIp
	ar.Options.UseExpotentialBackoff = defaultOpts.UseExpotentialBackoff

	// Delete from database
	ar.Database.Delete(DB_NAME, "options")

	if ar.Logger != nil {
		ar.Logger.PrintAndLog("zorxauth", "ZorxAuth settings reset to defaults", nil)
	}

	utils.SendOK(w)
}

// HandleGatewaySettings handles the gateway settings API endpoints
func (ar *AuthRouter) HandleGatewaySettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ar.handleGatewaySettingsGET(w, r)
	case http.MethodPost:
		ar.handleGatewaySettingsPOST(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGatewaySettingsGET returns the current gateway configuration
func (ar *AuthRouter) handleGatewaySettingsGET(w http.ResponseWriter, r *http.Request) {
	isRunning := false
	if ar.gatewayServer != nil {
		isRunning = ar.gatewayServer.IsRunning()
	}

	js, _ := json.Marshal(map[string]interface{}{
		"gatewayEnabled": ar.Options.EnableAuthGateway,
		"gatewayPort":    ar.Options.GatewayPort,
		"isRunning":      isRunning,
	})

	utils.SendJSONResponse(w, string(js))
}

// handleGatewaySettingsPOST updates the gateway configuration
func (ar *AuthRouter) handleGatewaySettingsPOST(w http.ResponseWriter, r *http.Request) {
	// Parse parameters
	gatewayEnabled, _ := utils.PostBool(r, "gatewayEnabled")
	gatewayPort, _ := utils.PostInt(r, "gatewayPort")

	if gatewayPort == 0 {
		gatewayPort = 5489
	}

	// Validate port range
	if gatewayPort < 1 || gatewayPort > 65535 {
		utils.SendErrorResponse(w, "Invalid port number. Must be between 1 and 65535")
		return
	}

	// Check if port or enabled state changed
	portChanged := ar.Options.GatewayPort != gatewayPort
	enabledChanged := ar.Options.EnableAuthGateway != gatewayEnabled

	// Update runtime configuration
	ar.Options.EnableAuthGateway = gatewayEnabled
	ar.Options.GatewayPort = gatewayPort

	// Save to database
	err := ar.Database.Write(DB_NAME, "options", ar.Options)
	if err != nil {
		utils.SendErrorResponse(w, "Failed to save gateway settings")
		return
	}

	// Initialize gateway server if it doesn't exist
	if ar.gatewayServer == nil {
		ar.gatewayServer = NewGatewayServer(ar)
	}

	// Handle server state changes
	if gatewayEnabled {
		if portChanged || enabledChanged {
			// Restart server to apply changes
			if err := ar.gatewayServer.Restart(); err != nil {
				if ar.Logger != nil {
					ar.Logger.PrintAndLog("zorxauth", "Failed to start gateway server: "+err.Error(), err)
				}
				utils.SendErrorResponse(w, "Failed to start gateway server: "+err.Error())
				return
			}
		} else if !ar.gatewayServer.IsRunning() {
			// Start server if it's not running
			if err := ar.gatewayServer.Start(); err != nil {
				if ar.Logger != nil {
					ar.Logger.PrintAndLog("zorxauth", "Failed to start gateway server: "+err.Error(), err)
				}
				utils.SendErrorResponse(w, "Failed to start gateway server: "+err.Error())
				return
			}
		}
	} else {
		// Stop server if it's running
		if ar.gatewayServer.IsRunning() {
			if err := ar.gatewayServer.Stop(); err != nil {
				if ar.Logger != nil {
					ar.Logger.PrintAndLog("zorxauth", "Failed to stop gateway server: "+err.Error(), err)
				}
				utils.SendErrorResponse(w, "Failed to stop gateway server: "+err.Error())
				return
			}
		}
	}

	utils.SendOK(w)
}

// HandleLogoutAllUsers logs out all users by clearing all session stores
func (ar *AuthRouter) HandleLogoutAllUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Clear all session ID store (one-time validation codes)
	ar.sessionIdStore.Range(func(key, value interface{}) bool {
		ar.sessionIdStore.Delete(key)
		return true
	})

	// Clear all gateway session store
	ar.gatewaySessionStore.Range(func(key, value interface{}) bool {
		ar.gatewaySessionStore.Delete(key)
		return true
	})

	// Clear all cookie ID store (browser sessions)
	ar.cookieIdStore.Range(func(key, value interface{}) bool {
		ar.cookieIdStore.Delete(key)
		return true
	})

	// Clear all browser sessions from database
	entries, err := ar.Database.ListTable(DB_BROWSER_SESSIONS_TABLE)
	if err == nil {
		for _, entry := range entries {
			key := string(entry[0])
			ar.Database.Delete(DB_BROWSER_SESSIONS_TABLE, key)
		}
	}

	// Clear all gateway sessions from database
	gatewayEntries, err := ar.Database.ListTable(DB_GATEWAY_SESSIONS_TABLE)
	if err == nil {
		for _, entry := range gatewayEntries {
			key := string(entry[0])
			ar.Database.Delete(DB_GATEWAY_SESSIONS_TABLE, key)
		}
	}

	if ar.Logger != nil {
		ar.Logger.PrintAndLog("zorxauth", "All user sessions have been cleared", nil)
	}

	utils.SendOK(w)
}
