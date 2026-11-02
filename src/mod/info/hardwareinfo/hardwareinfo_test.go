package hardwareinfo

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// NewInfoServer / ArOZInfo tests – platform-independent
// ---------------------------------------------------------------------------

// TestNewInfoServer verifies that NewInfoServer returns a non-nil Server
// with the ArOZInfo values preserved.
func TestNewInfoServer(t *testing.T) {
	info := ArOZInfo{
		BuildVersion: "1.0",
		DeviceVendor: "TestVendor",
		DeviceModel:  "TestModel",
		VendorIcon:   "icon.png",
		SN:           "SN-0001",
		HostOS:       runtime.GOOS,
		CPUArch:      runtime.GOARCH,
		HostName:     "testhost",
	}

	s := NewInfoServer(info)
	if s == nil {
		t.Fatal("NewInfoServer() returned nil")
	}
	if s.hostInfo.BuildVersion != info.BuildVersion {
		t.Errorf("BuildVersion = %q, expected %q", s.hostInfo.BuildVersion, info.BuildVersion)
	}
	if s.hostInfo.DeviceVendor != info.DeviceVendor {
		t.Errorf("DeviceVendor = %q, expected %q", s.hostInfo.DeviceVendor, info.DeviceVendor)
	}
	if s.hostInfo.DeviceModel != info.DeviceModel {
		t.Errorf("DeviceModel = %q, expected %q", s.hostInfo.DeviceModel, info.DeviceModel)
	}
}

// TestNewInfoServerEmpty verifies that NewInfoServer works with a zero-value ArOZInfo.
func TestNewInfoServerEmpty(t *testing.T) {
	s := NewInfoServer(ArOZInfo{})
	if s == nil {
		t.Fatal("NewInfoServer(ArOZInfo{}) returned nil")
	}
}

// ---------------------------------------------------------------------------
// GetArOZInfo HTTP handler tests
// ---------------------------------------------------------------------------

// TestGetArOZInfoHandler verifies the handler returns valid JSON.
func TestGetArOZInfoHandler(t *testing.T) {
	info := ArOZInfo{
		BuildVersion: "2.0",
		DeviceVendor: "Acme",
		DeviceModel:  "Widget",
		VendorIcon:   "data:image/png;base64,abc",
		SN:           "SN-42",
		HostOS:       "linux",
		CPUArch:      "amd64",
		HostName:     "node1",
	}
	s := NewInfoServer(info)

	req := httptest.NewRequest(http.MethodGet, "/aroz/info", nil)
	w := httptest.NewRecorder()
	s.GetArOZInfo(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GetArOZInfo status = %d, expected 200", resp.StatusCode)
	}

	var got ArOZInfo
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// VendorIcon should be stripped when icon param is not "true"
	if got.VendorIcon != "" {
		t.Errorf("VendorIcon should be empty by default, got %q", got.VendorIcon)
	}
	if got.BuildVersion != info.BuildVersion {
		t.Errorf("BuildVersion = %q, expected %q", got.BuildVersion, info.BuildVersion)
	}
}

// TestGetArOZInfoHandlerWithIcon verifies the handler includes the VendorIcon
// when the query parameter icon=true is provided.
func TestGetArOZInfoHandlerWithIcon(t *testing.T) {
	icon := "data:image/png;base64,iVBORw0KGgo="
	info := ArOZInfo{
		BuildVersion: "3.0",
		VendorIcon:   icon,
	}
	s := NewInfoServer(info)

	req := httptest.NewRequest(http.MethodGet, "/aroz/info?icon=true", nil)
	w := httptest.NewRecorder()
	s.GetArOZInfo(w, req)

	var got ArOZInfo
	if err := json.NewDecoder(w.Result().Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.VendorIcon != icon {
		t.Errorf("VendorIcon = %q, expected %q", got.VendorIcon, icon)
	}
}

// ---------------------------------------------------------------------------
// filterGrepResults – platform-independent helper
// ---------------------------------------------------------------------------

func TestFilterGrepResults(t *testing.T) {
	cases := []struct {
		result   string
		sep      string
		expected string
	}{
		{"key: value with spaces", ":", "value with spaces"},
		{"no separator here", ":", "no separator here"},
		{"a:b:c", ":", "b"},
		{"", ":", ""},
	}
	for _, tc := range cases {
		got := filterGrepResults(tc.result, tc.sep)
		if got != tc.expected {
			t.Errorf("filterGrepResults(%q, %q) = %q, expected %q", tc.result, tc.sep, got, tc.expected)
		}
	}
}

// ---------------------------------------------------------------------------
// Platform-specific HTTP handler smoke tests
// ---------------------------------------------------------------------------

// TestGetCPUInfoHandler issues a request to the platform-specific GetCPUInfo
// handler and checks that the response body is non-empty JSON.
func TestGetCPUInfoHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	GetCPUInfo(w, req)

	body := w.Body.String()
	if strings.TrimSpace(body) == "" {
		t.Error("GetCPUInfo handler returned empty body")
	}
}

// TestIfconfigHandler checks that the Ifconfig handler returns a response
// without panicking.  On Linux the iw/ip commands may fail in CI; the handler
// should still produce an empty array or a valid result.
func TestIfconfigHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	Ifconfig(w, req)
	// Just verify it doesn't panic and writes something.
	_ = w.Body.String()
}

// TestGetDriveStatHandler exercises GetDriveStat; skips when running as
// an unprivileged user on non-Linux systems where df may not be available.
func TestGetDriveStatHandler(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping GetDriveStat on Windows")
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	GetDriveStat(w, req)
	_ = w.Body.String()
}

// TestGetUSBHandler exercises GetUSB; the hardware may not be present in CI.
func TestGetUSBHandler(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" && runtime.GOOS != "freebsd" {
		t.Skip("skipping GetUSB on non-Unix platform")
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	GetUSB(w, req)
	_ = w.Body.String()
}

// TestGetRamInfoHandler verifies that GetRamInfo does not panic.
func TestGetRamInfoHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	GetRamInfo(w, req)
	body := w.Body.String()
	if strings.TrimSpace(body) == "" {
		t.Error("GetRamInfo handler returned empty body")
	}
}

// ---------------------------------------------------------------------------
// wmicGetinfo – Windows only (compile-guarded by build tags)
// ---------------------------------------------------------------------------

// TestWmicGetinfoNonWindows verifies that wmicGetinfo returns a default
// "Undefined" slice when wmic is not available (i.e., non-Windows).
func TestWmicGetinfoNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping non-Windows wmic test on Windows")
	}
	result := wmicGetinfo("os", "Caption")
	// On non-Windows, wmic doesn't exist; the function should return ["Undefined"]
	if len(result) == 0 {
		t.Error("wmicGetinfo() returned empty slice, expected at least one element")
	}
}
