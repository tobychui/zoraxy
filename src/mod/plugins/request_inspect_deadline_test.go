package plugins

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// deadlineEntry serves InspectRoute on a real connection and echoes allowed
// request bodies after an optional upstream delay.
func deadlineEntry(t *testing.T, m *Manager, upstreamDelay time.Duration) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var upstream atomic.Int64
	entry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if handled, _ := m.InspectRoute(w, r, inspectTarget("example.test")); handled {
			return
		}
		upstream.Add(1)
		time.Sleep(upstreamDelay)
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		if err := r.Context().Err(); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Write(data)
	}))
	t.Cleanup(entry.Close)
	return entry, &upstream
}

func TestInspectSlowUploadDeadline(t *testing.T) {
	for _, mode := range []string{"report", "block"} {
		t.Run(mode, func(t *testing.T) {
			m, fake := policyManager(t, `{"mode":"`+mode+`","body_timeout_ms":200}`, `{"action":"allow"}`)
			entry, upstream := deadlineEntry(t, m, 0)
			conn, err := net.Dial("tcp", entry.Listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			started := time.Now()
			fmt.Fprintf(conn, "POST / HTTP/1.1\r\nHost: example.test\r\nContent-Length: 1000\r\n\r\n0123456789")
			conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
			if err != nil {
				t.Fatal(err)
			}
			elapsed := time.Since(started)
			if resp.StatusCode != http.StatusRequestTimeout {
				t.Fatalf("status=%d, want 408", resp.StatusCode)
			}
			if elapsed < 200*time.Millisecond || elapsed > 2*time.Second {
				t.Fatalf("body deadline not applied: %v", elapsed)
			}
			if _, calls := fake.counts(); calls != 0 || upstream.Load() != 0 {
				t.Fatalf("partial upload forwarded: inspections=%d upstream=%d", calls, upstream.Load())
			}
		})
	}
}

// The deadline covers only the inspected prefix: the remainder and a slow
// upstream must not be cut off by it.
func TestInspectBodyDeadlineReset(t *testing.T) {
	m, fake := policyManager(t, `{"mode":"report","max_body_kb":1,"body_timeout_ms":200}`, `{"action":"allow"}`)
	entry, upstream := deadlineEntry(t, m, 400*time.Millisecond)
	head := strings.Repeat("a", 2048)
	tail := strings.Repeat("b", 2048)
	pr, pw := io.Pipe()
	go func() {
		pw.Write([]byte(head))
		time.Sleep(500 * time.Millisecond)
		pw.Write([]byte(tail))
		pw.Close()
	}()
	req, _ := http.NewRequest("POST", entry.URL, pr)
	req.ContentLength = int64(len(head) + len(tail))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(got) != head+tail {
		t.Fatalf("status=%d len=%d", resp.StatusCode, len(got))
	}
	if _, calls := fake.counts(); calls != 1 || upstream.Load() != 1 {
		t.Fatalf("inspections=%d upstream=%d", calls, upstream.Load())
	}

	//Small complete body with a slow upstream: the server's background read
	//must not inherit the expired deadline and cancel the request.
	resp, err = http.Post(entry.URL, "text/plain", strings.NewReader("small"))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, _ = io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(got) != "small" {
		t.Fatalf("status=%d body=%q", resp.StatusCode, got)
	}

	w := newDeadlineRecorder()
	m.InspectRoute(w, httptest.NewRequest("POST", "http://example.test/", strings.NewReader("x")), inspectTarget("example.test"))
	if len(w.deadlines) != 2 || w.deadlines[0].IsZero() || !w.deadlines[1].IsZero() {
		t.Fatalf("deadline not set and reset: %v", w.deadlines)
	}
}

func TestInspectDeadlineUnsupported(t *testing.T) {
	for _, tc := range []struct {
		policy string
		status int
	}{
		{`{"mode":"block","fail_closed":true}`, 503},
		{`{"mode":"block"}`, 0},
		{`{"mode":"report","fail_closed":true}`, 0},
	} {
		t.Run(tc.policy, func(t *testing.T) {
			m, fake := policyManager(t, tc.policy, `{"action":"allow"}`)
			r := httptest.NewRequest("POST", "http://example.test/", strings.NewReader("body"))
			original := r.Body
			w := httptest.NewRecorder() //no read deadline support
			if _, status := m.InspectRoute(w, r, inspectTarget("example.test")); status != tc.status {
				t.Fatalf("status=%d want %d", status, tc.status)
			}
			if _, calls := fake.counts(); calls != 0 || r.Body != original {
				t.Fatal("body read without a deadline")
			}
		})
	}
	//Requests without a body do not need a deadline.
	m, fake := policyManager(t, `{"mode":"block"}`, `{"action":"allow"}`)
	m.InspectRoute(httptest.NewRecorder(), httptest.NewRequest("GET", "http://example.test/", nil), inspectTarget("example.test"))
	if _, calls := fake.counts(); calls != 1 {
		t.Fatalf("bodyless request not inspected: %d", calls)
	}
}
