package routedebug

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/netutils"
)

/*
	Route Debugger module

	This module provides a lightweight HTTP server that echoes back every
	detail of the incoming request (method, URL, headers, query parameters,
	body preview, connection info) so that sysadmins can verify that proxy
	rules, custom headers, and auth settings are forwarded correctly to the
	upstream.

	Point any proxy rule or virtual-directory rule at
	http://127.0.0.1:<Port> to inspect what Zoraxy actually sends upstream.
*/

type RouteDebuggerOptions struct {
	Port        string             // Listening port (default: 5490)
	PrettyPrint bool               // Render HTML instead of plain text
	Logger      *logger.Logger     // System-wide logger
	Sysdb       *database.Database // Database for persisting settings
}

type RouteDebugger struct {
	server    *http.Server
	option    *RouteDebuggerOptions
	isRunning bool
	mu        sync.Mutex
}

// NewRouteDebugger creates a new RouteDebugger instance.
func NewRouteDebugger(options *RouteDebuggerOptions) *RouteDebugger {
	options.Sysdb.NewTable("routedebugger")
	return &RouteDebugger{
		option:    options,
		isRunning: false,
	}
}

// RestorePreviousState loads persisted settings and re-starts the server
// if it was running when Zoraxy last shut down.
func (rd *RouteDebugger) RestorePreviousState() {
	port := rd.option.Port
	rd.option.Sysdb.Read("routedebugger", "port", &port)
	rd.option.Port = port

	prettyPrint := rd.option.PrettyPrint
	rd.option.Sysdb.Read("routedebugger", "prettyprint", &prettyPrint)
	rd.option.PrettyPrint = prettyPrint

	wasRunning := false
	rd.option.Sysdb.Read("routedebugger", "enabled", &wasRunning)
	if wasRunning {
		rd.Start()
	}
}

// Start starts the route debugger HTTP server on 127.0.0.1:<Port>.
func (rd *RouteDebugger) Start() error {
	rd.mu.Lock()
	defer rd.mu.Unlock()

	if rd.isRunning {
		return fmt.Errorf("route debugger is already running")
	}

	if isPortInUse(rd.option.Port) {
		return fmt.Errorf("port %s is already in use", rd.option.Port)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", rd.handleDebugRequest)

	rd.server = &http.Server{
		Addr:    "127.0.0.1:" + rd.option.Port,
		Handler: mux,
	}

	go func() {
		if err := rd.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			rd.option.Logger.PrintAndLog("route-debugger", "Server error", err)
		}
	}()

	rd.isRunning = true
	rd.option.Sysdb.Write("routedebugger", "enabled", true)
	rd.option.Logger.PrintAndLog("route-debugger", "Route Debugger started on 127.0.0.1:"+rd.option.Port, nil)
	return nil
}

// Stop stops the route debugger server.
func (rd *RouteDebugger) Stop() error {
	rd.mu.Lock()
	defer rd.mu.Unlock()

	if !rd.isRunning {
		return fmt.Errorf("route debugger is not running")
	}

	if err := rd.server.Close(); err != nil {
		return err
	}

	rd.isRunning = false
	rd.option.Sysdb.Write("routedebugger", "enabled", false)
	rd.option.Logger.PrintAndLog("route-debugger", "Route Debugger stopped", nil)
	return nil
}

// Restart stops and then re-starts the route debugger server.
func (rd *RouteDebugger) Restart() error {
	if rd.isRunning {
		if err := rd.Stop(); err != nil {
			return err
		}
	}
	return rd.Start()
}

// IsRunning returns true if the route debugger server is currently running.
func (rd *RouteDebugger) IsRunning() bool {
	rd.mu.Lock()
	defer rd.mu.Unlock()
	return rd.isRunning
}

// ChangePort updates the listening port and restarts the server if it is running.
func (rd *RouteDebugger) ChangePort(port string) error {
	if isPortInUse(port) {
		return fmt.Errorf("port %s is already in use", port)
	}

	wasRunning := rd.isRunning
	if wasRunning {
		if err := rd.Stop(); err != nil {
			return err
		}
	}

	rd.option.Port = port
	rd.option.Sysdb.Write("routedebugger", "port", port)

	if wasRunning {
		return rd.Start()
	}
	return nil
}

// SetPrettyPrint toggles between HTML (pretty) and plain-text output modes.
func (rd *RouteDebugger) SetPrettyPrint(enable bool) {
	rd.option.PrettyPrint = enable
	rd.option.Sysdb.Write("routedebugger", "prettyprint", enable)
}

// GetListeningPort returns the currently configured listening port string.
func (rd *RouteDebugger) GetListeningPort() string {
	return rd.option.Port
}

// Close shuts down the server without surfacing an error (used on Zoraxy exit).
func (rd *RouteDebugger) Close() {
	rd.Stop()
}

func isPortInUse(port string) bool {
	portInt, err := strconv.Atoi(port)
	if err != nil {
		return false
	}
	return netutils.CheckIfPortOccupied(portInt)
}
