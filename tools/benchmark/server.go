package main

import (
	"fmt"
	"net/http"
	"time"
)

// Start the web server for reciving test request
// in Zoraxy, point test.localhost to this server at the given port in the start variables
func startWebServer() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Print the request details to console
		fmt.Printf("Timestamp: %s\n", time.Now().Format(time.RFC1123))
		fmt.Printf("Request type: %s\n", r.Method)
		fmt.Printf("Payload size: %d bytes\n", r.ContentLength)
		fmt.Printf("Request URI: %s\n", r.RequestURI)
		fmt.Printf("User Agent: %s\n", r.UserAgent())
		fmt.Printf("Remote Address: %s\n", r.RemoteAddr)
		fmt.Println("----------------------------------------")

		//Set header to text
		w.Header().Set("Content-Type", "text/plain")
		// Send response, print the request details to web page
		w.Write([]byte("----------------------------------------\n"))
		w.Write([]byte("Request type: " + r.Method + "\n"))
		w.Write([]byte(fmt.Sprintf("Payload size: %d bytes\n", r.ContentLength)))
		w.Write([]byte("Request URI: " + r.RequestURI + "\n"))
		w.Write([]byte("User Agent: " + r.UserAgent() + "\n"))
		w.Write([]byte("Remote Address: " + r.RemoteAddr + "\n"))
		w.Write([]byte("----------------------------------------\n"))
	})

	go func() {
		err := http.ListenAndServe(fmt.Sprintf(":%d", benchmarkWebserverListeningPort), nil)
		if err != nil {
			fmt.Printf("Failed to start server: %v\n", err)
			stopchan <- true
		}
	}()

}
