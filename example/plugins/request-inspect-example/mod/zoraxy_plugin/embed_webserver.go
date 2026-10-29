package zoraxy_plugin

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type PluginUiRouter struct {
	PluginID         string    //The ID of the plugin
	TargetFs         *embed.FS //The embed.FS where the UI files are stored
	TargetFsPrefix   string    //The prefix of the embed.FS where the UI files are stored, e.g. /web
	HandlerPrefix    string    //The prefix of the handler used to route this router, e.g. /ui
	EnableDebug      bool      //Enable debug mode
	terminateHandler func()    //The handler to be called when the plugin is terminated
}

// NewPluginEmbedUIRouter creates a new PluginUiRouter with embed.FS
// The targetFsPrefix is the prefix of the embed.FS where the UI files are stored
// The targetFsPrefix should be relative to the root of the embed.FS
// The targetFsPrefix should start with a slash (e.g. /web) that corresponds to the root folder of the embed.FS
// The handlerPrefix is the prefix of the handler used to route this router
// The handlerPrefix should start with a slash (e.g. /ui) that matches the http.Handle path
// All prefix should not end with a slash
func NewPluginEmbedUIRouter(pluginID string, targetFs *embed.FS, targetFsPrefix string, handlerPrefix string) *PluginUiRouter {
	//Make sure all prefix are in /prefix format
	if !strings.HasPrefix(targetFsPrefix, "/") {
		targetFsPrefix = "/" + targetFsPrefix
	}
	targetFsPrefix = strings.TrimSuffix(targetFsPrefix, "/")

	if !strings.HasPrefix(handlerPrefix, "/") {
		handlerPrefix = "/" + handlerPrefix
	}
	handlerPrefix = strings.TrimSuffix(handlerPrefix, "/")

	//Return the PluginUiRouter
	return &PluginUiRouter{
		PluginID:       pluginID,
		TargetFs:       targetFs,
		TargetFsPrefix: targetFsPrefix,
		HandlerPrefix:  handlerPrefix,
	}
}

func (p *PluginUiRouter) populateCSRFToken(r *http.Request, fsHandler http.Handler) http.Handler {
	//Get the CSRF token from header
	csrfToken := r.Header.Get("X-Zoraxy-Csrf")
	if csrfToken == "" {
		csrfToken = "missing-csrf-token"
	}

	//Return the middleware
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if the request is for an HTML file
		if strings.HasSuffix(r.URL.Path, ".html") {
			//Read the target file from embed.FS
			targetFilePath := strings.TrimPrefix(r.URL.Path, "/")
			targetFilePath = p.TargetFsPrefix + "/" + targetFilePath
			targetFilePath = strings.TrimPrefix(targetFilePath, "/")
			targetFileContent, err := fs.ReadFile(*p.TargetFs, targetFilePath)
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
			// Check if the directory has an index.html file
			indexFilePath := strings.TrimPrefix(r.URL.Path, "/") + "index.html"
			indexFilePath = p.TargetFsPrefix + "/" + indexFilePath
			indexFilePath = strings.TrimPrefix(indexFilePath, "/")
			indexFileContent, err := fs.ReadFile(*p.TargetFs, indexFilePath)
			if err == nil {
				body := string(indexFileContent)
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
func (p *PluginUiRouter) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//Remove the plugin UI handler path prefix
		if p.EnableDebug {
			fmt.Print("Request URL:", r.URL.Path, " rewriting to ")
		}

		rewrittenURL := r.RequestURI
		rewrittenURL = strings.TrimPrefix(rewrittenURL, p.HandlerPrefix)
		rewrittenURL = strings.ReplaceAll(rewrittenURL, "//", "/")
		r.URL, _ = url.Parse(rewrittenURL)
		r.RequestURI = rewrittenURL
		if p.EnableDebug {
			fmt.Println(r.URL.Path)
		}

		//Serve the file from the embed.FS
		subFS, err := fs.Sub(*p.TargetFs, strings.TrimPrefix(p.TargetFsPrefix, "/"))
		if err != nil {
			fmt.Println(err.Error())
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Replace {{csrf_token}} with the actual CSRF token and serve the file
		p.populateCSRFToken(r, http.FileServer(http.FS(subFS))).ServeHTTP(w, r)
	})
}

// RegisterTerminateHandler registers the terminate handler for the PluginUiRouter
// The terminate handler will be called when the plugin is terminated from Zoraxy plugin manager
// if mux is nil, the handler will be registered to http.DefaultServeMux
func (p *PluginUiRouter) RegisterTerminateHandler(termFunc func(), mux *http.ServeMux) {
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

// HandleFunc registers a handler function for the given pattern
// The pattern should start with the handler prefix, e.g. /ui/hello
// If the pattern does not start with the handler prefix, it will be prepended with the handler prefix
func (p *PluginUiRouter) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request), mux *http.ServeMux) {
	// If mux is nil, use the default ServeMux
	if mux == nil {
		mux = http.DefaultServeMux
	}

	// Make sure the pattern starts with the handler prefix
	if !strings.HasPrefix(pattern, p.HandlerPrefix) {
		pattern = p.HandlerPrefix + pattern
	}

	// Register the handler with the http.ServeMux
	mux.HandleFunc(pattern, handler)
}

// Attach the embed UI handler to the target http.ServeMux
func (p *PluginUiRouter) AttachHandlerToMux(mux *http.ServeMux) {
	if mux == nil {
		mux = http.DefaultServeMux
	}

	p.HandlerPrefix = strings.TrimSuffix(p.HandlerPrefix, "/")
	mux.Handle(p.HandlerPrefix+"/", p.Handler())
}
