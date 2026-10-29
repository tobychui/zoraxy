package plugins

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"imuslab.com/zoraxy/mod/plugins/zoraxy_plugin"
)

func inspectManager(t *testing.T, handler http.HandlerFunc) (*Manager, *Plugin) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	p := &Plugin{Enabled: true, AssignedPort: port, Spec: &zoraxy_plugin.IntroSpect{ID: "test", RequestInspect: "/inspect"}}
	m := &Manager{
		LoadedPlugins: map[string]*Plugin{"test": p},
		Options:       &ManagerOptions{PluginGroups: map[string][]string{"waf": {"test"}}},
	}
	m.UpdateTagsToPluginMaps()
	return m, p
}

// republish applies test mutations of plugin state the way lifecycle code does.
func republish(m *Manager) { m.UpdateTagsToPluginMaps() }

// deadlineRecorder is a ResponseRecorder that supports read deadlines like a real connection.
type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadlines []time.Time
}

func newDeadlineRecorder() *deadlineRecorder {
	return &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
}

func (d *deadlineRecorder) SetReadDeadline(t time.Time) error {
	d.deadlines = append(d.deadlines, t)
	return nil
}

type brokenInspectBody struct {
	sent, closed bool
	err          error
}

func (b *brokenInspectBody) Read(p []byte) (int, error) {
	if b.sent {
		return 0, io.EOF
	}
	b.sent = true
	return copy(p, "prefix"), b.err
}
func (b *brokenInspectBody) Close() error { b.closed = true; return nil }

func TestInspectReadError(t *testing.T) {
	for _, closed := range []bool{false, true} {
		t.Run(strconv.FormatBool(closed), func(t *testing.T) {
			m, p := inspectManager(t, func(w http.ResponseWriter, r *http.Request) { t.Error("partial/error body must not reach inspector") })
			p.Spec.RequestInspectFailClosed = closed
			republish(m)
			readErr := errors.New("body read failed")
			body := &brokenInspectBody{err: readErr}
			r := httptest.NewRequest("POST", "http://example.test/", nil)
			r.Body = body
			w := newDeadlineRecorder()
			if got := m.InspectRequest(w, r, []string{"waf"}, ""); got != closed {
				t.Fatalf("handled=%t", got)
			}
			data, err := io.ReadAll(r.Body)
			if string(data) != "prefix" || !errors.Is(err, readErr) {
				t.Fatalf("replay got %q, %v", data, err)
			}
			r.Body.Close()
			if !body.closed {
				t.Fatal("original body not closed")
			}
		})
	}
}

func TestInspectMultiplePlugins(t *testing.T) {
	body := strings.Repeat("x", 3072)
	seen := make(chan int, 2)
	first, p1 := inspectManager(t, func(w http.ResponseWriter, r *http.Request) {
		var payload zoraxy_plugin.RequestInspectPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		seen <- len(payload.Body)
		io.WriteString(w, `{"action":"allow"}`)
	})
	_, p2 := inspectManager(t, func(w http.ResponseWriter, r *http.Request) {
		var payload zoraxy_plugin.RequestInspectPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		seen <- len(payload.Body)
		io.WriteString(w, `{"action":"allow"}`)
	})
	p1.Spec.RequestInspectMaxBody = 1
	p2.Spec.RequestInspectMaxBody = 2
	p2.Spec.ID = "test2"
	first.LoadedPlugins["test2"] = p2
	first.Options.PluginGroups["waf"] = []string{"test", "test2"}
	republish(first)
	r := httptest.NewRequest("POST", "http://example.test/", strings.NewReader(body))
	if first.InspectRequest(newDeadlineRecorder(), r, []string{"waf"}, "") {
		t.Fatal("allow handled")
	}
	if len(seen) != 2 {
		t.Fatalf("calls=%d", len(seen))
	}
	if a, b := <-seen, <-seen; a != 1024 || b != 2048 {
		t.Fatalf("body prefixes=%d/%d", a, b)
	}
	got, err := io.ReadAll(r.Body)
	if err != nil || string(got) != body {
		t.Fatalf("body replay: len=%d err=%v", len(got), err)
	}
	r.Body.Close()
}

func TestInspectEmptyBody(t *testing.T) {
	m, _ := inspectManager(t, func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload["body"] != "" || payload["body_truncated"] != false {
			t.Errorf("invalid empty body: %+v", payload)
		}
		io.WriteString(w, `{"action":"allow"}`)
	})
	if m.InspectRequest(httptest.NewRecorder(), httptest.NewRequest("GET", "http://example.test/", nil), []string{"waf"}, "") {
		t.Fatal("allow handled")
	}
}

func TestInspectSelection(t *testing.T) {
	for _, mode := range []string{"no_field", "disabled", "no_tags", "duplicate_tags"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			m, p := inspectManager(t, func(w http.ResponseWriter, r *http.Request) { calls++; io.WriteString(w, `{"action":"allow"}`) })
			tags := []string{"waf", "other", "waf"}
			m.Options.PluginGroups["other"] = []string{"test"}
			switch mode {
			case "no_field":
				p.Spec.RequestInspect = ""
			case "disabled":
				p.Enabled = false
			case "no_tags":
				tags = nil
			}
			republish(m)
			r := httptest.NewRequest("POST", "http://example.test/", strings.NewReader("untouched"))
			original := r.Body
			w := newDeadlineRecorder()
			if m.InspectRequest(w, r, tags, "") {
				t.Fatal("allow handled request")
			}
			want := 0
			if mode == "duplicate_tags" {
				want = 1
			} else if r.Body != original {
				t.Fatal("opt-out touched body")
			}
			if calls != want {
				t.Fatalf("calls=%d want=%d", calls, want)
			}
		})
	}
}

func TestInspectBodyLimit(t *testing.T) {
	for _, tc := range []struct {
		kb, size, want int
		truncated      bool
	}{
		{1, 1024, 1024, false}, {1, 1025, 1024, true}, {0, 65537, 65536, true}, {-1, 65537, 65536, true}, {2048, 1048577, 1048576, true},
	} {
		t.Run(strconv.Itoa(tc.kb)+"/"+strconv.Itoa(tc.size), func(t *testing.T) {
			body := strings.Repeat("x", tc.size)
			m, p := inspectManager(t, func(w http.ResponseWriter, r *http.Request) {
				var payload zoraxy_plugin.RequestInspectPayload
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				if len(payload.Body) != tc.want || payload.BodyTruncated != tc.truncated {
					t.Errorf("captured %d bytes truncated=%t; want %d/%t", len(payload.Body), payload.BodyTruncated, tc.want, tc.truncated)
				}
				io.WriteString(w, `{"action":"allow"}`)
			})
			p.Spec.RequestInspectMaxBody = tc.kb
			republish(m)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				data, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				w.Write(data)
			}))
			defer upstream.Close()
			target, _ := url.Parse(upstream.URL)
			proxy := httputil.NewSingleHostReverseProxy(target)
			entry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !m.InspectRequest(w, r, []string{"waf"}, "") {
					proxy.ServeHTTP(w, r)
				}
			}))
			defer entry.Close()
			resp, err := http.Post(entry.URL, "text/plain", strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			got, err := io.ReadAll(resp.Body)
			if err != nil || string(got) != body {
				t.Fatalf("upstream body changed: len=%d err=%v", len(got), err)
			}
		})
	}
}

func TestInspectErrors(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"timeout", func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(2500 * time.Millisecond)
			io.WriteString(w, `{"action":"deny"}`)
		}},
		{"bad_json", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `garbage`) }},
		{"unknown_action", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{"action":"capture"}`) }},
		{"non_200", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(500)
			io.WriteString(w, `{"action":"allow"}`)
		}},
		{"oversized", func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, `{"action":"allow","reason":"`+strings.Repeat("x", 4096)+`"}`)
		}},
		{"redirect", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "http://127.0.0.1:1/", http.StatusTemporaryRedirect) }},
	}
	for _, tc := range cases {
		for _, closed := range []bool{false, true} {
			t.Run(tc.name+"/closed="+strconv.FormatBool(closed), func(t *testing.T) {
				m, p := inspectManager(t, tc.handler)
				p.Spec.RequestInspectFailClosed = closed
				republish(m)
				w := httptest.NewRecorder()
				started := time.Now()
				handled := m.InspectRequest(w, httptest.NewRequest("GET", "http://example.test/", nil), []string{"waf"}, "")
				if handled != closed {
					t.Fatalf("handled=%t, fail_closed=%t", handled, closed)
				}
				if closed && w.Code != 503 {
					t.Fatalf("got status %d, want 503", w.Code)
				}
				if !closed && w.Body.Len() != 0 {
					t.Fatal("fail-open wrote response")
				}
				if tc.name == "timeout" && time.Since(started) > 2400*time.Millisecond {
					t.Fatal("inspection timeout not enforced")
				}
			})
		}
	}
}

func TestInspectDeny(t *testing.T) {
	for _, status := range []int{0, 429, 200, 999} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			m, _ := inspectManager(t, func(w http.ResponseWriter, r *http.Request) {
				json.NewEncoder(w).Encode(zoraxy_plugin.RequestInspectResult{Action: "deny", Status: status, Reason: "blocked query"})
			})
			w := httptest.NewRecorder()
			if !m.InspectRequest(w, httptest.NewRequest("GET", "http://example.test/?block=1", nil), []string{"waf"}, "192.0.2.1") {
				t.Fatal("deny did not stop routing")
			}
			want := status
			if status < 400 || status > 599 {
				want = 403
			}
			if w.Code != want || w.Body.String() != "blocked query\n" {
				t.Fatalf("got %d %q", w.Code, w.Body.String())
			}
		})
	}
}

func TestInspectAllowReplaysBody(t *testing.T) {
	body := "original request body"
	calls := 0
	m, _ := inspectManager(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/inspect" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("bad inspection request: %s %s", r.Method, r.URL.Path)
		}
		var payload zoraxy_plugin.RequestInspectPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if string(payload.Body) != body || payload.BodyTruncated || payload.ClientIP != "192.0.2.1" {
			t.Errorf("bad payload: %+v", payload)
		}
		json.NewEncoder(w).Encode(zoraxy_plugin.RequestInspectResult{Action: "allow"})
	})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Write(data)
	}))
	defer upstream.Close()
	target, _ := url.Parse(upstream.URL)
	proxy := httputil.NewSingleHostReverseProxy(target)
	entry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.InspectRequest(w, r, []string{"waf"}, "192.0.2.1") {
			proxy.ServeHTTP(w, r)
		}
	}))
	defer entry.Close()
	resp, err := http.Post(entry.URL, "text/plain", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(got) != body {
		t.Fatalf("upstream got %d %q", resp.StatusCode, got)
	}
	if calls != 1 {
		t.Fatalf("inspector calls = %d, want 1", calls)
	}
}
