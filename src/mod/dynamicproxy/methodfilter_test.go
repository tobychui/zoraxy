package dynamicproxy

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestParseAllowedMethods(t *testing.T) {
	tests := []struct {
		raw  string
		want []string
	}{
		{"", []string{}},
		{"  ,  ", []string{}},
		{"GET", []string{"GET"}},
		{"get, head,POST", []string{"GET", "HEAD", "POST"}},
		{"POST GET\tGET\nPUT", []string{"GET", "POST", "PUT"}},
		{"PROPFIND, M-SEARCH", []string{"M-SEARCH", "PROPFIND"}},
	}
	for _, tt := range tests {
		got, err := ParseAllowedMethods(tt.raw)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", tt.raw, err)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%q: got %v, want %v", tt.raw, got, tt.want)
		}
	}

	for _, raw := range []string{"GET; DROP", "GE/T", "POST, <script>", strings.Repeat("A", 33)} {
		if _, err := ParseAllowedMethods(raw); err == nil {
			t.Errorf("%q: expected an error", raw)
		}
	}
}

func TestIsMethodAllowed(t *testing.T) {
	all := &ProxyEndpoint{}
	for _, method := range []string{"GET", "DELETE", "PROPFIND", "anything"} {
		if !all.IsMethodAllowed(method) {
			t.Errorf("endpoint without allow-list refused %s", method)
		}
	}

	limited := &ProxyEndpoint{AllowedMethods: []string{"GET", "HEAD", "POST"}}
	tests := map[string]bool{
		"GET":     true,
		"HEAD":    true,
		"POST":    true,
		"PUT":     false,
		"OPTIONS": false,
		"TRACE":   false,
		"get":     false, //methods are case-sensitive
		"":        false,
	}
	for method, want := range tests {
		if got := limited.IsMethodAllowed(method); got != want {
			t.Errorf("%q: got %v, want %v", method, got, want)
		}
	}
}

func TestHandleMethodNotAllowed(t *testing.T) {
	router := newTestRateLimitRouter(t, false)
	h := &ProxyHandler{Parent: router}
	pe := &ProxyEndpoint{
		RootOrMatchingDomain: "host.example.com",
		AllowedMethods:       []string{"GET", "HEAD", "POST"},
		DisableLogging:       true,
	}

	r := httptest.NewRequest(http.MethodDelete, "https://host.example.com/item/1", nil)
	w := httptest.NewRecorder()
	h.handleMethodNotAllowed(w, r, pe)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("got status %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
	if got := w.Header().Get("Allow"); got != "GET, HEAD, POST" {
		t.Errorf("got Allow header %q, want %q", got, "GET, HEAD, POST")
	}
	if !strings.Contains(w.Body.String(), "405") {
		t.Errorf("error page does not show the status code")
	}
}
