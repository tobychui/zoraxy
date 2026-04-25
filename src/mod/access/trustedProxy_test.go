package access

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// newTestController creates a minimal Controller for testing trusted proxy operations.
func newTestController(t *testing.T) (*Controller, string) {
	t.Helper()
	tmpDir := t.TempDir()
	c := &Controller{
		ProxyAccessRule: &sync.Map{},
		Options: &Options{
			ConfigFolder:       tmpDir,
			TrustedProxiesFile: filepath.Join(tmpDir, "trusted_proxies.json"),
		},
	}
	return c, tmpDir
}

// --- IsTrustedProxy ---

func TestIsTrustedProxy_ExactIP(t *testing.T) {
	c, _ := newTestController(t)
	c.AddTrustedProxy("10.0.0.1", "test")

	if !c.IsTrustedProxy("10.0.0.1") {
		t.Error("expected 10.0.0.1 to be trusted")
	}
	if c.IsTrustedProxy("10.0.0.2") {
		t.Error("expected 10.0.0.2 to NOT be trusted")
	}
}

func TestIsTrustedProxy_CIDR(t *testing.T) {
	c, _ := newTestController(t)
	c.AddTrustedProxy("192.168.1.0/24", "local subnet")

	tests := []struct {
		ip   string
		want bool
	}{
		{"192.168.1.1", true},
		{"192.168.1.254", true},
		{"192.168.2.1", false},
		{"10.0.0.1", false},
	}
	for _, tt := range tests {
		if got := c.IsTrustedProxy(tt.ip); got != tt.want {
			t.Errorf("IsTrustedProxy(%q) = %v, want %v", tt.ip, got, tt.want)
		}
	}
}

func TestIsTrustedProxy_IPv6(t *testing.T) {
	c, _ := newTestController(t)
	c.AddTrustedProxy("::1", "loopback v6")
	c.AddTrustedProxy("2606:4700::/32", "cloudflare v6")

	tests := []struct {
		ip   string
		want bool
	}{
		{"::1", true},
		{"2606:4700::1", true},
		{"2606:4700:ffff:ffff:ffff:ffff:ffff:ffff", true},
		{"2607:4700::1", false},
	}
	for _, tt := range tests {
		if got := c.IsTrustedProxy(tt.ip); got != tt.want {
			t.Errorf("IsTrustedProxy(%q) = %v, want %v", tt.ip, got, tt.want)
		}
	}
}

func TestIsTrustedProxy_InvalidIP(t *testing.T) {
	c, _ := newTestController(t)
	c.AddTrustedProxy("10.0.0.1", "test")

	if c.IsTrustedProxy("not-an-ip") {
		t.Error("expected invalid IP to return false")
	}
	if c.IsTrustedProxy("") {
		t.Error("expected empty string to return false")
	}
}

func TestIsTrustedProxy_EmptyList(t *testing.T) {
	c, _ := newTestController(t)
	if c.IsTrustedProxy("10.0.0.1") {
		t.Error("expected false when no proxies configured")
	}
}

// --- AddTrustedProxy ---

func TestAddTrustedProxy(t *testing.T) {
	c, _ := newTestController(t)

	if !c.AddTrustedProxy("10.0.0.1", "first") {
		t.Error("expected add to succeed for new entry")
	}
	// Duplicate add should return false
	if c.AddTrustedProxy("10.0.0.1", "duplicate") {
		t.Error("expected add to fail for duplicate entry")
	}
}

func TestAddTrustedProxy_CIDR_RebuildCache(t *testing.T) {
	c, _ := newTestController(t)
	c.AddTrustedProxy("172.16.0.0/12", "private range")

	// The CIDR cache should be rebuilt, so IsTrustedProxy should work immediately
	if !c.IsTrustedProxy("172.20.5.5") {
		t.Error("expected 172.20.5.5 to match 172.16.0.0/12 after add")
	}
}

// --- RemoveTrustedProxy ---

func TestRemoveTrustedProxy(t *testing.T) {
	c, _ := newTestController(t)
	c.AddTrustedProxy("10.0.0.1", "test")

	if !c.RemoveTrustedProxy("10.0.0.1") {
		t.Error("expected remove to succeed for existing entry")
	}
	if c.IsTrustedProxy("10.0.0.1") {
		t.Error("expected 10.0.0.1 to no longer be trusted after removal")
	}
}

func TestRemoveTrustedProxy_NotExists(t *testing.T) {
	c, _ := newTestController(t)
	if c.RemoveTrustedProxy("10.0.0.1") {
		t.Error("expected remove to return false for non-existent entry")
	}
}

func TestRemoveTrustedProxy_CIDR_RebuildCache(t *testing.T) {
	c, _ := newTestController(t)
	c.AddTrustedProxy("192.168.0.0/16", "local")

	if !c.IsTrustedProxy("192.168.50.1") {
		t.Fatal("precondition: IP should match CIDR before removal")
	}

	c.RemoveTrustedProxy("192.168.0.0/16")

	if c.IsTrustedProxy("192.168.50.1") {
		t.Error("expected CIDR match to fail after removing the CIDR entry")
	}
}

// --- TrustedProxyExists ---

func TestTrustedProxyExists(t *testing.T) {
	c, _ := newTestController(t)
	c.AddTrustedProxy("10.0.0.1", "test")

	if !c.TrustedProxyExists("10.0.0.1") {
		t.Error("expected TrustedProxyExists to return true")
	}
	if c.TrustedProxyExists("10.0.0.2") {
		t.Error("expected TrustedProxyExists to return false for unknown IP")
	}
}

// --- ListTrustedProxies ---

func TestListTrustedProxies(t *testing.T) {
	c, _ := newTestController(t)
	c.AddTrustedProxy("10.0.0.1", "a")
	c.AddTrustedProxy("10.0.0.2", "b")
	c.AddTrustedProxy("192.168.0.0/16", "c")

	list := c.ListTrustedProxies()
	if len(list) != 3 {
		t.Errorf("expected 3 proxies, got %d", len(list))
	}

	// Verify all entries are present
	found := map[string]bool{}
	for _, p := range list {
		found[p.IP] = true
	}
	for _, ip := range []string{"10.0.0.1", "10.0.0.2", "192.168.0.0/16"} {
		if !found[ip] {
			t.Errorf("expected %s in list", ip)
		}
	}
}

func TestListTrustedProxies_Empty(t *testing.T) {
	c, _ := newTestController(t)
	list := c.ListTrustedProxies()
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d entries", len(list))
	}
}

// --- Save / Load ---

func TestSaveAndLoadTrustedProxies(t *testing.T) {
	c, _ := newTestController(t)
	c.AddTrustedProxy("10.0.0.1", "server1")
	c.AddTrustedProxy("172.16.0.0/12", "private range")

	if err := c.SaveTrustedProxies(); err != nil {
		t.Fatalf("SaveTrustedProxies failed: %v", err)
	}

	// Create a new controller loading from the same file
	c2 := &Controller{
		ProxyAccessRule: &sync.Map{},
		Options:         c.Options,
	}
	if err := c2.LoadTrustedProxies(); err != nil {
		t.Fatalf("LoadTrustedProxies failed: %v", err)
	}

	// Verify loaded data
	if !c2.IsTrustedProxy("10.0.0.1") {
		t.Error("expected 10.0.0.1 to be trusted after load")
	}
	if !c2.IsTrustedProxy("172.20.1.1") {
		t.Error("expected 172.20.1.1 to match 172.16.0.0/12 after load")
	}
	if c2.IsTrustedProxy("10.0.0.2") {
		t.Error("expected 10.0.0.2 to NOT be trusted after load")
	}
}

func TestLoadTrustedProxies_DefaultsWhenNoFile(t *testing.T) {
	c, _ := newTestController(t)

	if err := c.LoadTrustedProxies(); err != nil {
		t.Fatalf("LoadTrustedProxies failed: %v", err)
	}

	// Should have loaded defaults from embedded CSV
	list := c.ListTrustedProxies()
	if len(list) == 0 {
		t.Error("expected default proxies to be loaded when no config file exists")
	}

	// The config file should have been created
	if _, err := os.Stat(c.Options.TrustedProxiesFile); os.IsNotExist(err) {
		t.Error("expected config file to be created with defaults")
	}

	// Cloudflare 104.16.0.0/13 should be in defaults
	if !c.IsTrustedProxy("104.16.1.1") {
		t.Error("expected Cloudflare IP 104.16.1.1 to be trusted from defaults")
	}
}

func TestLoadTrustedProxies_InvalidJSON(t *testing.T) {
	c, _ := newTestController(t)
	os.WriteFile(c.Options.TrustedProxiesFile, []byte("not valid json"), 0644)

	if err := c.LoadTrustedProxies(); err == nil {
		t.Error("expected error when loading invalid JSON")
	}
}

// --- loadDefaultTrustedProxiesFromCSV ---

func TestLoadDefaultTrustedProxiesFromCSV(t *testing.T) {
	proxies := loadDefaultTrustedProxiesFromCSV()
	if len(proxies) == 0 {
		t.Fatal("expected at least one default proxy from embedded CSV")
	}
	for _, p := range proxies {
		if p.IP == "" {
			t.Error("proxy IP should not be empty")
		}
		if p.Desc != "Cloudflare" {
			t.Errorf("expected desc 'Cloudflare', got %q", p.Desc)
		}
	}
}

// --- GetClientIP ---

func TestGetClientIP_TrustedProxy_TrustsHeaders(t *testing.T) {
	c, _ := newTestController(t)
	c.AddTrustedProxy("10.0.0.1", "load balancer")

	rule := &AccessRule{
		TrustProxyHeadersOnly: true,
		parent:                c,
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")

	ip := rule.GetClientIP(req)
	if ip != "203.0.113.50" {
		t.Errorf("expected client IP from X-Forwarded-For (203.0.113.50), got %q", ip)
	}
}

func TestGetClientIP_UntrustedProxy_IgnoresHeaders(t *testing.T) {
	c, _ := newTestController(t)
	// 10.0.0.1 is NOT in the trusted proxy list

	rule := &AccessRule{
		TrustProxyHeadersOnly: true,
		parent:                c,
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")

	ip := rule.GetClientIP(req)
	if ip != "10.0.0.1" {
		t.Errorf("expected RemoteAddr IP (10.0.0.1) when proxy not trusted, got %q", ip)
	}
}

func TestGetClientIP_TrustDisabled_UsesRemoteAddr(t *testing.T) {
	c, _ := newTestController(t)
	c.AddTrustedProxy("10.0.0.1", "proxy")

	rule := &AccessRule{
		TrustProxyHeadersOnly: false,
		parent:                c,
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")

	ip := rule.GetClientIP(req)
	if ip != "10.0.0.1" {
		t.Errorf("expected RemoteAddr IP (10.0.0.1) when trust disabled, got %q", ip)
	}
}

func TestGetClientIP_NilParent_Fallback(t *testing.T) {
	rule := &AccessRule{
		TrustProxyHeadersOnly: true,
		parent:                nil,
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")

	ip := rule.GetClientIP(req)
	if ip != "10.0.0.1" {
		t.Errorf("expected RemoteAddr IP (10.0.0.1) when parent is nil, got %q", ip)
	}
}

func TestGetClientIP_TrustedCIDR_TrustsHeaders(t *testing.T) {
	c, _ := newTestController(t)
	c.AddTrustedProxy("10.0.0.0/8", "internal network")

	rule := &AccessRule{
		TrustProxyHeadersOnly: true,
		parent:                c,
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.50.25.3:9999"
	req.Header.Set("X-Real-Ip", "198.51.100.20")

	ip := rule.GetClientIP(req)
	if ip != "198.51.100.20" {
		t.Errorf("expected X-Real-Ip (198.51.100.20) when CIDR matches, got %q", ip)
	}
}

func TestGetClientIP_CFConnectingIP(t *testing.T) {
	c, _ := newTestController(t)
	c.AddTrustedProxy("104.16.0.0/13", "cloudflare")

	rule := &AccessRule{
		TrustProxyHeadersOnly: true,
		parent:                c,
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "104.16.5.1:443"
	req.Header.Set("CF-Connecting-IP", "1.2.3.4")

	ip := rule.GetClientIP(req)
	if ip != "1.2.3.4" {
		t.Errorf("expected CF-Connecting-IP (1.2.3.4), got %q", ip)
	}
}

// --- rebuildTrustedCIDRCache ---

func TestRebuildCIDRCache_MixedEntries(t *testing.T) {
	c, _ := newTestController(t)
	c.AddTrustedProxy("10.0.0.1", "exact ip")
	c.AddTrustedProxy("192.168.0.0/16", "cidr1")
	c.AddTrustedProxy("172.16.0.0/12", "cidr2")

	c.trustedCIDRMu.RLock()
	count := len(c.trustedCIDRs)
	c.trustedCIDRMu.RUnlock()

	if count != 2 {
		t.Errorf("expected 2 CIDR entries in cache, got %d", count)
	}
}

func TestRebuildCIDRCache_InvalidCIDR(t *testing.T) {
	c, _ := newTestController(t)
	// Manually store an invalid CIDR entry
	c.TrustedProxies.Store("bad/cidr", TrustedProxy{IP: "bad/cidr", Desc: "invalid"})
	c.rebuildTrustedCIDRCache()

	c.trustedCIDRMu.RLock()
	count := len(c.trustedCIDRs)
	c.trustedCIDRMu.RUnlock()

	if count != 0 {
		t.Errorf("expected 0 CIDR entries for invalid CIDR, got %d", count)
	}
}

// --- Save/Load round-trip with file content verification ---

func TestSaveTrustedProxies_FileContent(t *testing.T) {
	c, _ := newTestController(t)
	c.AddTrustedProxy("10.0.0.1", "server1")
	c.AddTrustedProxy("10.0.0.2", "server2")

	if err := c.SaveTrustedProxies(); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	data, err := os.ReadFile(c.Options.TrustedProxiesFile)
	if err != nil {
		t.Fatalf("read file failed: %v", err)
	}

	var proxies []TrustedProxy
	if err := json.Unmarshal(data, &proxies); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if len(proxies) != 2 {
		t.Errorf("expected 2 proxies in file, got %d", len(proxies))
	}
}
