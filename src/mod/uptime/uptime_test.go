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
