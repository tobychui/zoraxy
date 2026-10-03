package dynamicproxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
	"imuslab.com/zoraxy/mod/dynamicproxy/loadbalance"
)

func TestH2CRoutesSurviveConfigReload(t *testing.T) {
	backend := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Protocol", r.Proto)
		w.Header().Set("X-Path", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	backend.Config.Protocols = new(http.Protocols)
	backend.Config.Protocols.SetUnencryptedHTTP2(true)
	backend.Start()
	defer backend.Close()
	origin := strings.TrimPrefix(backend.URL, "http://")
	ep := &ProxyEndpoint{
		ActiveOrigins:      []*loadbalance.Upstream{{OriginIpOrDomain: origin, UseH2C: true}},
		VirtualDirectories: []*VirtualDirectoryEndpoint{{MatchingPath: "/rpc", Domain: origin, UseH2C: true}},
	}
	data, err := json.Marshal(ep)
	if err != nil {
		t.Fatal(err)
	}
	var restored ProxyEndpoint
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	router := &Router{}
	if _, err := router.PrepareProxyRoute(&restored); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		serve func(http.ResponseWriter, *http.Request, *dpcore.ResponseRewriteRuleSet) (int, error)
	}{
		{"upstream", restored.ActiveOrigins[0].ServeHTTP},
		{"virtual directory", restored.VirtualDirectories[0].proxy.ServeHTTP},
	} {
		t.Run(tc.name, func(t *testing.T) {
			front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				code, err := tc.serve(w, r, &dpcore.ResponseRewriteRuleSet{OriginalHost: r.Host})
				if err != nil {
					http.Error(w, err.Error(), code)
				}
			}))
			defer front.Close()
			client := &http.Client{Timeout: 5 * time.Second}
			res, err := client.Post(front.URL+"/service/Export", "application/grpc", strings.NewReader("message"))
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusOK || res.Header.Get("X-Protocol") != "HTTP/2.0" || res.Header.Get("X-Path") != "/service/Export" {
				t.Fatalf("restored route failed: status=%d, headers=%v", res.StatusCode, res.Header)
			}
		})
	}
}

func TestH2CConflictingRouteSettings(t *testing.T) {
	for _, inactive := range []bool{false, true} {
		ep := &ProxyEndpoint{ForceHTTP11: true}
		origin := &loadbalance.Upstream{OriginIpOrDomain: "127.0.0.1:4317", UseH2C: true}
		if inactive {
			ep.InactiveOrigins = []*loadbalance.Upstream{origin}
		} else {
			ep.ActiveOrigins = []*loadbalance.Upstream{origin}
		}
		if _, err := (&Router{}).PrepareProxyRoute(ep); err == nil {
			t.Fatalf("Force HTTP/1.1 accepted for h2c upstream (inactive=%v)", inactive)
		}
	}
	ep := &ProxyEndpoint{VirtualDirectories: []*VirtualDirectoryEndpoint{{Domain: "localhost:4317", UseH2C: true, RequireTLS: true}}}
	if _, err := (&Router{}).PrepareProxyRoute(ep); err == nil {
		t.Fatal("TLS accepted for an h2c virtual directory")
	}
	custom := &VirtualDirectoryEndpoint{Domain: "localhost:4317", UseH2C: true}
	if got := ClassifyBulkVdir(false, custom, custom.Domain, false, false); got != BulkVdirConflict {
		t.Fatalf("bulk remove must preserve an h2c directory, got %q", got)
	}
}

func TestH2CRejectsWebSocketUpgrades(t *testing.T) {
	for _, virtualDirectory := range []bool{false, true} {
		req := httptest.NewRequest("GET", "http://otel.example.com/rpc", nil)
		req.Header.Set("Connection", "Upgrade")
		req.Header.Set("Upgrade", "websocket")
		rec := httptest.NewRecorder()
		handler := &ProxyHandler{}
		if virtualDirectory {
			handler.vdirRequest(rec, req, &VirtualDirectoryEndpoint{UseH2C: true})
		} else {
			handler.hostWebSocketRequest(rec, req, &ProxyEndpoint{}, &loadbalance.Upstream{UseH2C: true})
		}
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("WebSocket upgrade was not rejected (virtual directory=%v): %d", virtualDirectory, rec.Code)
		}
	}
}
