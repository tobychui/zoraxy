package zoraxy_plugin

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

type PathRouter struct {
	enableDebugPrint bool
	pathHandlers     map[string]http.Handler
	defaultHandler   http.Handler
}

// NewPathRouter creates a new PathRouter
func NewPathRouter() *PathRouter {
	return &PathRouter{
		enableDebugPrint: false,
		pathHandlers:     make(map[string]http.Handler),
	}
}

// RegisterPathHandler registers a handler for a path
func (p *PathRouter) RegisterPathHandler(path string, handler http.Handler) {
	path = strings.TrimSuffix(path, "/")
	p.pathHandlers[path] = handler
}

// RemovePathHandler removes a handler for a path
func (p *PathRouter) RemovePathHandler(path string) {
	delete(p.pathHandlers, path)
}

// SetDefaultHandler sets the default handler for the router
// This handler will be called if no path handler is found
func (p *PathRouter) SetDefaultHandler(handler http.Handler) {
	p.defaultHandler = handler
}

// SetDebugPrintMode sets the debug print mode
func (p *PathRouter) SetDebugPrintMode(enable bool) {
	p.enableDebugPrint = enable
}

// StartStaticCapture starts the static capture ingress
func (p *PathRouter) RegisterStaticCaptureHandle(capture_ingress string, mux *http.ServeMux) {
	if !strings.HasSuffix(capture_ingress, "/") {
		capture_ingress = capture_ingress + "/"
	}
	mux.Handle(capture_ingress, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.staticCaptureServeHTTP(w, r)
	}))
}

// staticCaptureServeHTTP serves the static capture path using user defined handler
func (p *PathRouter) staticCaptureServeHTTP(w http.ResponseWriter, r *http.Request) {
	capturePath := r.Header.Get("X-Zoraxy-Capture")
	if capturePath != "" {
		if p.enableDebugPrint {
			fmt.Printf("Using capture path: %s\n", capturePath)
		}
		originalURI := r.Header.Get("X-Zoraxy-Uri")
		r.URL.Path = originalURI
		if handler, ok := p.pathHandlers[capturePath]; ok {
			handler.ServeHTTP(w, r)
			return
		}
	}
	p.defaultHandler.ServeHTTP(w, r)
}

func (p *PathRouter) PrintRequestDebugMessage(r *http.Request) {
	if p.enableDebugPrint {
		fmt.Printf("Capture Request with path: %s \n\n**Request Headers** \n\n", r.URL.Path)
		keys := make([]string, 0, len(r.Header))
		for key := range r.Header {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			for _, value := range r.Header[key] {
				fmt.Printf("%s: %s\n", key, value)
			}
		}

		fmt.Printf("\n\n**Request Details**\n\n")
		fmt.Printf("Method: %s\n", r.Method)
		fmt.Printf("URL: %s\n", r.URL.String())
		fmt.Printf("Proto: %s\n", r.Proto)
		fmt.Printf("Host: %s\n", r.Host)
		fmt.Printf("RemoteAddr: %s\n", r.RemoteAddr)
		fmt.Printf("RequestURI: %s\n", r.RequestURI)
		fmt.Printf("ContentLength: %d\n", r.ContentLength)
		fmt.Printf("TransferEncoding: %v\n", r.TransferEncoding)
		fmt.Printf("Close: %v\n", r.Close)
		fmt.Printf("Form: %v\n", r.Form)
		fmt.Printf("PostForm: %v\n", r.PostForm)
		fmt.Printf("MultipartForm: %v\n", r.MultipartForm)
		fmt.Printf("Trailer: %v\n", r.Trailer)
		fmt.Printf("RemoteAddr: %s\n", r.RemoteAddr)
		fmt.Printf("RequestURI: %s\n", r.RequestURI)

	}
}
