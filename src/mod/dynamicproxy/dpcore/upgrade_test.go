package dpcore

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const testUpgradeProto = "tailscale-control-protocol"

// newUpgradeUpstream starts a TS2021 style upstream that replies 101 then echoes with a prefix
func newUpgradeUpstream(t *testing.T, respondUpType string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.EqualFold(r.Header.Get("Upgrade"), testUpgradeProto) {
			http.Error(w, "no upgrade header in TS2021 request", http.StatusInternalServerError)
			return
		}

		conn, brw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()

		brw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: " + respondUpType + "\r\n\r\n")
		brw.WriteString("SERVER-HELLO")
		brw.Flush()

		conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		buf := make([]byte, 64)
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		conn.Write([]byte("ECHO:" + string(buf[:n])))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// newUpgradeProxy starts a dpcore backed proxy in front of the given upstream
func newUpgradeProxy(t *testing.T, upstream string, rrr *ResponseRewriteRuleSet) *httptest.Server {
	t.Helper()
	target, err := url.Parse(upstream)
	if err != nil {
		t.Fatalf("failed to parse upstream url: %v", err)
	}
	core := NewDynamicProxyCore(target, "", &DpcoreOptions{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rules := *rrr
		rules.ProxyDomain = target.Host
		rules.OriginalHost = r.Host
		core.ServeHTTP(w, r, &rules)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// dialUpgrade performs an upgrade request over a raw connection and returns the status line
func dialUpgrade(t *testing.T, addr string, upType string) (net.Conn, *bufio.Reader, string) {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("failed to dial proxy: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	_, err = conn.Write([]byte("POST /ts2021 HTTP/1.1\r\nHost: hs.example.com\r\nConnection: Upgrade\r\nUpgrade: " + upType + "\r\nContent-Length: 0\r\n\r\n"))
	if err != nil {
		t.Fatalf("failed to write upgrade request: %v", err)
	}

	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	br := bufio.NewReader(conn)
	statusLine, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("failed to read status line: %v", err)
	}
	for {
		line, err := br.ReadString('\n')
		if err != nil || strings.TrimSpace(line) == "" {
			break
		}
	}
	return conn, br, strings.TrimSpace(statusLine)
}

// TestUpgradeTunnelsBidirectionally covers the TS2021 failure in issue #1290, where the
// upgrade headers were stripped and the upstream was closed right after the 101 response.
func TestUpgradeTunnelsBidirectionally(t *testing.T) {
	tests := []struct {
		name             string
		noRemoveHopByHop bool
	}{
		{"DefaultHopByHopRemoval", false},
		{"HopByHopRemovalDisabled", true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			upstream := newUpgradeUpstream(t, testUpgradeProto)
			proxy := newUpgradeProxy(t, upstream.URL, &ResponseRewriteRuleSet{AllowUpgrade: true, NoRemoveHopByHop: test.noRemoveHopByHop})

			conn, br, statusLine := dialUpgrade(t, proxy.Listener.Addr().String(), testUpgradeProto)
			if !strings.Contains(statusLine, "101 Switching Protocols") {
				t.Fatalf("expected 101 Switching Protocols, got %q", statusLine)
			}

			// The upstream payload sent after the 101 must reach the client
			payload := make([]byte, len("SERVER-HELLO"))
			if _, err := br.Read(payload); err != nil {
				t.Fatalf("failed to read post-upgrade payload: %v", err)
			}
			if string(payload) != "SERVER-HELLO" {
				t.Fatalf("expected post-upgrade payload SERVER-HELLO, got %q", payload)
			}

			// The client must still be able to write upstream after the upgrade
			if _, err := conn.Write([]byte("CLIENT-HELLO")); err != nil {
				t.Fatalf("failed to write after upgrade: %v", err)
			}

			conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			echo := make([]byte, len("ECHO:CLIENT-HELLO"))
			if _, err := br.Read(echo); err != nil {
				t.Fatalf("upstream did not echo client bytes, tunnel is not bidirectional: %v", err)
			}
			if string(echo) != "ECHO:CLIENT-HELLO" {
				t.Fatalf("expected ECHO:CLIENT-HELLO, got %q", echo)
			}
		})
	}
}

func TestUpgradeProtocolMismatchIsRejected(t *testing.T) {
	upstream := newUpgradeUpstream(t, "some-other-protocol")
	proxy := newUpgradeProxy(t, upstream.URL, &ResponseRewriteRuleSet{AllowUpgrade: true})

	_, _, statusLine := dialUpgrade(t, proxy.Listener.Addr().String(), testUpgradeProto)
	if strings.Contains(statusLine, "101") {
		t.Fatalf("expected the mismatched upgrade to be rejected, got %q", statusLine)
	}
}

func TestNonUpgradeRequestIsUnaffected(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "" {
			t.Errorf("unexpected upgrade header on a plain request: %q", r.Header.Get("Upgrade"))
		}
		w.Write([]byte("plain response"))
	}))
	defer upstream.Close()

	proxy := newUpgradeProxy(t, upstream.URL, &ResponseRewriteRuleSet{})
	res, err := http.Get(proxy.URL + "/")
	if err != nil {
		t.Fatalf("failed to send plain request: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for a plain request, got %d", res.StatusCode)
	}
}

func TestUpgradeTypeDetection(t *testing.T) {
	tests := []struct {
		name       string
		connection string
		upgrade    string
		expected   string
	}{
		{"NoUpgrade", "keep-alive", "", ""},
		{"UpgradeWithoutConnection", "", testUpgradeProto, ""},
		{"SimpleUpgrade", "Upgrade", testUpgradeProto, testUpgradeProto},
		{"MixedCaseConnection", "upgrade", "websocket", "websocket"},
		{"TokenListConnection", "keep-alive, Upgrade", testUpgradeProto, testUpgradeProto},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			header := http.Header{}
			if test.connection != "" {
				header.Set("Connection", test.connection)
			}
			if test.upgrade != "" {
				header.Set("Upgrade", test.upgrade)
			}
			if got := upgradeType(header); got != test.expected {
				t.Fatalf("expected upgrade type %q, got %q", test.expected, got)
			}
		})
	}
}

func TestUpgradeIsBlockedByDefault(t *testing.T) {
	upstream := newUpgradeUpstream(t, testUpgradeProto)
	proxy := newUpgradeProxy(t, upstream.URL, &ResponseRewriteRuleSet{})

	_, _, statusLine := dialUpgrade(t, proxy.Listener.Addr().String(), testUpgradeProto)
	if !strings.Contains(statusLine, "403 Forbidden") {
		t.Fatalf("expected 403 Forbidden when upgrade forwarding is not enabled, got %q", statusLine)
	}
}

func TestUpgradeGateAllowsPlainRequest(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("plain response"))
	}))
	defer upstream.Close()

	proxy := newUpgradeProxy(t, upstream.URL, &ResponseRewriteRuleSet{})
	res, err := http.Get(proxy.URL + "/")
	if err != nil {
		t.Fatalf("failed to send plain request: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected a plain request to be unaffected by the upgrade gate, got %d", res.StatusCode)
	}
}
