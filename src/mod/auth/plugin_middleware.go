// Handles the API-Key based authentication for plugins

package auth

import (
	"net/http"
	"strings"
)

// PluginAuthMiddleware provides authentication middleware for plugin API requests
type PluginAuthMiddleware struct {
	apiKeyManager *APIKeyManager
}

// NewPluginAuthMiddleware creates a new plugin authentication middleware
func NewPluginAuthMiddleware(apiKeyManager *APIKeyManager) *PluginAuthMiddleware {
	return &PluginAuthMiddleware{
		apiKeyManager: apiKeyManager,
	}
}

// WrapHandler wraps an HTTP handler with plugin authentication middleware
func (m *PluginAuthMiddleware) WrapHandler(endpoint string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// First, remove any existing plugin authentication headers
		r.Header.Del("X-Zoraxy-Plugin-ID")
		r.Header.Del("X-Zoraxy-Plugin-Auth")

		// Check for API key in the Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			// No authorization header, proceed with normal authentication
			handler(w, r)
			return
		}

		// Check if it's a plugin API key (Bearer token)
		if !strings.HasPrefix(authHeader, "Bearer ") {
			// Not a Bearer token, proceed with normal authentication
			handler(w, r)
			return
		}

		// Extract the API key
		apiKey := strings.TrimPrefix(authHeader, "Bearer ")

		// Validate the API key for this endpoint
		pluginAPIKey, err := m.apiKeyManager.ValidateAPIKeyForEndpoint(endpoint, r.Method, apiKey)
		if err != nil {
			// Invalid API key or endpoint not permitted
			http.Error(w, "Unauthorized: Invalid API key or endpoint not permitted", http.StatusUnauthorized)
			return
		}

		// Add plugin information to the request context
		r.Header.Set("X-Zoraxy-Plugin-ID", pluginAPIKey.PluginID)
		r.Header.Set("X-Zoraxy-Plugin-Auth", "true")

		// Call the original handler
		handler(w, r)
	}
}
