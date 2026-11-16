package streamproxy

import (
	"io"
	"net"
	"testing"
	"time"
)

// tcpPair returns the two ends of a loopback TCP connection.
func tcpPair(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		c, _ := ln.Accept()
		accepted <- c
	}()
	dialed, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return dialed, <-accepted
}

// forward must return (and release both connections) whichever side closes first.
// Before the fix, the copy goroutine that saw "use of closed network connection"
// returned without calling wg.Done, so forward blocked forever: one leaked
// goroutine per proxied connection.
func TestForwardReturnsAfterClose(t *testing.T) {
	for _, upstreamClosesFirst := range []bool{true, false} {
		client, proxyClientSide := tcpPair(t)
		proxyUpstreamSide, upstream := tcpPair(t)

		var aTob, bToa int64
		c := &ProxyRelayInstance{}
		done := make(chan struct{})
		go func() {
			c.forward(proxyUpstreamSide, proxyClientSide, &aTob, &bToa)
			close(done)
		}()

		client.Write([]byte("request"))
		buf := make([]byte, len("request"))
		if _, err := io.ReadFull(upstream, buf); err != nil {
			t.Fatal(err)
		}
		upstream.Write([]byte("response"))

		if upstreamClosesFirst {
			// e.g. a Zabbix server: answer, then close
			upstream.Close()
			if got, _ := io.ReadAll(client); string(got) != "response" {
				t.Fatalf("client got %q, want %q", got, "response")
			}
			client.Close()
		} else {
			buf = make([]byte, len("response"))
			if _, err := io.ReadFull(client, buf); err != nil {
				t.Fatal(err)
			}
			client.Close()
			io.ReadAll(upstream)
			upstream.Close()
		}

		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatalf("forward did not return (upstreamClosesFirst=%v): goroutine leak", upstreamClosesFirst)
		}
	}
}
