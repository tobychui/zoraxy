package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/csrf"
	"imuslab.com/zoraxy/mod/sshprox"
)

/*
	router.go

	This script holds the static resources router
	for the reverse proxy service

	If you are looking for reverse proxy handler, see Server.go in mod/dynamicproxy/
*/

func FSHandler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow access to /script/*, /img/pubic/* and /login.html without authentication
		if strings.HasPrefix(r.URL.Path, "/script/") ||
			strings.HasPrefix(r.URL.Path, "/img/public/") ||
			r.URL.Path == "/login.html" ||
			r.URL.Path == "/reset.html" ||
			r.URL.Path == "/favicon.png" {
			if isHTMLFilePath(r.URL.Path) {
				handleInjectHTML(w, r, r.URL.Path)
				return
			}
			serveStaticWithCache(w, r, handler)
			return
		}

		// Check authentication
		if !authAgent.CheckAuth(r) && requireAuth {
			http.Redirect(w, r, "/login.html", http.StatusTemporaryRedirect)
			return
		}

		//For Plugin Routing
		if strings.HasPrefix(r.URL.Path, "/plugin.ui/") {
			//Extract the plugin ID from the request path
			parts := strings.Split(r.URL.Path, "/")
			if len(parts) > 2 {
				//There is always a prefix slash, so [2] is the plugin ID
				pluginID := parts[2]
				pluginManager.HandlePluginUI(pluginID, w, r)
			} else {
				http.Error(w, "Invalid Usage", http.StatusInternalServerError)
			}
			return
		}

		//For WebSSH Routing
		//Example URL Path: /web.ssh/{{instance_uuid}}/*
		if strings.HasPrefix(r.URL.Path, "/web.ssh/") {
			requestPath := r.URL.Path
			parts := strings.Split(requestPath, "/")
			if !strings.HasSuffix(requestPath, "/") && len(parts) == 3 {
				http.Redirect(w, r, requestPath+"/", http.StatusTemporaryRedirect)
				return
			}
			if len(parts) > 2 {
				//Extract the instance ID from the request path
				instanceUUID := parts[2]
				//fmt.Println(instanceUUID)

				//Rewrite the url so the proxy knows how to serve stuffs
				r.URL, _ = sshprox.RewriteURL("/web.ssh/"+instanceUUID, r.RequestURI)
				webSshManager.HandleHttpByInstanceId(instanceUUID, w, r)
			} else {
				//fmt.Println(parts)
				http.Error(w, "Invalid Usage", http.StatusInternalServerError)
			}
			return
		}

		//Authenticated
		if isHTMLFilePath(r.URL.Path) {
			handleInjectHTML(w, r, r.URL.Path)
			return
		}
		serveStaticWithCache(w, r, handler)
	})
}

/*
	Cache-Control / ETag utilities for static web resources.

	The web UI is either served from an embedded filesystem (production) or
	from disk (development). Embedded files report a zero modtime, which means
	http.FileServer cannot emit Last-Modified and browsers have no validator to
	reuse their cached copy. This results in every script (jquery, semantic,
	utils.js) being re-downloaded on each iframe load. By emitting a strong
	content based ETag with "Cache-Control: no-cache", browsers always
	revalidate but can serve the cached body (304) instead of re-downloading it.
*/

// staticETagCache caches path -> ETag. Only used in production, where the
// embedded resources are immutable for the lifetime of the process.
var staticETagCache sync.Map

// computeETag returns a quoted strong ETag derived from the content hash.
func computeETag(content []byte) string {
	sum := sha256.Sum256(content)
	return `"` + hex.EncodeToString(sum[:16]) + `"`
}

// readWebResource loads a web resource by its request URL path, from the
// embedded filesystem in production or from the "web" folder in development.
func readWebResource(urlPath string) ([]byte, error) {
	rel := strings.TrimPrefix(urlPath, "/")
	if rel == "" || strings.Contains(rel, "..") {
		return nil, os.ErrNotExist
	}
	if *development_build {
		return os.ReadFile(filepath.Join("web", rel))
	}
	return webres.ReadFile("web/" + rel)
}

// getStaticETag returns the ETag of a static resource, or an empty string when
// the resource cannot be read. In production the hash is cached after the
// first read; in development it is recomputed on each request to support hot
// reload.
func getStaticETag(urlPath string) string {
	if !*development_build {
		if cached, ok := staticETagCache.Load(urlPath); ok {
			return cached.(string)
		}
	}
	content, err := readWebResource(urlPath)
	if err != nil {
		return ""
	}
	etag := computeETag(content)
	if !*development_build {
		staticETagCache.Store(urlPath, etag)
	}
	return etag
}

// serveStaticWithCache attaches revalidatable cache headers to static
// resources before delegating to the underlying file server, which will then
// handle If-None-Match and reply with 304 when appropriate.
func serveStaticWithCache(w http.ResponseWriter, r *http.Request, next http.Handler) {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		if etag := getStaticETag(r.URL.Path); etag != "" {
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("ETag", etag)
		}
	}
	next.ServeHTTP(w, r)
}

func isHTMLFilePath(requestURI string) bool {
	return strings.HasSuffix(requestURI, ".html") || strings.HasSuffix(requestURI, "/")
}

// Serve the html file with template token injected
func handleInjectHTML(w http.ResponseWriter, r *http.Request, relativeFilepath string) {
	// Read the HTML file
	if len(relativeFilepath) > 0 && relativeFilepath[len(relativeFilepath)-1:] == "/" {
		relativeFilepath = relativeFilepath + "index.html"
	}
	content, err := readWebResource(relativeFilepath)
	if err != nil {
		SystemWideLogger.Println("Load web file failed: ", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Convert the file content to a string
	htmlContent := string(content)

	//Defeine the system template for this request
	templateStrings := map[string]string{
		".csrfToken": csrf.Token(r),
	}

	// Replace template tokens in the HTML content
	for key, value := range templateStrings {
		placeholder := "{{" + key + "}}"
		htmlContent = strings.ReplaceAll(htmlContent, placeholder, value)
	}

	// Emit a strong ETag over the final body. The rendered content includes the
	// per-session CSRF token, so the validator is session specific. Combined
	// with "Cache-Control: no-cache" the browser revalidates on every load and
	// reuses the cached body (304) when nothing changed.
	body := []byte(htmlContent)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("ETag", computeETag(body))
	http.ServeContent(w, r, filepath.Base(relativeFilepath), time.Time{}, bytes.NewReader(body))
}
