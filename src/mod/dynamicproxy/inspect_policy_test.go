package dynamicproxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"

	"imuslab.com/zoraxy/mod/dynamicproxy/loadbalance"
	"imuslab.com/zoraxy/mod/plugins"
	"imuslab.com/zoraxy/mod/plugins/zoraxy_plugin"
)

func routePolicyFixture(t *testing.T, policy, verdict string) (*Router, func() []zoraxy_plugin.RequestInspectPayload, *atomic.Int64) {
	t.Helper()
	router, _, _ := inspectRouter(t)
	var mu sync.Mutex
	var payloads []zoraxy_plugin.RequestInspectPayload
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/policy" {
			io.WriteString(w, policy)
			return
		}
		var payload zoraxy_plugin.RequestInspectPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		mu.Lock()
		payloads = append(payloads, payload)
		mu.Unlock()
		if verdict == "" {
			w.WriteHeader(500)
			return
		}
		io.WriteString(w, verdict)
	}))
	t.Cleanup(server.Close)
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	router.Option.PluginManager = &plugins.Manager{
		LoadedPlugins: map[string]*plugins.Plugin{"waf": {Enabled: true, AssignedPort: port, Spec: &zoraxy_plugin.IntroSpect{ID: "waf", RequestInspect: "/inspect", RequestInspectPolicy: "/policy"}}},
		Options:       &plugins.ManagerOptions{PluginGroups: map[string][]string{"waf": {"waf"}}},
	}
	router.Option.PluginManager.UpdateTagsToPluginMaps()
	var upstream atomic.Int64
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { upstream.Add(1); io.WriteString(w, "backend") }))
	t.Cleanup(backend.Close)
	origin := &loadbalance.Upstream{OriginIpOrDomain: backend.Listener.Addr().String(), Weight: 1}
	if err := origin.StartProxy(""); err != nil {
		t.Fatal(err)
	}
	router.Root.ActiveOrigins = []*loadbalance.Upstream{origin}
	router.Root.parent = router
	router.ProxyEndpoints.Range(func(_, value any) bool {
		ep := value.(*ProxyEndpoint)
		ep.ActiveOrigins = []*loadbalance.Upstream{origin}
		ep.parent = router
		return true
	})
	return router, func() []zoraxy_plugin.RequestInspectPayload {
		mu.Lock()
		defer mu.Unlock()
		return append([]zoraxy_plugin.RequestInspectPayload(nil), payloads...)
	}, &upstream
}

func TestInspectRootPolicyModes(t *testing.T) {
	for _, tc := range []struct {
		mode          string
		status, calls int
	}{{"block", 451, 1}, {"report", 200, 1}, {"off", 200, 0}} {
		t.Run(tc.mode, func(t *testing.T) {
			router, payloads, upstream := routePolicyFixture(t, `{"mode":"off","hosts":{"/":{"mode":"`+tc.mode+`"}}}`, `{"action":"deny","status":451}`)
			w := httptest.NewRecorder()
			router.mux.ServeHTTP(w, inspectHTTPRequest("GET", "http://unknown.test/root", nil))
			if w.Code != tc.status || len(payloads()) != tc.calls {
				t.Fatalf("status=%d payloads=%v", w.Code, payloads())
			}
			if tc.calls > 0 && payloads()[0].RouteID != "/" {
				t.Fatalf("root identity=%q", payloads()[0].RouteID)
			}
			if tc.status == 451 && upstream.Load() != 0 || tc.status == 200 && upstream.Load() != 1 {
				t.Fatalf("upstream calls=%d", upstream.Load())
			}
		})
	}
}

func TestInspectRootFailureInversion(t *testing.T) {
	for _, closed := range []bool{false, true} {
		t.Run(strconv.FormatBool(closed), func(t *testing.T) {
			policy := `{"mode":"block","fail_closed":` + strconv.FormatBool(closed) + `,"invert_failure_hosts":["/"]}`
			router, _, upstream := routePolicyFixture(t, policy, "")
			for _, tc := range []struct {
				host string
				want int
			}{{"unknown.test", 503}, {"app.test", 200}} {
				want := tc.want
				if closed {
					if want == 503 {
						want = 200
					} else {
						want = 503
					}
				}
				before := upstream.Load()
				w := httptest.NewRecorder()
				router.mux.ServeHTTP(w, inspectHTTPRequest("GET", "http://"+tc.host+"/", nil))
				if w.Code != want {
					t.Fatalf("host=%s status=%d want=%d", tc.host, w.Code, want)
				}
				if want == 503 && upstream.Load() != before || want == 200 && upstream.Load() != before+1 {
					t.Fatal("wrong backend forwarding")
				}
			}
		})
	}
}

func TestInspectWebSocketHandshakeBeforeUpgrade(t *testing.T) {
	for _, entry := range []string{"host", "root", "non-tls"} {
		t.Run(entry, func(t *testing.T) {
			router, payloads, upstream := routePolicyFixture(t, `{"mode":"block"}`, `{"action":"deny","status":451}`)
			host, route := "app.test", "app.test"
			if entry == "root" {
				host, route = "unknown.test", "/"
			}
			r := inspectHTTPRequest("GET", "http://"+host+"/socket?token=test", nil)
			r.Header.Set("Connection", "Upgrade")
			r.Header.Set("Upgrade", "websocket")
			r.Header.Set("Sec-WebSocket-Key", "test")
			w := httptest.NewRecorder()
			if entry == "non-tls" {
				ep, _ := router.ProxyEndpoints.Load(host)
				router.handleNonTLSRequest(w, r, ep.(*ProxyEndpoint))
			} else {
				router.mux.ServeHTTP(w, r)
			}
			got := payloads()
			if w.Code != 451 || upstream.Load() != 0 || len(got) != 1 {
				t.Fatalf("status=%d backend=%d payloads=%v", w.Code, upstream.Load(), got)
			}
			if got[0].RouteID != route || got[0].RequestURI != "/socket?token=test" || got[0].Header["Upgrade"][0] != "websocket" {
				t.Fatalf("handshake payload=%+v", got[0])
			}
		})
	}
}
