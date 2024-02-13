package webserv

import (
	"embed"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/utils"
	"imuslab.com/zoraxy/mod/webserv/filemanager"
)

/*
	Static Web Server package

	This module host a static web server
*/

//go:embed templates/*
var templates embed.FS

type WebServerOptions struct {
	Port                   string             //Port for listening
	EnableDirectoryListing bool               //Enable listing of directory
	WebRoot                string             //Folder for stroing the static web folders
	EnableWebDirManager    bool               //Enable web file manager to handle files in web directory
	Sysdb                  *database.Database //Database for storing configs
}

type WebServer struct {
	FileManager *filemanager.FileManager

	mux       *http.ServeMux
	server    *http.Server
	option    *WebServerOptions
	isRunning bool
	mu        sync.Mutex
}

// NewWebServer creates a new WebServer instance. One instance only
func NewWebServer(options *WebServerOptions) *WebServer {
	if !utils.FileExists(options.WebRoot) {
		//Web root folder not exists. Create one with default templates
		os.MkdirAll(filepath.Join(options.WebRoot, "html"), 0775)
		os.MkdirAll(filepath.Join(options.WebRoot, "templates"), 0775)
		indexTemplate, err := templates.ReadFile("templates/index.html")
		if err != nil {
			log.Println("Failed to read static wev server template file: ", err.Error())
		} else {
			os.WriteFile(filepath.Join(options.WebRoot, "html", "index.html"), indexTemplate, 0775)
		}

	}

	//Create a new file manager if it is enabled
	var newDirManager *filemanager.FileManager
	if options.EnableWebDirManager {
		fm := filemanager.NewFileManager(filepath.Join(options.WebRoot, "/html"))
		newDirManager = fm
	}

	//Create new table to store the config
	options.Sysdb.NewTable("webserv")
	return &WebServer{
		mux:         http.NewServeMux(),
		FileManager: newDirManager,
		option:      options,
		isRunning:   false,
		mu:          sync.Mutex{},
	}
}

// Restore the configuration to previous config
func (ws *WebServer) RestorePreviousState() {
	//Set the port
	port := ws.option.Port
	ws.option.Sysdb.Read("webserv", "port", &port)
	ws.option.Port = port

	//Set the enable directory list
	enableDirList := ws.option.EnableDirectoryListing
	ws.option.Sysdb.Read("webserv", "dirlist", &enableDirList)
	ws.option.EnableDirectoryListing = enableDirList

	//Check the running state
	webservRunning := true
	ws.option.Sysdb.Read("webserv", "enabled", &webservRunning)
	if webservRunning {
		ws.Start()
	} else {
		ws.Stop()
	}

}

// ChangePort changes the server's port.
func (ws *WebServer) ChangePort(port string) error {
	if IsPortInUse(port) {
		return errors.New("Selected port is used by another process")
	}

	if ws.isRunning {
		if err := ws.Stop(); err != nil {
			return err
		}
	}

	ws.option.Port = port
	ws.server.Addr = ":" + port

	err := ws.Start()
	if err != nil {
		return err
	}

	ws.option.Sysdb.Write("webserv", "port", port)

	return nil
}

// Get current using port in options
func (ws *WebServer) GetListeningPort() string {
	return ws.option.Port
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
	ws.option.Sysdb.Write("webserv", "enabled", true)
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
	ws.option.Sysdb.Write("webserv", "enabled", false)
	return nil
}

// UpdateDirectoryListing enables or disables directory listing.
func (ws *WebServer) UpdateDirectoryListing(enable bool) {
	ws.option.EnableDirectoryListing = enable
	ws.option.Sysdb.Write("webserv", "dirlist", enable)
}

// Close stops the web server without returning an error.
func (ws *WebServer) Close() {
	ws.Stop()
}
