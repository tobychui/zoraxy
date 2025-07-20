// Handles the API-Key based authentication for plugins

package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

const (
	PLUGIN_API_PREFIX = "/plugin"
)

type PluginMiddlewareOptions struct {
	DeniedHandler http.HandlerFunc //Thing(s) to do when request is rejected
	ApiKeyManager *APIKeyManager
	TargetMux     *http.ServeMux
}

// PluginAuthMiddleware provides authentication middleware for plugin API requests
type PluginAuthMiddleware struct {
	option    PluginMiddlewareOptions
	endpoints map[string]http.HandlerFunc
}

// NewPluginAuthMiddleware creates a new plugin authentication middleware
func NewPluginAuthMiddleware(option PluginMiddlewareOptions) *PluginAuthMiddleware {
	return &PluginAuthMiddleware{
		option:    option,
		endpoints: make(map[string]http.HandlerFunc),
	}
}

func (m *PluginAuthMiddleware) HandleAuthCheck(w http.ResponseWriter, r *http.Request, handler http.HandlerFunc) {
	// Check for API key in the Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		// No authorization header
		m.option.DeniedHandler(w, r)
		return
	}

	// Check if it's a plugin API key (Bearer token)
	if !strings.HasPrefix(authHeader, "Bearer ") {
		// Not a Bearer token
		m.option.DeniedHandler(w, r)
		return
	}

	// Extract the API key
	apiKey := strings.TrimPrefix(authHeader, "Bearer ")

	// Validate the API key for this endpoint
	_, err := m.option.ApiKeyManager.ValidateAPIKeyForEndpoint(r.URL.Path, r.Method, apiKey)
	if err != nil {
		// Invalid API key or endpoint not permitted
		m.option.DeniedHandler(w, r)
		return
	}

	// Call the original handler
	handler(w, r)
}

// wraps an HTTP handler with plugin authentication middleware
func (m *PluginAuthMiddleware) HandleFunc(endpoint string, handler http.HandlerFunc) error {
	// ensure the endpoint is prefixed with PLUGIN_API_PREFIX
	if !strings.HasPrefix(endpoint, PLUGIN_API_PREFIX) {
		endpoint = PLUGIN_API_PREFIX + endpoint
	}

	// Check if the endpoint already registered
	if _, exist := m.endpoints[endpoint]; exist {
		fmt.Println("WARNING! Duplicated registering of plugin api endpoint: " + endpoint)
		return errors.New("endpoint register duplicated")
	}

	m.endpoints[endpoint] = handler

	wrappedHandler := func(w http.ResponseWriter, r *http.Request) {
		m.HandleAuthCheck(w, r, handler)
	}

	// Ok. Register handler
	if m.option.TargetMux == nil {
		http.HandleFunc(endpoint, wrappedHandler)
	} else {
		m.option.TargetMux.HandleFunc(endpoint, wrappedHandler)
	}

	return nil
}
