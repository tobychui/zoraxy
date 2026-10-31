package plugins

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"imuslab.com/zoraxy/mod/plugins/zoraxy_plugin"
)

// startSniffPlugin runs a fake plugin sniff endpoint on 127.0.0.1 and returns a
// Plugin that points at it, plus a counter of how many times the ingress was hit.
func startSniffPlugin(t *testing.T, sniff http.HandlerFunc) (*Plugin, *int, func()) {
	t.Helper()
	ingressHits := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/sniff/", sniff)
	mux.HandleFunc("/ingress/", func(w http.ResponseWriter, r *http.Request) {
		ingressHits++
		w.WriteHeader(http.StatusOK)
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)

	p := &Plugin{
		Enabled:      true,
		AssignedPort: ln.Addr().(*net.TCPAddr).Port,
		Spec: &zoraxy_plugin.IntroSpect{
			DynamicCaptureSniff:   "/sniff",
			DynamicCaptureIngress: "/ingress",
		},
	}
	p.StartDynamicForwardRouter()
	return p, &ingressHits, func() { srv.Close() }
}

func TestHandleDynamicRoute_DenyWithDefaultStatus(t *testing.T) {
	p, ingressHits, stop := startSniffPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(int(zoraxy_plugin.ControlStatusCode_DENY))
	})
	defer stop()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/admin?id=1%20OR%201=1", nil)
	if handled := p.HandleDynamicRoute(rec, req, "203.0.113.5"); !handled {
		t.Fatal("deny must count as handled so no other plugin or upstream sees the request")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 by default, got %d", rec.Code)
	}
	if *ingressHits != 0 {
		t.Fatalf("denied request must not reach the plugin ingress, got %d hits", *ingressHits)
	}
}

func TestHandleDynamicRoute_DenyWithPluginStatus(t *testing.T) {
	p, _, stop := startSniffPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Zoraxy-Deny-Status", strconv.Itoa(http.StatusTooManyRequests))
		w.WriteHeader(int(zoraxy_plugin.ControlStatusCode_DENY))
	})
	defer stop()

	rec := httptest.NewRecorder()
	p.HandleDynamicRoute(rec, httptest.NewRequest("GET", "http://example.com/", nil), "203.0.113.5")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected plugin-chosen 429, got %d", rec.Code)
	}
}

func TestHandleDynamicRoute_DenyIgnoresNonErrorStatus(t *testing.T) {
	p, _, stop := startSniffPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Zoraxy-Deny-Status", "200")
		w.WriteHeader(int(zoraxy_plugin.ControlStatusCode_DENY))
	})
	defer stop()

	rec := httptest.NewRecorder()
	p.HandleDynamicRoute(rec, httptest.NewRequest("GET", "http://example.com/", nil), "203.0.113.5")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a deny must never turn into a success status, got %d", rec.Code)
	}
}

func TestHandleDynamicRoute_SkipStillFallsThrough(t *testing.T) {
	p, ingressHits, stop := startSniffPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotImplemented)
	})
	defer stop()

	rec := httptest.NewRecorder()
	if handled := p.HandleDynamicRoute(rec, httptest.NewRequest("GET", "http://example.com/", nil), "203.0.113.5"); handled {
		t.Fatal("skip must not count as handled")
	}
	if *ingressHits != 0 {
		t.Fatal("skipped request must not reach the plugin ingress")
	}
}

func TestSniffHandler_DenyHelperSetsHeaders(t *testing.T) {
	mux := http.NewServeMux()
	pr := zoraxy_plugin.NewPathRouter()
	pr.RegisterDynamicSniffHandler("/sniff", mux, func(dsfr *zoraxy_plugin.DynamicSniffForwardRequest) zoraxy_plugin.SniffResult {
		return dsfr.Deny(http.StatusNotAcceptable, "test rule 1")
	})

	payload := zoraxy_plugin.EncodeForwardRequestPayload(httptest.NewRequest("GET", "http://example.com/x", nil), "203.0.113.5")
	body, _ := json.Marshal(payload)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/sniff/", bytes.NewReader(body)))

	if rec.Code != int(zoraxy_plugin.ControlStatusCode_DENY) {
		t.Fatalf("expected %d, got %d", zoraxy_plugin.ControlStatusCode_DENY, rec.Code)
	}
	if rec.Header().Get("X-Zoraxy-Deny-Status") != "406" || rec.Header().Get("X-Zoraxy-Deny-Reason") != "test rule 1" {
		t.Fatalf("unexpected deny headers: %v", rec.Header())
	}
}
