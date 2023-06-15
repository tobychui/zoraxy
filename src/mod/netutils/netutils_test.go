package netutils_test

import (
	"testing"

	"imuslab.com/zoraxy/mod/netutils"
)

func TestHandleTraceRoute(t *testing.T) {
	results, err := netutils.TraceRoute("imuslab.com", 64)
	if err != nil {
		t.Fatal(err)
	}

	t.Log(results)
}

func TestHandlePing(t *testing.T) {
	ipOrDomain := "example.com"

	realIP, pingTime, ttl, err := netutils.PingIP(ipOrDomain)
	if err != nil {
		t.Fatal("Error:", err)
		return
	}

	t.Log(realIP, pingTime, ttl)
}
