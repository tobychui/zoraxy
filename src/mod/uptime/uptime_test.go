package uptime

import (
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// This test reproduces GitHub issue #1241: getWebsiteStatus used to leak one
// upstream TCP connection per call because it built a fresh keep-alive
// http.Transport every time and never closed/reused it. It verifies that
// every connection opened by a check is closed again shortly after.
func TestGetWebsiteStatusClosesConnections(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	var opened, closed int64
	var mu sync.Mutex
	seen := map[string]bool{}
	server.Config.ConnState = func(c net.Conn, state http.ConnState) {
		key := c.RemoteAddr().String()
		switch state {
		case http.StateNew:
			mu.Lock()
			if !seen[key] {
				seen[key] = true
				atomic.AddInt64(&opened, 1)
			}
			mu.Unlock()
		case http.StateClosed:
			atomic.AddInt64(&closed, 1)
		}
	}

	m := &Monitor{Config: &Config{}}

	const n = 5
	for i := 0; i < n; i++ {
		code, err := m.getWebsiteStatus(server.URL, false, 2*time.Second)
		if err != nil {
			t.Fatalf("check %d failed: %v", i, err)
		}
		if code != http.StatusOK {
			t.Fatalf("check %d unexpected status: %d", i, code)
		}
	}

	// Give the transport a moment to finish tearing down the connection.
	time.Sleep(300 * time.Millisecond)

	closedNow := atomic.LoadInt64(&closed)
	if closedNow < n {
		t.Fatalf("expected at least %d closed connections after %d checks, got %d closed (opened=%d) - connections are leaking", n, n, closedNow, atomic.LoadInt64(&opened))
	}
}

// GitHub issue #1369: a target URL that net/url cannot parse (for example a
// host with a space, or a health-check URI with a bad percent escape) made
// http.NewRequest return a nil request, and getWebsiteStatus panicked on it,
// taking the whole process down. It should report an error instead.
func TestGetWebsiteStatusInvalidURL(t *testing.T) {
	m := &Monitor{Config: &Config{}}
	urls := []string{
		"http://bad host:8080",
		"https://bad host:8443",
		buildHealthCheckURL("http://127.0.0.1:8080", "/health%zz"),
		"http://[fe80::1%eth0]:8080",
	}
	for _, u := range urls {
		code, err := m.getWebsiteStatus(u, false, time.Second)
		if err == nil {
			t.Errorf("%q: expected an error, got status %d", u, code)
		}
		if code != 0 {
			t.Errorf("%q: expected status 0, got %d", u, code)
		}
	}
}
