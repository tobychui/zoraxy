package utils

import (
	"net/http"
)

/*
	Web Template Generator

	This is the main system core module that perform function similar to what PHP did.
	To replace part of the content of any file, use {{paramter}} to replace it.


*/

func SendHTMLResponse(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(msg))
}
