package main

import (
	_ "embed"
	"fmt"
	"net/http"
	"sort"
)

// Render the debug UI
func RenderDebugUI(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "**Plugin UI Debug Interface**\n\n[Recv Headers] \n")

	headerKeys := make([]string, 0, len(r.Header))
	for name := range r.Header {
		headerKeys = append(headerKeys, name)
	}
	sort.Strings(headerKeys)
	for _, name := range headerKeys {
		values := r.Header[name]
		for _, value := range values {
			fmt.Fprintf(w, "%s: %s\n", name, value)
		}
	}
	w.Header().Set("Content-Type", "text/html")
}
