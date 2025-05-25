package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
)

type FileInfo struct {
	Filename string `json:"filename"`
	Title    string `json:"title"`
	Type     string `json:"type"`
}

/* Change this before deploying */
var (
	mode     = flag.String("m", "web", "Mode to run the application: 'web' or 'build'")
	root_url = flag.String("root", "/html/", "Root URL for the web server")
)

func main() {

	flag.Parse()

	switch *mode {
	case "build":
		build()
	default:
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.FileServer(http.Dir("./")).ServeHTTP(w, r)
		})
		fmt.Println("Starting server at :8080")
		if err := http.ListenAndServe(":8080", nil); err != nil {
			log.Fatal(err)
		}
	}
}
