package webserv

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"golang.org/x/net/webdav"
	"imuslab.com/zoraxy/mod/auth"
	"imuslab.com/zoraxy/mod/netutils"
)

/*
	WebDAV Server

	This module provides a WebDAV server for managing files
	in the static web server directory
*/

type WebDAVServer struct {
	server    *http.Server
	handler   *webdav.Handler
	options   *WebServerOptions
	authAgent *auth.AuthAgent
	isRunning bool
	mu        sync.Mutex
}

// NewWebDAVServer creates a new WebDAV server instance
func NewWebDAVServer(options *WebServerOptions, authAgent *auth.AuthAgent) *WebDAVServer {
	handler := &webdav.Handler{
		FileSystem: webdav.Dir(options.WebRoot),
		LockSystem: webdav.NewMemLS(),
	}

	return &WebDAVServer{
		handler:   handler,
		options:   options,
		authAgent: authAgent,
		isRunning: false,
		mu:        sync.Mutex{},
	}
}

// basicAuthMiddleware provides basic authentication for WebDAV
func (wd *WebDAVServer) basicAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || !wd.authAgent.ValidateUsernameAndPassword(username, password) {
			w.Header().Set("WWW-Authenticate", `Basic realm="Zoraxy WebDAV"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Start starts the WebDAV server
func (wd *WebDAVServer) Start(port string) error {
	wd.mu.Lock()
	defer wd.mu.Unlock()

	if wd.isRunning {
		return fmt.Errorf("WebDAV server is already running")
	}

	// Check if the port is usable
	if isPortInUse(port) {
		return errors.New("port already in use or access denied by host OS")
	}

	// Create server with basic auth middleware
	mux := http.NewServeMux()
	mux.Handle("/", wd.basicAuthMiddleware(wd.handler))

	wd.server = &http.Server{
		Addr:    "127.0.0.1:" + port,
		Handler: mux,
	}

	go func() {
		if err := wd.server.ListenAndServe(); err != nil {
			if err != http.ErrServerClosed {
				wd.options.Logger.PrintAndLog("webdav", "WebDAV server failed to start", err)
			}
		}
	}()

	wd.isRunning = true
	wd.options.Logger.PrintAndLog("webdav", "WebDAV server started. Listening on 127.0.0.1:"+port, nil)
	return nil
}

// Stop stops the WebDAV server
func (wd *WebDAVServer) Stop() error {
	wd.mu.Lock()
	defer wd.mu.Unlock()

	if !wd.isRunning {
		return fmt.Errorf("WebDAV server is not running")
	}

	if err := wd.server.Shutdown(context.Background()); err != nil {
		return err
	}

	wd.isRunning = false
	wd.options.Logger.PrintAndLog("webdav", "WebDAV server stopped", nil)
	return nil
}

// Restart restarts the WebDAV server
func (wd *WebDAVServer) Restart(port string) error {
	if wd.isRunning {
		if err := wd.Stop(); err != nil {
			return err
		}
	}

	if err := wd.Start(port); err != nil {
		return err
	}

	wd.options.Logger.PrintAndLog("webdav", "WebDAV server restarted. Listening on 127.0.0.1:"+port, nil)
	return nil
}

// IsRunning returns the running state of the WebDAV server
func (wd *WebDAVServer) IsRunning() bool {
	wd.mu.Lock()
	defer wd.mu.Unlock()
	return wd.isRunning
}

// Close stops the WebDAV server without returning an error
func (wd *WebDAVServer) Close() {
	wd.Stop()
}

// IsPortInUse checks if a port is in use.
func isPortInUse(port string) bool {
	portInt, err := strconv.Atoi(port)
	if err != nil {
		return false
	}

	return netutils.CheckIfPortOccupied(portInt)
}
