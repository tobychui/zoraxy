package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"imuslab.com/zoraxy/mod/dynamicproxy"
	"imuslab.com/zoraxy/mod/dynamicproxy/loadbalance"
)

func TestH2CInvalidEditsPreserveWorkingRoutes(t *testing.T) {
	previousRouter := dynamicProxyRouter
	t.Cleanup(func() { dynamicProxyRouter = previousRouter })
	dynamicProxyRouter = &dynamicproxy.Router{ProxyEndpoints: &sync.Map{}}
	upstream := &loadbalance.Upstream{OriginIpOrDomain: "localhost:4317", UseH2C: true}
	vdir := &dynamicproxy.VirtualDirectoryEndpoint{MatchingPath: "/rpc", Domain: "localhost:4317", UseH2C: true}
	ep := &dynamicproxy.ProxyEndpoint{RootOrMatchingDomain: "otel.example.com", ActiveOrigins: []*loadbalance.Upstream{upstream}, VirtualDirectories: []*dynamicproxy.VirtualDirectoryEndpoint{vdir}}
	dynamicProxyRouter.ProxyEndpoints.Store(ep.RootOrMatchingDomain, ep)
	for _, tc := range []struct {
		name    string
		handler http.HandlerFunc
		form    url.Values
	}{
		{"upstream", ReverseProxyUpstreamUpdate, url.Values{"ep": {ep.RootOrMatchingDomain}, "origin": {upstream.OriginIpOrDomain}, "payload": {`{"RequireTLS":true}`}, "active": {"true"}}},
		{"virtual directory", ReverseProxyEditVdir, url.Values{"type": {"host"}, "path": {ep.RootOrMatchingDomain}, "vdir": {"/rpc"}, "domain": {vdir.Domain}, "reqTLS": {"true"}, "h2c": {"true"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/", strings.NewReader(tc.form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rec := httptest.NewRecorder()
			tc.handler(rec, req)
			var result map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil || result["error"] == nil {
				t.Fatalf("expected configuration error, got %s", rec.Body.String())
			}
			if len(ep.ActiveOrigins) != 1 || ep.ActiveOrigins[0] != upstream || upstream.RequireTLS || len(ep.VirtualDirectories) != 1 || ep.VirtualDirectories[0] != vdir || vdir.RequireTLS {
				t.Fatal("rejected edit changed the working route")
			}
		})
	}
}

func TestH2CUptimeTargets(t *testing.T) {
	router := &dynamicproxy.Router{ProxyEndpoints: &sync.Map{}}
	router.ProxyEndpoints.Store("otel.example.com", &dynamicproxy.ProxyEndpoint{
		ActiveOrigins:      []*loadbalance.Upstream{{OriginIpOrDomain: "localhost:8080", RequireTLS: true}},
		VirtualDirectories: []*dynamicproxy.VirtualDirectoryEndpoint{{MatchingPath: "/rpc", Domain: "localhost:4317", UseH2C: true}},
	})
	targets := GetUptimeTargetsFromReverseProxyRules(router)
	if len(targets) != 2 {
		t.Fatalf("got %d uptime targets", len(targets))
	}
	if targets[0].UseH2C || targets[0].URL != "https://localhost:8080" || !targets[1].UseH2C || targets[1].URL != "http://localhost:4317" {
		t.Fatalf("uptime protocols do not match routes: %+v %+v", targets[0], targets[1])
	}
}
