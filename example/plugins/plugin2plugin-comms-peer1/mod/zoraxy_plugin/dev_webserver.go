package zoraxy_plugin

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type PluginUiDebugRouter struct {
	PluginID         string //The ID of the plugin
	TargetDir        string //The directory where the UI files are stored
	HandlerPrefix    string //The prefix of the handler used to route this router, e.g. /ui
	EnableDebug      bool   //Enable debug mode
	terminateHandler func() //The handler to be called when the plugin is terminated
}

// NewPluginFileSystemUIRouter creates a new PluginUiRouter with file system
// The targetDir is the directory where the UI files are stored (e.g. ./www)
// The handlerPrefix is the prefix of the handler used to route this router
// The handlerPrefix should start with a slash (e.g. /ui) that matches the http.Handle path
// All prefix should not end with a slash
func NewPluginFileSystemUIRouter(pluginID string, targetDir string, handlerPrefix string) *PluginUiDebugRouter {
	//Make sure all prefix are in /prefix format
	if !strings.HasPrefix(handlerPrefix, "/") {
		handlerPrefix = "/" + handlerPrefix
	}
	handlerPrefix = strings.TrimSuffix(handlerPrefix, "/")

	//Return the PluginUiRouter
	return &PluginUiDebugRouter{
		PluginID:      pluginID,
		TargetDir:     targetDir,
		HandlerPrefix: handlerPrefix,
	}
}

func (p *PluginUiDebugRouter) populateCSRFToken(r *http.Request, fsHandler http.Handler) http.Handler {
	//Get the CSRF token from header
	csrfToken := r.Header.Get("X-Zoraxy-Csrf")
	if csrfToken == "" {
		csrfToken = "missing-csrf-token"
	}

	//Return the middleware
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if the request is for an HTML file
		if strings.HasSuffix(r.URL.Path, ".html") {
			//Read the target file from file system
			targetFilePath := strings.TrimPrefix(r.URL.Path, "/")
			targetFilePath = p.TargetDir + "/" + targetFilePath
			targetFilePath = strings.TrimPrefix(targetFilePath, "/")
			targetFileContent, err := os.ReadFile(targetFilePath)
			if err != nil {
				http.Error(w, "File not found", http.StatusNotFound)
				return
			}
			body := string(targetFileContent)
			body = strings.ReplaceAll(body, "{{.csrfToken}}", csrfToken)
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(body))
			return
		} else if strings.HasSuffix(r.URL.Path, "/") {
			//Check if the request is for a directory
			//Check if the directory has an index.html file
			targetFilePath := strings.TrimPrefix(r.URL.Path, "/")
			targetFilePath = p.TargetDir + "/" + targetFilePath + "index.html"
			targetFilePath = strings.TrimPrefix(targetFilePath, "/")
			if _, err := os.Stat(targetFilePath); err == nil {
				//Serve the index.html file
				targetFileContent, err := os.ReadFile(targetFilePath)
				if err != nil {
					http.Error(w, "File not found", http.StatusNotFound)
					return
				}
				body := string(targetFileContent)
				body = strings.ReplaceAll(body, "{{.csrfToken}}", csrfToken)
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(body))
				return
			}
		}

		//Call the next handler
		fsHandler.ServeHTTP(w, r)
	})

}

// GetHttpHandler returns the http.Handler for the PluginUiRouter
func (p *PluginUiDebugRouter) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//Remove the plugin UI handler path prefix
		if p.EnableDebug {
			fmt.Print("Request URL:", r.URL.Path, " rewriting to ")
		}

		rewrittenURL := r.RequestURI
		rewrittenURL = strings.TrimPrefix(rewrittenURL, p.HandlerPrefix)
		rewrittenURL = strings.ReplaceAll(rewrittenURL, "//", "/")
		r.URL.Path = rewrittenURL
		r.RequestURI = rewrittenURL
		if p.EnableDebug {
			fmt.Println(r.URL.Path)
		}

		//Serve the file from the file system
		fsHandler := http.FileServer(http.Dir(p.TargetDir))

		// Replace {{csrf_token}} with the actual CSRF token and serve the file
		p.populateCSRFToken(r, fsHandler).ServeHTTP(w, r)
	})
}

// RegisterTerminateHandler registers the terminate handler for the PluginUiRouter
// The terminate handler will be called when the plugin is terminated from Zoraxy plugin manager
// if mux is nil, the handler will be registered to http.DefaultServeMux
func (p *PluginUiDebugRouter) RegisterTerminateHandler(termFunc func(), mux *http.ServeMux) {
	p.terminateHandler = termFunc
	if mux == nil {
		mux = http.DefaultServeMux
	}
	mux.HandleFunc(p.HandlerPrefix+"/term", func(w http.ResponseWriter, r *http.Request) {
		p.terminateHandler()
		w.WriteHeader(http.StatusOK)
		go func() {
			//Make sure the response is sent before the plugin is terminated
			time.Sleep(100 * time.Millisecond)
			os.Exit(0)
		}()
	})
}

// Attach the file system UI handler to the target http.ServeMux
func (p *PluginUiDebugRouter) AttachHandlerToMux(mux *http.ServeMux) {
	if mux == nil {
		mux = http.DefaultServeMux
	}

	p.HandlerPrefix = strings.TrimSuffix(p.HandlerPrefix, "/")
	mux.Handle(p.HandlerPrefix+"/", p.Handler())
}
