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
func TestMatchIpWildcard_IPv6(t *testing.T) {
	// IPv6 wildcards are not supported by MatchIpWildcard, so these should all return false
	tests := []struct {
		ip       string
		wildcard string
		want     bool
	}{
		{"fd7a:115c:a1e0::e101:6f0f", "fd7a:115c:a1e0::e101:6f0f", false}, // not supported
		{"fd7a:115c:a1e0::e101:6f0f", "*:*:*:*:*:*:*:*", false},
	}
	for _, tt := range tests {
		got := netutils.MatchIpWildcard(tt.ip, tt.wildcard)
		if got != tt.want {
			t.Errorf("MatchIpWildcard(%q, %q) = %v, want %v", tt.ip, tt.wildcard, got, tt.want)
		}
	}
}

func TestMatchIpCIDR_IPv6(t *testing.T) {
	tests := []struct {
		ip   string
		cidr string
		want bool
	}{
		{"fd7a:115c:a1e0::e101:6f0f", "fd7a:115c:a1e0::/48", true},
		{"fd7a:115c:a1e0::e101:6f0f", "fd7a:115c:a1e0::/64", true},
		{"fd7a:115c:a1e0::e101:6f0f", "fd7a:115c:a1e1::/48", false},
		{"fd7a:115c:a1e0::e101:6f0f", "fd7a:115c:a1e0::/128", false},
	}
	for _, tt := range tests {
		got := netutils.MatchIpCIDR(tt.ip, tt.cidr)
		if got != tt.want {
			t.Errorf("MatchIpCIDR(%q, %q) = %v, want %v", tt.ip, tt.cidr, got, tt.want)
		}
	}
}

func TestIsPrivateIP_IPv6(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"fd7a:115c:a1e0::e101:6f0f", true}, // Unique local address (fc00::/7)
		{"fe80::1", true},                   // Link-local
		{"2001:db8::1", false},              // Documentation address
	}
	for _, tt := range tests {
		got := netutils.IsPrivateIP(tt.ip)
		if got != tt.want {
			t.Errorf("IsPrivateIP(%q) = %v, want %v", tt.ip, got, tt.want)
		}
	}
}
