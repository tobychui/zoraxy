package dpcore

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
)

// newOneShotUpstream mimics a server that advertises keep-alive but drops the
// connection without a response when a second request arrives on it
func newOneShotUpstream(t *testing.T) (addr string, connCount *atomic.Int32) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	connCount = &atomic.Int32{}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			connCount.Add(1)
			go serveOneRequest(conn)
		}
	}()
	return ln.Addr().String(), connCount
}

func serveOneRequest(conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	if _, err := http.ReadRequest(reader); err != nil {
		return
	}
	conn.Write([]byte("HTTP/1.1 200 OK\r\nConnection: Keep-Alive\r\nKeep-Alive: timeout=15, max=95\r\nContent-Length: 2\r\n\r\nok"))
	http.ReadRequest(reader)
}

// newHTTP2Proxy starts a dpcore backed proxy that accepts HTTP/2 from clients
func newHTTP2Proxy(t *testing.T, upstreamAddr string, opts *DpcoreOptions) *httptest.Server {
	t.Helper()
	target := &url.URL{Scheme: "http", Host: upstreamAddr}
	core := NewDynamicProxyCore(target, "", opts)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		statusCode, err := core.ServeHTTP(w, r, &ResponseRewriteRuleSet{
			ProxyDomain:  target.Host,
			OriginalHost: r.Host,
		})
		if err != nil {
			w.WriteHeader(statusCode)
		}
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

func getSequentially(t *testing.T, proxy *httptest.Server, count int) {
	t.Helper()
	client := proxy.Client()
	for i := 0; i < count; i++ {
		resp, err := client.Get(proxy.URL + "/asset.js")
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.ProtoMajor != 2 {
			t.Fatalf("expected HTTP/2 between client and proxy, got %s", resp.Proto)
		}
		if resp.StatusCode != http.StatusOK || string(body) != "ok" {
			t.Fatalf("request %d got status %d body %q", i, resp.StatusCode, body)
		}
	}
}

func TestHTTP2BodylessRequestRetriedOnClosedReusedConnection(t *testing.T) {
	upstreamAddr, _ := newOneShotUpstream(t)
	proxy := newHTTP2Proxy(t, upstreamAddr, &DpcoreOptions{})
	getSequentially(t, proxy, 5)
}

func TestDisableKeepAliveOpensConnectionPerRequest(t *testing.T) {
	upstreamAddr, connCount := newOneShotUpstream(t)
	proxy := newHTTP2Proxy(t, upstreamAddr, &DpcoreOptions{DisableKeepAlive: true})

	const requests = 5
	getSequentially(t, proxy, requests)
	if got := connCount.Load(); got != requests {
		t.Fatalf("expected %d upstream connections, got %d", requests, got)
	}
}
