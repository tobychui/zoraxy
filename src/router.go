package main

import (
	"net/http"
	"strings"
)

/*
	router.go

	This script holds the static resources router
	for the reverse proxy service
*/

func AuthFsHandler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow access to /script/*, /img/pubic/* and /login.html without authentication
		if strings.HasPrefix(r.URL.Path, "/script/") || strings.HasPrefix(r.URL.Path, "/img/public/") || r.URL.Path == "/login.html" || r.URL.Path == "/favicon.png" {
			handler.ServeHTTP(w, r)
			return
		}

		// check authentication
		if !authAgent.CheckAuth(r) {
			http.Redirect(w, r, "/login.html", http.StatusTemporaryRedirect)
			return
		}

		//Authenticated
		handler.ServeHTTP(w, r)
	})
}
