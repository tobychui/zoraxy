package webserv

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"imuslab.com/zoraxy/mod/utils"
)

/*
	Static Web Server package

	This module host a static web server
*/

type WebServerOptions struct {
	Port                   string //Port for listening
	EnableDirectoryListing bool   //Enable listing of directory
	WebRoot                string //Folder for stroing the static web folders
}
type WebServer struct {
	mux       *http.ServeMux
	server    *http.Server
	option    *WebServerOptions
	isRunning bool
	mu        sync.Mutex
}

// NewWebServer creates a new WebServer instance.
func NewWebServer(options *WebServerOptions) *WebServer {
	if !utils.FileExists(options.WebRoot) {
		//Web root folder not exists. Create one
		os.MkdirAll(filepath.Join(options.WebRoot, "html"), 0775)
		os.MkdirAll(filepath.Join(options.WebRoot, "templates"), 0775)
	}
	return &WebServer{
		mux:       http.NewServeMux(),
		option:    options,
		isRunning: false,
		mu:        sync.Mutex{},
	}
}

// ChangePort changes the server's port.
func (ws *WebServer) ChangePort(port string) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if ws.isRunning {
		if err := ws.Stop(); err != nil {
			return err
		}
	}

	ws.option.Port = port
	ws.server.Addr = ":" + port

	return nil
}

// Start starts the web server.
func (ws *WebServer) Start() error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	//Check if server already running
	if ws.isRunning {
		return fmt.Errorf("web server is already running")
	}

	//Check if the port is usable
	if IsPortInUse(ws.option.Port) {
		return errors.New("Port already in use or access denied by host OS")
	}

	//Dispose the old mux and create a new one
	ws.mux = http.NewServeMux()

	//Create a static web server
	fs := http.FileServer(http.Dir(filepath.Join(ws.option.WebRoot, "html")))
	ws.mux.Handle("/", ws.fsMiddleware(fs))

	ws.server = &http.Server{
		Addr:    ":" + ws.option.Port,
		Handler: ws.mux,
	}

	go func() {
		if err := ws.server.ListenAndServe(); err != nil {
			if err != http.ErrServerClosed {
				fmt.Printf("Web server error: %v\n", err)
			}
		}
	}()

	log.Println("Static Web Server started. Listeing on :" + ws.option.Port)
	ws.isRunning = true

	return nil
}

// Stop stops the web server.
func (ws *WebServer) Stop() error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if !ws.isRunning {
		return fmt.Errorf("web server is not running")
	}

	if err := ws.server.Close(); err != nil {
		return err
	}

	ws.isRunning = false

	return nil
}

// UpdateDirectoryListing enables or disables directory listing.
func (ws *WebServer) UpdateDirectoryListing(enable bool) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.option.EnableDirectoryListing = enable

}

// Close stops the web server without returning an error.
func (ws *WebServer) Close() {
	ws.Stop()
}
