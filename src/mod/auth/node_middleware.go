// Handles the API-Key based authentication for plugins

package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"imuslab.com/zoraxy/mod/node"
)

const (
	NODE_API_PREFIX = "/node"
)

type HandlerFunc func(*node.Node, http.ResponseWriter, *http.Request)

type NodeMiddlewareOptions struct {
	DeniedHandler http.HandlerFunc //Thing(s) to do when request is rejected
	NodeManager   *node.Manager
	TargetMux     *http.ServeMux
}

// NodeAuthMiddleware provides authentication middleware for node API requests
type NodeAuthMiddleware struct {
	option    NodeMiddlewareOptions
	endpoints map[string]HandlerFunc
}

// NewNodeAuthMiddleware creates a new node authentication middleware
func NewNodeAuthMiddleware(option NodeMiddlewareOptions) *NodeAuthMiddleware {
	return &NodeAuthMiddleware{
		option:    option,
		endpoints: make(map[string]HandlerFunc),
	}
}

func (m *NodeAuthMiddleware) HandleAuthCheck(w http.ResponseWriter, r *http.Request, handler HandlerFunc) {
	// Check for API key in the Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		// No authorization header
		m.option.DeniedHandler(w, r)
		return
	}

	// Check if it's a node API key (Bearer token)
	if !strings.HasPrefix(authHeader, "Bearer ") {
		// Not a Bearer token
		m.option.DeniedHandler(w, r)
		return
	}

	// Extract the API key
	apiKey := strings.TrimPrefix(authHeader, "Bearer ")

	// Validate the API key for this endpoint
	node, err := m.option.NodeManager.GetNodeByToken(apiKey)
	if err != nil {
		// Invalid API key or endpoint not permitted
		m.option.DeniedHandler(w, r)
		return
	}

	// Call the original handler
	handler(node, w, r)
}

// wraps an HTTP handler with plugin authentication middleware
func (m *NodeAuthMiddleware) HandleFunc(endpoint string, handler HandlerFunc) error {
	// ensure the endpoint is prefixed with PLUGIN_API_PREFIX
	if !strings.HasPrefix(endpoint, NODE_API_PREFIX) {
		endpoint = NODE_API_PREFIX + endpoint
	}

	// Check if the endpoint already registered
	if _, exist := m.endpoints[endpoint]; exist {
		fmt.Println("WARNING! Duplicated registering of node api endpoint: " + endpoint)
		return errors.New("endpoint register duplicated")
	}

	fmt.Println("Registering node api endpoint: " + endpoint)

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
