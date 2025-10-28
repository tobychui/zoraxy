package modh2c

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

func TestH2CRoundTripper_RoundTrip(t *testing.T) {
	// Create a test server that supports h2c
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Proto != "HTTP/2.0" {
			t.Errorf("Expected HTTP/2.0, got %s", r.Proto)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello, h2c!"))
	})

	server := httptest.NewServer(h2c.NewHandler(mux, &http2.Server{}))
	defer server.Close()

	// Create the round tripper
	rt := NewH2CRoundTripper()

	// Create a request
	req, err := http.NewRequest("GET", server.URL, nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Perform the round trip
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	// Check the response body
	body := make([]byte, 1024)
	n, err := resp.Body.Read(body)
	if err != nil && err.Error() != "EOF" {
		t.Fatalf("Failed to read body: %v", err)
	}
	if string(body[:n]) != "Hello, h2c!" {
		t.Errorf("Expected 'Hello, h2c!', got '%s'", string(body[:n]))
	}
}

func TestH2CRoundTripper_CheckServerSupportsH2C(t *testing.T) {
	// Test with h2c server
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := httptest.NewServer(h2c.NewHandler(mux, &http2.Server{}))
	defer server.Close()

	rt := NewH2CRoundTripper()
	supports := rt.CheckServerSupportsH2C(server.URL)
	if !supports {
		t.Error("Expected server to support h2c")
	}

	// Test with non-h2c server (regular HTTP/1.1)
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server2.Close()

	supports2 := rt.CheckServerSupportsH2C(server2.URL)
	if supports2 {
		t.Error("Expected server to not support h2c")
	}
}
