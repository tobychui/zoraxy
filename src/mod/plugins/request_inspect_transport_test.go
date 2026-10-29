package plugins

import (
	"net/http"
	"testing"
	"time"
)

// pluginServerIdleTimeout mirrors the WAF plugin's http.Server IdleTimeout.
// The core must drop pooled inspection connections first: an inspection POST
// is not automatically replayed once written to a connection the server closed.
const pluginServerIdleTimeout = 30 * time.Second

func TestInspectClientIdleTimeoutBelowPluginServer(t *testing.T) {
	tr, ok := requestInspectClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("inspection client transport is %T, want *http.Transport", requestInspectClient.Transport)
	}
	if tr.IdleConnTimeout <= 0 || tr.IdleConnTimeout >= pluginServerIdleTimeout {
		t.Fatalf("IdleConnTimeout=%v, want 0 < t < %v", tr.IdleConnTimeout, pluginServerIdleTimeout)
	}
	if tr.MaxConnsPerHost != 0 {
		t.Fatalf("MaxConnsPerHost=%d, want 0 (no active cap; admission stays in the plugin)", tr.MaxConnsPerHost)
	}
	if tr.Proxy != nil {
		t.Fatal("inspection transport must not use environment proxies")
	}
}
