package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/fsnotify/fsnotify"
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

	webserver_stop_chan = make(chan bool, 1)
)

func main() {

	flag.Parse()

	if (*root_url)[0] != '/' {
		*root_url = "/" + *root_url
	}

	switch *mode {
	case "build":
		build()
	default:
		go watchDocsChange()
		fmt.Println("Running in web mode")
		startWebServerInBackground()
		select {}
	}
}

func startWebServerInBackground() {
	go func() {
		http.DefaultServeMux = http.NewServeMux()
		server := &http.Server{Addr: ":8080", Handler: http.DefaultServeMux}
		http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.FileServer(http.Dir("./")).ServeHTTP(w, r)
		})

		go func() {
			<-webserver_stop_chan
			fmt.Println("Stopping server at :8080")
			if err := server.Close(); err != nil {
				log.Println("Error stopping server:", err)
			}
		}()

		fmt.Println("Starting server at :8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
}

func watchDocsChange() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	err = watcher.Add("./docs")
	if err != nil {
		log.Fatal(err)
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
				log.Println("Change detected in docs folder:", event)
				webserver_stop_chan <- true
				time.Sleep(1 * time.Second) // Allow server to stop gracefully
				build()
				startWebServerInBackground()
				log.Println("Static html files regenerated")
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Println("Watcher error:", err)
		}
	}
}
