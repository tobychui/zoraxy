package webserv

import (
	"net/http"
	"path/filepath"
	"strings"

	"imuslab.com/zoraxy/mod/utils"
)

// Convert a request path (e.g. /index.html) into physical path on disk
func (ws *WebServer) resolveFileDiskPath(requestPath string) string {
	fileDiskpath := filepath.Join(ws.option.WebRoot, "html", requestPath)

	//Force convert it to slash even if the host OS is on Windows
	fileDiskpath = filepath.Clean(fileDiskpath)
	fileDiskpath = strings.ReplaceAll(fileDiskpath, "\\", "/")
	return fileDiskpath

}

// File server middleware to handle directory listing (and future expansion)
func (ws *WebServer) fsMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ws.option.EnableDirectoryListing {
			if strings.HasSuffix(r.URL.Path, "/") {
				//This is a folder. Let check if index exists
				if utils.FileExists(filepath.Join(ws.resolveFileDiskPath(r.URL.Path), "index.html")) {

				} else if utils.FileExists(filepath.Join(ws.resolveFileDiskPath(r.URL.Path), "index.htm")) {

				} else {
					http.NotFound(w, r)
					return
				}
			}
		}

		h.ServeHTTP(w, r)
	})
}
