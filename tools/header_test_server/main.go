package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"sort"
	"strings"
)

func main() {
	// Create a custom server that only supports HTTP/1.1
	server := &http.Server{
		Addr:    ":8080",
		Handler: http.HandlerFunc(handleRequest),
	}

	log.Println("Starting HTTP/1.1 test server on :8080")
	log.Println("Only HTTP/1.1 connections are allowed")

	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	// Check if the request is HTTP/1.1
	if r.ProtoMajor != 1 || r.ProtoMinor != 1 {
		w.WriteHeader(http.StatusHTTPVersionNotSupported)
		fmt.Fprintf(w, "Error: Only HTTP/1.1 is supported. Received: %s\n", r.Proto)
		log.Printf("Rejected connection: %s from %s", r.Proto, r.RemoteAddr)
		return
	}

	// Set response headers
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	// Build response with all request information
	var response strings.Builder

	response.WriteString("========================================\n")
	response.WriteString("HTTP/1.1 TEST SERVER - REQUEST INFO\n")
	response.WriteString("========================================\n\n")

	// Protocol Information
	response.WriteString("PROTOCOL INFORMATION:\n")
	response.WriteString(fmt.Sprintf("  Protocol: %s\n", r.Proto))
	response.WriteString(fmt.Sprintf("  Major Version: %d\n", r.ProtoMajor))
	response.WriteString(fmt.Sprintf("  Minor Version: %d\n", r.ProtoMinor))
	response.WriteString("\n")

	// Request Line
	response.WriteString("REQUEST LINE:\n")
	response.WriteString(fmt.Sprintf("  Method: %s\n", r.Method))
	response.WriteString(fmt.Sprintf("  URL: %s\n", r.URL.String()))
	response.WriteString(fmt.Sprintf("  Path: %s\n", r.URL.Path))
	response.WriteString(fmt.Sprintf("  RawQuery: %s\n", r.URL.RawQuery))
	response.WriteString("\n")

	// Connection Information
	response.WriteString("CONNECTION INFORMATION:\n")
	response.WriteString(fmt.Sprintf("  Remote Address: %s\n", r.RemoteAddr))

	// Parse IP and Port
	host, port, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		response.WriteString(fmt.Sprintf("  Remote IP: %s\n", host))
		response.WriteString(fmt.Sprintf("  Remote Port: %s\n", port))

		// Determine IP type
		ip := net.ParseIP(host)
		if ip != nil {
			if ip.To4() != nil {
				response.WriteString("  IP Type: IPv4\n")
			} else {
				response.WriteString("  IP Type: IPv6\n")
			}
		}
	}

	// Local address (server side)
	if r.Context().Value(http.LocalAddrContextKey) != nil {
		localAddr := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
		response.WriteString(fmt.Sprintf("  Local Address: %s\n", localAddr.String()))
	}

	response.WriteString(fmt.Sprintf("  Host: %s\n", r.Host))
	response.WriteString("\n")

	// HTTP Headers
	response.WriteString("REQUEST HEADERS:\n")

	// Sort headers for consistent output
	var headerKeys []string
	for key := range r.Header {
		headerKeys = append(headerKeys, key)
	}
	sort.Strings(headerKeys)

	for _, key := range headerKeys {
		values := r.Header[key]
		for _, value := range values {
			response.WriteString(fmt.Sprintf("  %s: %s\n", key, value))
		}
	}
	response.WriteString("\n")

	// Connection Attributes
	response.WriteString("CONNECTION ATTRIBUTES:\n")
	response.WriteString(fmt.Sprintf("  Content Length: %d bytes\n", r.ContentLength))
	response.WriteString(fmt.Sprintf("  Transfer Encoding: %v\n", r.TransferEncoding))
	response.WriteString(fmt.Sprintf("  Close Connection: %v\n", r.Close))
	response.WriteString("\n")

	// TLS Information (if applicable)
	if r.TLS != nil {
		response.WriteString("TLS INFORMATION:\n")
		response.WriteString(fmt.Sprintf("  TLS Version: %d\n", r.TLS.Version))
		response.WriteString(fmt.Sprintf("  Cipher Suite: %d\n", r.TLS.CipherSuite))
		response.WriteString(fmt.Sprintf("  Server Name: %s\n", r.TLS.ServerName))
		response.WriteString(fmt.Sprintf("  Negotiated Protocol: %s\n", r.TLS.NegotiatedProtocol))
		response.WriteString("\n")
	} else {
		response.WriteString("TLS: Not enabled\n\n")
	}

	// HTTP/1.1 Specific Features
	response.WriteString("HTTP/1.1 SUPPORTED FEATURES:\n")
	response.WriteString("  ✓ Persistent Connections (Keep-Alive)\n")
	response.WriteString("  ✓ Chunked Transfer Encoding\n")
	response.WriteString("  ✓ Pipelining\n")
	response.WriteString("  ✓ Host Header Required\n")
	response.WriteString("  ✓ Cache Control\n")
	response.WriteString("  ✓ Range Requests\n")
	response.WriteString("\n")

	// Additional Request Information
	response.WriteString("ADDITIONAL REQUEST INFO:\n")
	response.WriteString(fmt.Sprintf("  Request URI: %s\n", r.RequestURI))
	response.WriteString(fmt.Sprintf("  Referer: %s\n", r.Referer()))
	response.WriteString(fmt.Sprintf("  User-Agent: %s\n", r.UserAgent()))
	response.WriteString("\n")

	// Form/Query Parameters (if any)
	if len(r.URL.Query()) > 0 {
		response.WriteString("QUERY PARAMETERS:\n")
		for key, values := range r.URL.Query() {
			for _, value := range values {
				response.WriteString(fmt.Sprintf("  %s = %s\n", key, value))
			}
		}
		response.WriteString("\n")
	}

	response.WriteString("========================================\n")
	response.WriteString("End of Request Information\n")
	response.WriteString("========================================\n")

	// Write response
	fmt.Fprint(w, response.String())

	// Log the request
	log.Printf("Accepted HTTP/1.1 request: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
}
