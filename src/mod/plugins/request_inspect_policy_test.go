package plugins

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"imuslab.com/zoraxy/mod/plugins/zoraxy_plugin"
)

// policyInspector is a fake plugin serving /policy and /inspect.
type policyInspector struct {
	mu       sync.Mutex
	policy   string //served with HTTP 200 unless status is set
	status   int
	verdict  string //raw /inspect response, "" answers HTTP 500
	policies int
	calls    int
	payloads []map[string]any
}

func (f *policyInspector) set(fn func(*policyInspector)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

func (f *policyInspector) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.policies, f.calls
}

func (f *policyInspector) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.URL.Path {
	case "/policy":
		f.policies++
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if f.status != 0 {
			w.WriteHeader(f.status)
		}
		io.WriteString(w, f.policy)
	case "/inspect":
		f.calls++
		var payload map[string]any
		json.NewDecoder(r.Body).Decode(&payload)
		f.payloads = append(f.payloads, payload)
		if f.verdict == "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		io.WriteString(w, f.verdict)
	default:
		http.NotFound(w, r)
	}
}

func policyManager(t *testing.T, policy, verdict string) (*Manager, *policyInspector) {
	t.Helper()
	fake := &policyInspector{policy: policy, verdict: verdict}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	p := &Plugin{Enabled: true, AssignedPort: port, Spec: &zoraxy_plugin.IntroSpect{
		ID: "waf", RequestInspect: "/inspect", RequestInspectPolicy: "/policy",
	}}
	m := &Manager{
		LoadedPlugins: map[string]*Plugin{"waf": p},
		Options:       &ManagerOptions{PluginGroups: map[string][]string{"waf": {"waf"}}},
	}
	m.UpdateTagsToPluginMaps()
	return m, fake
}

func inspectTarget(routeID string) RequestInspectTarget {
	return RequestInspectTarget{RouteID: routeID, Tags: []string{"waf"}, ClientIP: "192.0.2.1"}
}

const denyVerdict = `{"action":"deny","status":403,"reason":"rule 942100","event_id":"ev-1"}`

func TestInspectPayloadProtocolAndRoute(t *testing.T) {
	m, fake := policyManager(t, `{"mode":"block"}`, `{"action":"allow","event_id":"ev-allow"}`)
	r := httptest.NewRequest("POST", "http://Example.test/a?b=1", strings.NewReader("abc"))
	if handled, _ := m.InspectRoute(newDeadlineRecorder(), r, inspectTarget("Example.test")); handled {
		t.Fatal("allow handled the request")
	}
	if len(fake.payloads) != 1 {
		t.Fatalf("inspections=%d", len(fake.payloads))
	}
	p := fake.payloads[0]
	if p["protocol_version"] != float64(1) || p["route_id"] != "Example.test" || p["body"] != "YWJj" || p["body_truncated"] != false || p["client_ip"] != "192.0.2.1" {
		t.Fatalf("payload: %+v", p)
	}
	var res zoraxy_plugin.RequestInspectResult
	json.Unmarshal([]byte(denyVerdict), &res)
	if res.EventID != "ev-1" {
		t.Fatalf("event id not decoded: %+v", res)
	}
}

func TestInspectPolicyRejectsInvalid(t *testing.T) {
	huge := `{"mode":"block"}` + strings.Repeat(" ", 128*1024)
	for name, policy := range map[string]string{
		"not_json":         `garbage`,
		"null":             `null`,
		"trailing_data":    `{"mode":"block"} {}`,
		"unknown_field":    `{"mode":"block","fail_close":true}`,
		"mode_inherit":     `{"mode":"inherit"}`,
		"mode_case":        `{"mode":"BLOCK"}`,
		"max_body_low":     `{"mode":"block","max_body_kb":-1}`,
		"max_body_high":    `{"mode":"block","max_body_kb":1025}`,
		"timeout_low":      `{"mode":"block","body_timeout_ms":99}`,
		"timeout_high":     `{"mode":"block","body_timeout_ms":30001}`,
		"host_uppercase":   `{"mode":"block","hosts":{"Example.test":{"mode":"off"}}}`,
		"host_empty":       `{"mode":"block","hosts":{"":{"mode":"off"}}}`,
		"host_path":        `{"mode":"block","hosts":{"/admin":{"mode":"off"}}}`,
		"host_path_suffix": `{"mode":"block","hosts":{"example.test/x":{"mode":"off"}}}`,
		"invert_path":      `{"mode":"block","invert_failure_hosts":["//"]}`,
		"invert_backslash": `{"mode":"block","invert_failure_hosts":["a\\b"]}`,
		"host_mode":        `{"mode":"block","hosts":{"example.test":{"mode":"deny"}}}`,
		"host_body_high":   `{"mode":"block","hosts":{"example.test":{"max_body_kb":2048}}}`,
		"host_timeout_low": `{"mode":"block","hosts":{"example.test":{"body_timeout_ms":1}}}`,
		"host_unknown":     `{"mode":"block","hosts":{"example.test":{"mode":"off","x":1}}}`,
		"invert_space":     `{"mode":"block","invert_failure_hosts":[" example.test"]}`,
		"invert_empty":     `{"mode":"block","invert_failure_hosts":[""]}`,
		"too_large":        huge,
	} {
		t.Run(name, func(t *testing.T) {
			m, fake := policyManager(t, policy, denyVerdict)
			w := newDeadlineRecorder()
			if handled, _ := m.InspectRoute(w, httptest.NewRequest("GET", "http://example.test/", nil), inspectTarget("example.test")); handled {
				t.Fatalf("invalid policy without last-known-good blocked: %d", w.Code)
			}
			if policies, calls := fake.counts(); policies != 1 || calls != 0 {
				t.Fatalf("policies=%d inspections=%d", policies, calls)
			}
		})
	}
	t.Run("http_status", func(t *testing.T) {
		m, fake := policyManager(t, `{"mode":"block"}`, denyVerdict)
		fake.status = http.StatusInternalServerError
		if handled, _ := m.InspectRoute(newDeadlineRecorder(), httptest.NewRequest("GET", "http://example.test/", nil), inspectTarget("example.test")); handled {
			t.Fatal("non-200 policy enforced")
		}
	})
	t.Run("valid_defaults", func(t *testing.T) {
		m, _ := policyManager(t, `{"mode":"block","hosts":{"*.example.test":{},"/":{"mode":"inherit"}}}`, denyVerdict)
		w := newDeadlineRecorder()
		if handled, status := m.InspectRoute(w, httptest.NewRequest("GET", "http://example.test/", nil), inspectTarget("example.test")); !handled || status != 403 || w.Code != 403 {
			t.Fatalf("valid policy not enforced: %t %d", handled, w.Code)
		}
	})
}

func TestInspectPolicyCacheAndLastKnownGood(t *testing.T) {
	defer func(ttl time.Duration) { inspectPolicyTTL = ttl }(inspectPolicyTTL)
	inspectPolicyTTL = 100 * time.Millisecond
	m, fake := policyManager(t, `{"mode":"block","fail_closed":true}`, "")
	run := func() int {
		w := newDeadlineRecorder()
		handled, status := m.InspectRoute(w, httptest.NewRequest("GET", "http://example.test/", nil), inspectTarget("example.test"))
		if handled != (status != 0) || (handled && w.Code != status) {
			t.Fatalf("handled=%t status=%d code=%d", handled, status, w.Code)
		}
		return status
	}
	//Engine failure in block mode with fail_closed.
	if s := run(); s != 503 {
		t.Fatalf("status=%d want 503", s)
	}
	run()
	if policies, calls := fake.counts(); policies != 1 || calls != 2 {
		t.Fatalf("cached policy refetched: policies=%d calls=%d", policies, calls)
	}
	//Policy endpoint fails: last-known-good fail_closed applies without inspection.
	fake.set(func(f *policyInspector) { f.status = 500; f.verdict = denyVerdict })
	time.Sleep(150 * time.Millisecond)
	if s := run(); s != 503 {
		t.Fatalf("status=%d, want 503 from last-known-good", s)
	}
	if policies, calls := fake.counts(); policies != 2 || calls != 2 {
		t.Fatalf("policies=%d calls=%d", policies, calls)
	}
	//Failed refreshes are not retried before the TTL either.
	run()
	if policies, _ := fake.counts(); policies != 2 {
		t.Fatalf("failed refresh retried within TTL: %d", policies)
	}
	//A new valid policy replaces last-known-good; report never blocks.
	fake.set(func(f *policyInspector) { f.status = 0; f.policy = `{"mode":"report","fail_closed":true}` })
	time.Sleep(150 * time.Millisecond)
	if s := run(); s != 0 {
		t.Fatalf("report mode blocked: %d", s)
	}
	fake.set(func(f *policyInspector) { f.policy = `{"mode":"block","fail_closed":"yes"}` })
	time.Sleep(150 * time.Millisecond)
	if s := run(); s != 0 {
		t.Fatalf("invalid refresh with report last-known-good blocked: %d", s)
	}
}

// A plugin publishes changes immediately, but the core applies them on the next
// request after cache expiry. Do not shorten the production TTL in this test.
func TestInspectPolicyEventuallyEnforcesRevision(t *testing.T) {
	m, fake := policyManager(t, `{"mode":"report"}`, denyVerdict)
	run := func() int {
		_, status := m.InspectRoute(newDeadlineRecorder(), httptest.NewRequest("GET", "http://example.test/", nil), inspectTarget("example.test"))
		return status
	}
	if status := run(); status != 0 {
		t.Fatalf("report blocked: %d", status)
	}
	fake.set(func(f *policyInspector) { f.policy = `{"mode":"block"}` })
	if status := run(); status != 0 {
		t.Fatalf("unexpired policy replaced early: %d", status)
	}
	deadline := time.Now().Add(3 * time.Second)
	for run() != http.StatusForbidden {
		if time.Now().After(deadline) {
			t.Fatal("block revision never enforced")
		}
		time.Sleep(25 * time.Millisecond)
	}
	fake.set(func(f *policyInspector) { f.policy = `{"mode":"off"}` })
	for run() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("off revision never enforced")
		}
		time.Sleep(25 * time.Millisecond)
	}
	_, before := fake.counts()
	r := httptest.NewRequest("POST", "http://example.test/", strings.NewReader("body"))
	original := r.Body
	w := newDeadlineRecorder()
	if handled, _ := m.InspectRoute(w, r, inspectTarget("example.test")); handled {
		t.Fatal("off blocked")
	}
	_, after := fake.counts()
	if before != after || r.Body != original || len(w.deadlines) != 0 {
		t.Fatal("propagated off mode still inspected the body")
	}
}

func TestInspectPolicyFreshInstallFailOpen(t *testing.T) {
	m, fake := policyManager(t, ``, denyVerdict)
	fake.status = 404
	m.LoadedPlugins["waf"].Spec.RequestInspectFailClosed = true
	republish(m)
	if handled, _ := m.InspectRoute(newDeadlineRecorder(), httptest.NewRequest("GET", "http://example.test/", nil), inspectTarget("example.test")); handled {
		t.Fatal("unavailable policy without last-known-good must fail open")
	}
}

func TestInspectPolicyModes(t *testing.T) {
	cases := []struct {
		name, policy, verdict, host, route string
		status, calls                      int
	}{
		{"off", `{"mode":"off","fail_closed":true}`, denyVerdict, "example.test", "example.test", 0, 0},
		{"report_deny", `{"mode":"report"}`, denyVerdict, "example.test", "example.test", 0, 1},
		{"report_error", `{"mode":"report","fail_closed":true}`, "", "example.test", "example.test", 0, 1},
		{"report_bad_verdict", `{"mode":"report","fail_closed":true}`, `{"action":"challenge"}`, "example.test", "example.test", 0, 1},
		{"block_deny", `{"mode":"block"}`, denyVerdict, "example.test", "example.test", 403, 1},
		{"block_allow", `{"mode":"block"}`, `{"action":"allow"}`, "example.test", "example.test", 0, 1},
		{"block_error_open", `{"mode":"block"}`, "", "example.test", "example.test", 0, 1},
		{"block_error_closed", `{"mode":"block","fail_closed":true}`, "", "example.test", "example.test", 503, 1},
		{"host_off", `{"mode":"block","hosts":{"example.test":{"mode":"off"}}}`, denyVerdict, "example.test", "other.route", 0, 0},
		{"host_canonical", `{"mode":"block","hosts":{"example.test":{"mode":"off"}}}`, denyVerdict, "EXAMPLE.test.:8443", "", 0, 0},
		{"host_report", `{"mode":"block","hosts":{"example.test":{"mode":"report"}}}`, denyVerdict, "example.test", "", 0, 1},
		{"host_block", `{"mode":"off","hosts":{"example.test":{"mode":"block"}}}`, denyVerdict, "example.test", "", 403, 1},
		{"host_inherit", `{"mode":"block","hosts":{"example.test":{"mode":"inherit"}}}`, denyVerdict, "example.test", "", 403, 1},
		{"route_precedence", `{"mode":"off","hosts":{"example.test":{"mode":"off"},"*.example.test":{"mode":"block"}}}`, denyVerdict, "example.test", "*.Example.test", 403, 1},
		{"other_host_default", `{"mode":"report","hosts":{"example.test":{"mode":"block"}}}`, denyVerdict, "other.test", "other.test", 0, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, fake := policyManager(t, tc.policy, tc.verdict)
			body := "payload"
			r := httptest.NewRequest("POST", "http://"+tc.host+"/", strings.NewReader(body))
			original := r.Body
			w := newDeadlineRecorder()
			handled, status := m.InspectRoute(w, r, inspectTarget(tc.route))
			if status != tc.status || handled != (tc.status != 0) {
				t.Fatalf("handled=%t status=%d, want %d", handled, status, tc.status)
			}
			if handled && w.Code != tc.status {
				t.Fatalf("written status %d", w.Code)
			}
			if _, calls := fake.counts(); calls != tc.calls {
				t.Fatalf("inspections=%d want %d", calls, tc.calls)
			}
			if tc.calls == 0 && (r.Body != original || len(w.deadlines) != 0) {
				t.Fatal("off mode acquired the request body")
			}
			if !handled {
				got, err := io.ReadAll(r.Body)
				if err != nil || string(got) != body {
					t.Fatalf("body replay %q %v", got, err)
				}
			}
		})
	}
}

func TestInspectFailureInversion(t *testing.T) {
	cases := []struct {
		name, policy, host, route string
		status                    int
	}{
		{"open_default", `{"mode":"block","invert_failure_hosts":["example.test"]}`, "other.test", "other.test", 0},
		{"open_inverted_host", `{"mode":"block","invert_failure_hosts":["example.test"]}`, "example.test", "x", 503},
		{"open_inverted_route", `{"mode":"block","invert_failure_hosts":["route-a"]}`, "example.test", "Route-A", 503},
		{"closed_default", `{"mode":"block","fail_closed":true,"invert_failure_hosts":["route-a"]}`, "example.test", "route-b", 503},
		{"closed_inverted", `{"mode":"block","fail_closed":true,"invert_failure_hosts":["route-a"]}`, "example.test", "route-a", 0},
		{"no_patterns", `{"mode":"block","invert_failure_hosts":["*.test"]}`, "example.test", "example.test", 0},
		{"report_never_blocks", `{"mode":"report","invert_failure_hosts":["example.test"]}`, "example.test", "example.test", 0},
		{"host_mode_override", `{"mode":"report","invert_failure_hosts":["example.test"],"hosts":{"example.test":{"mode":"block"}}}`, "example.test", "", 503},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := policyManager(t, tc.policy, "")
			w := newDeadlineRecorder()
			if _, status := m.InspectRoute(w, httptest.NewRequest("GET", "http://"+tc.host+"/", nil), inspectTarget(tc.route)); status != tc.status {
				t.Fatalf("status=%d want %d", status, tc.status)
			}
		})
	}
}

func TestInspectPolicyBodyLimits(t *testing.T) {
	cases := []struct {
		name, policy string
		size         int
		status, want int
		truncated    bool
	}{
		{"block_exact", `{"mode":"block","max_body_kb":1}`, 1024, 0, 1024, false},
		{"block_oversize", `{"mode":"block","max_body_kb":1}`, 1025, 413, -1, false},
		{"report_oversize", `{"mode":"report","max_body_kb":1}`, 1025, 0, 1024, true},
		{"default_64k", `{"mode":"report"}`, 65537, 0, 65536, true},
		{"host_override", `{"mode":"report","max_body_kb":1,"hosts":{"example.test":{"max_body_kb":2}}}`, 4096, 0, 2048, true},
		{"host_block_oversize", `{"mode":"report","hosts":{"example.test":{"mode":"block","max_body_kb":2}}}`, 2049, 413, -1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, fake := policyManager(t, tc.policy, `{"action":"allow"}`)
			body := strings.Repeat("x", tc.size)
			r := httptest.NewRequest("POST", "http://example.test/", strings.NewReader(body))
			w := newDeadlineRecorder()
			if _, status := m.InspectRoute(w, r, inspectTarget("example.test")); status != tc.status || (status != 0 && w.Code != status) {
				t.Fatalf("status=%d code=%d want %d", status, w.Code, tc.status)
			}
			_, calls := fake.counts()
			if tc.want < 0 {
				if calls != 0 {
					t.Fatal("oversize block still inspected")
				}
				return
			}
			if calls != 1 {
				t.Fatalf("inspections=%d", calls)
			}
			var payload zoraxy_plugin.RequestInspectPayload
			raw, _ := json.Marshal(fake.payloads[0])
			json.Unmarshal(raw, &payload)
			if len(payload.Body) != tc.want || payload.BodyTruncated != tc.truncated {
				t.Fatalf("captured %d truncated=%t", len(payload.Body), payload.BodyTruncated)
			}
			got, err := io.ReadAll(r.Body)
			if err != nil || string(got) != body {
				t.Fatalf("replay len=%d err=%v", len(got), err)
			}
		})
	}
}
