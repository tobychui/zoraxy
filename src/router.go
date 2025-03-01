package main

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

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
		/*
			Development Mode Override
			=> Web root is located in /
		*/
		if DEVELOPMENT_BUILD && strings.HasPrefix(r.URL.Path, "/web/") {
			u, _ := url.Parse(strings.TrimPrefix(r.URL.Path, "/web"))
			r.URL = u
		}

		/*
			Production Mode Override
			=> Web root is located in /web
		*/
		if !DEVELOPMENT_BUILD && r.URL.Path == "/" {
			//Redirect to web UI
			http.Redirect(w, r, "/web/", http.StatusTemporaryRedirect)
			return
		}

		// Allow access to /script/*, /img/pubic/* and /login.html without authentication
		if strings.HasPrefix(r.URL.Path, ppf("/script/")) || strings.HasPrefix(r.URL.Path, ppf("/img/public/")) || r.URL.Path == ppf("/login.html") || r.URL.Path == ppf("/reset.html") || r.URL.Path == ppf("/favicon.png") {
			if isHTMLFilePath(r.URL.Path) {
				handleInjectHTML(w, r, r.URL.Path)
				return
			}
			handler.ServeHTTP(w, r)
			return
		}

		// Check authentication
		if !authAgent.CheckAuth(r) && requireAuth {
			http.Redirect(w, r, ppf("/login.html"), http.StatusTemporaryRedirect)
			return
		}

		//For Plugin Routing
		if strings.HasPrefix(r.URL.Path, "/plugin.ui/") {
			//Extract the plugin ID from the request path
			parts := strings.Split(r.URL.Path, "/")
			if len(parts) > 2 {
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
				fmt.Println(instanceUUID)

				//Rewrite the url so the proxy knows how to serve stuffs
				r.URL, _ = sshprox.RewriteURL("/web.ssh/"+instanceUUID, r.RequestURI)
				webSshManager.HandleHttpByInstanceId(instanceUUID, w, r)
			} else {
				fmt.Println(parts)
				http.Error(w, "Invalid Usage", http.StatusInternalServerError)
			}
			return
		}

		//Authenticated
		if isHTMLFilePath(r.URL.Path) {
			handleInjectHTML(w, r, r.URL.Path)
			return
		}
		handler.ServeHTTP(w, r)
	})
}

// Production path fix wrapper. Fix the path on production or development environment
func ppf(relativeFilepath string) string {
	if !DEVELOPMENT_BUILD {
		return strings.ReplaceAll(filepath.Join("/web/", relativeFilepath), "\\", "/")
	}
	return relativeFilepath
}

func isHTMLFilePath(requestURI string) bool {
	return strings.HasSuffix(requestURI, ".html") || strings.HasSuffix(requestURI, "/")
}

// Serve the html file with template token injected
func handleInjectHTML(w http.ResponseWriter, r *http.Request, relativeFilepath string) {
	// Read the HTML file
	var content []byte
	var err error
	if len(relativeFilepath) > 0 && relativeFilepath[len(relativeFilepath)-1:] == "/" {
		relativeFilepath = relativeFilepath + "index.html"
	}
	if DEVELOPMENT_BUILD {
		//Load from disk
		targetFilePath := strings.ReplaceAll(filepath.Join("web/", relativeFilepath), "\\", "/")
		content, err = os.ReadFile(targetFilePath)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
	} else {
		//Load from embedded fs, require trimming off the prefix slash for relative path
		relativeFilepath = strings.TrimPrefix(relativeFilepath, "/")
		content, err = webres.ReadFile(relativeFilepath)
		if err != nil {
			SystemWideLogger.Println("load embedded web file failed: ", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
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

	// Write the modified HTML content to the response
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(htmlContent))
}
