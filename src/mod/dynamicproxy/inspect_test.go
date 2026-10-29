package dynamicproxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"imuslab.com/zoraxy/mod/access"
	"imuslab.com/zoraxy/mod/dynamicproxy/loadbalance"
	"imuslab.com/zoraxy/mod/dynamicproxy/redirection"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/plugins"
	"imuslab.com/zoraxy/mod/plugins/zoraxy_plugin"
)

// inspectRouter returns a router whose plugin manager has a denying
// inspector on tag "waf", and the route IDs that inspector received.
func inspectRouter(t *testing.T) (*Router, func() []string, string) {
	t.Helper()
	var mu sync.Mutex
	var routes []string
	inspector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload zoraxy_plugin.RequestInspectPayload
		json.NewDecoder(r.Body).Decode(&payload)
		mu.Lock()
		routes = append(routes, payload.RouteID)
		mu.Unlock()
		io.WriteString(w, `{"action":"deny","status":451,"reason":"inspected"}`)
	}))
	t.Cleanup(inspector.Close)
	u, _ := url.Parse(inspector.URL)
	port, _ := strconv.Atoi(u.Port())
	pm := &plugins.Manager{
		LoadedPlugins: map[string]*plugins.Plugin{"waf": {
			Enabled:      true,
			AssignedPort: port,
			Spec:         &zoraxy_plugin.IntroSpect{ID: "waf", RequestInspect: "/inspect"},
		}},
		Options: &plugins.ManagerOptions{PluginGroups: map[string][]string{"waf": {"waf"}}},
	}
	pm.UpdateTagsToPluginMaps()

	logDir := t.TempDir()
	log, err := logger.NewLogger("zr", logDir)
	if err != nil {
		t.Fatal(err)
	}
	openRule := func() *access.AccessRule {
		return &access.AccessRule{
			ID:                   "default",
			WhiteListCountryCode: &map[string]string{},
			WhiteListIP:          &map[string]string{},
			BlackListContryCode:  &map[string]string{},
			BlackListIP:          &map[string]string{},
		}
	}
	controller := &access.Controller{DefaultAccessRule: openRule(), ProxyAccessRule: &sync.Map{}}
	//A missing access rule is rejected before inspection.

	router := &Router{
		Option: &RouterOption{
			PluginManager:     pm,
			AccessController:  controller,
			Logger:            log,
			RedirectRuleTable: &redirection.RuleTable{},
		},
		ProxyEndpoints: &sync.Map{},
		Root: &ProxyEndpoint{
			RootOrMatchingDomain:   "/",
			ProxyType:              ProxyTypeRoot,
			AuthenticationProvider: GetDefaultAuthenticationProvider(),
			DefaultSiteOption:      DefaultSite_ReverseProxy,
			Tags:                   []string{"waf"},
		},
	}
	router.loadBalancer = &loadbalance.RouteManager{Options: loadbalance.Options{Logger: log}}
	router.mux = &ProxyHandler{Parent: router}
	router.ProxyEndpoints.Store("app.test", &ProxyEndpoint{RootOrMatchingDomain: "app.test", ProxyType: ProxyTypeHost, Tags: []string{"waf"}, AuthenticationProvider: GetDefaultAuthenticationProvider()})
	router.ProxyEndpoints.Store("blocked.test", &ProxyEndpoint{RootOrMatchingDomain: "blocked.test", ProxyType: ProxyTypeHost, Tags: []string{"waf"}, AccessFilterUUID: "blocked", AuthenticationProvider: GetDefaultAuthenticationProvider()})
	got := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string{}, routes...)
	}
	return router, got, logDir
}

// waitForLog waits for the asynchronous access log to contain all parts in one line.
func waitForLog(t *testing.T, dir string, parts ...string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		files, _ := filepath.Glob(filepath.Join(dir, "*"))
		for _, f := range files {
			data, _ := os.ReadFile(f)
			for _, line := range strings.Split(string(data), "\n") {
				found := true
				for _, part := range parts {
					found = found && strings.Contains(line, part)
				}
				if found {
					return
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("access log has no line with %q", parts)
}

func TestInspectHostRouteDeniedAndLogged(t *testing.T) {
	router, routes, logDir := inspectRouter(t)
	w := httptest.NewRecorder()
	(&ProxyHandler{Parent: router}).ServeHTTP(w, inspectHTTPRequest("GET", "http://app.test/x", nil))
	if w.Code != 451 || got(routes) != "app.test" {
		t.Fatalf("status=%d routes=%v", w.Code, routes())
	}
	waitForLog(t, logDir, "[router:inspect-blocked]", "GET /x 451")
}

func TestInspectRootDefaultSiteDeniedAndLogged(t *testing.T) {
	router, routes, logDir := inspectRouter(t)
	w := httptest.NewRecorder()
	(&ProxyHandler{Parent: router}).ServeHTTP(w, inspectHTTPRequest("GET", "http://unknown.test/root", nil))
	if w.Code != 451 || got(routes) != "/" {
		t.Fatalf("status=%d routes=%v", w.Code, routes())
	}
	waitForLog(t, logDir, "[router:inspect-blocked]", "GET /root 451")
}

// Default sites that never reach a backend are not inspected.
func TestInspectRootNonForwardingSkipped(t *testing.T) {
	router, routes, _ := inspectRouter(t)
	router.Root.DefaultSiteOption = DefaultSite_TeaPot
	w := httptest.NewRecorder()
	(&ProxyHandler{Parent: router}).ServeHTTP(w, inspectHTTPRequest("GET", "http://unknown.test/", nil))
	if w.Code != http.StatusTeapot || len(routes()) != 0 {
		t.Fatalf("status=%d routes=%v", w.Code, routes())
	}
}

// Inspection runs after access checks: an invalid access rule never reaches it.
func TestInspectAfterAccessChecks(t *testing.T) {
	router, routes, _ := inspectRouter(t)
	r := inspectHTTPRequest("GET", "http://blocked.test/", nil)
	r.RemoteAddr = "192.0.2.1:1234"
	w := httptest.NewRecorder()
	(&ProxyHandler{Parent: router}).ServeHTTP(w, r)
	if w.Code != http.StatusInternalServerError || len(routes()) != 0 {
		t.Fatalf("status=%d routes=%v", w.Code, routes())
	}
}

// Port 80 and secondary listeners route plain HTTP through handleNonTLSRequest.
func TestInspectNonTLSListener(t *testing.T) {
	router, routes, logDir := inspectRouter(t)
	ep, _ := router.ProxyEndpoints.Load("app.test")
	w := httptest.NewRecorder()
	router.handleNonTLSRequest(w, inspectHTTPRequest("GET", "http://app.test/plain", nil), ep.(*ProxyEndpoint))
	if w.Code != 451 || got(routes) != "app.test" {
		t.Fatalf("status=%d routes=%v", w.Code, routes())
	}
	waitForLog(t, logDir, "[router:inspect-blocked]", "GET /plain 451")
}

func inspectHTTPRequest(method, target string, body io.Reader) *http.Request {
	r := httptest.NewRequest(method, target, body)
	r.RequestURI = r.URL.RequestURI()
	return r
}

func got(routes func() []string) string { return strings.Join(routes(), ",") }
