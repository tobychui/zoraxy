package main

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

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
		if development && strings.HasPrefix(r.URL.Path, "/web/") {
			u, _ := url.Parse(strings.TrimPrefix(r.URL.Path, "/web"))
			r.URL = u
		}

		/*
			Production Mode Override
			=> Web root is located in /web
		*/
		if !development && r.URL.Path == "/" {
			//Redirect to web UI
			http.Redirect(w, r, "/web/", http.StatusTemporaryRedirect)
			return
		}

		// Allow access to /script/*, /img/pubic/* and /login.html without authentication
		if strings.HasPrefix(r.URL.Path, ppf("/script/")) || strings.HasPrefix(r.URL.Path, ppf("/img/public/")) || r.URL.Path == ppf("/login.html") || r.URL.Path == ppf("/reset.html") || r.URL.Path == ppf("/favicon.png") {
			handler.ServeHTTP(w, r)
			return
		}

		// check authentication
		if !authAgent.CheckAuth(r) && requireAuth {
			http.Redirect(w, r, ppf("/login.html"), http.StatusTemporaryRedirect)
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
		handler.ServeHTTP(w, r)
	})
}

// Production path fix wrapper. Fix the path on production or development environment
func ppf(relativeFilepath string) string {
	if !development {
		return strings.ReplaceAll(filepath.Join("/web/", relativeFilepath), "\\", "/")
	}
	return relativeFilepath
}
