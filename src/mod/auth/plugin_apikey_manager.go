package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"imuslab.com/zoraxy/mod/plugins/zoraxy_plugin"
)

// PluginAPIKey represents an API key for a plugin
type PluginAPIKey struct {
	PluginID           string
	APIKey             string
	PermittedEndpoints []zoraxy_plugin.PermittedAPIEndpoint // List of permitted API endpoints
	CreatedAt          time.Time
}

// APIKeyManager manages API keys for plugins
type APIKeyManager struct {
	keys  map[string]*PluginAPIKey // key: API key, value: plugin info
	mutex sync.RWMutex
}

// NewAPIKeyManager creates a new API key manager
func NewAPIKeyManager() *APIKeyManager {
	return &APIKeyManager{
		keys:  make(map[string]*PluginAPIKey),
		mutex: sync.RWMutex{},
	}
}

// GenerateAPIKey generates a new API key for a plugin
func (m *APIKeyManager) GenerateAPIKey(pluginID string, permittedEndpoints []zoraxy_plugin.PermittedAPIEndpoint) (*PluginAPIKey, error) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	// Generate a cryptographically secure random key
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return nil, fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Hash the random bytes to create the API key
	hash := sha256.Sum256(bytes)
	apiKey := hex.EncodeToString(hash[:])

	// Create the plugin API key
	pluginAPIKey := &PluginAPIKey{
		PluginID:           pluginID,
		APIKey:             apiKey,
		PermittedEndpoints: permittedEndpoints,
		CreatedAt:          time.Now(),
	}

	// Store the API key
	m.keys[apiKey] = pluginAPIKey

	return pluginAPIKey, nil
}

// ValidateAPIKey validates an API key and returns the associated plugin information
func (m *APIKeyManager) ValidateAPIKey(apiKey string) (*PluginAPIKey, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	pluginAPIKey, exists := m.keys[apiKey]
	if !exists {
		return nil, fmt.Errorf("invalid API key")
	}

	return pluginAPIKey, nil
}

// ValidateAPIKeyForEndpoint validates an API key for a specific endpoint
func (m *APIKeyManager) ValidateAPIKeyForEndpoint(endpoint string, method string, apiKey string) (*PluginAPIKey, error) {
	pluginAPIKey, err := m.ValidateAPIKey(apiKey)
	if err != nil {
		return nil, err
	}

	// Check if the endpoint is permitted
	for _, permittedEndpoint := range pluginAPIKey.PermittedEndpoints {
		if permittedEndpoint.Endpoint == endpoint && permittedEndpoint.Method == method {
			return pluginAPIKey, nil
		}
	}

	return nil, fmt.Errorf("endpoint not permitted for this API key")
}

// RevokeAPIKey revokes an API key
func (m *APIKeyManager) RevokeAPIKey(apiKey string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	if _, exists := m.keys[apiKey]; !exists {
		return fmt.Errorf("API key not found")
	}

	delete(m.keys, apiKey)
	return nil
}

// RevokeAPIKeysForPlugin revokes all API keys for a specific plugin
func (m *APIKeyManager) RevokeAPIKeysForPlugin(pluginID string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	keysToRemove := []string{}
	for apiKey, pluginAPIKey := range m.keys {
		if pluginAPIKey.PluginID == pluginID {
			keysToRemove = append(keysToRemove, apiKey)
		}
	}

	for _, apiKey := range keysToRemove {
		delete(m.keys, apiKey)
	}

	return nil
}

// GetAPIKeyForPlugin returns the API key for a plugin (if exists)
func (m *APIKeyManager) GetAPIKeyForPlugin(pluginID string) (*PluginAPIKey, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	for _, pluginAPIKey := range m.keys {
		if pluginAPIKey.PluginID == pluginID {
			return pluginAPIKey, nil
		}
	}

	return nil, fmt.Errorf("no API key found for plugin")
}

// ListAPIKeys returns all API keys (for debugging purposes)
func (m *APIKeyManager) ListAPIKeys() []*PluginAPIKey {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	keys := make([]*PluginAPIKey, 0, len(m.keys))
	for _, pluginAPIKey := range m.keys {
		keys = append(keys, pluginAPIKey)
	}

	return keys
}
