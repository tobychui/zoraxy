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
		want []HTTPMethod
	}{
		{"", []HTTPMethod{}},
		{"  ,  ", []HTTPMethod{}},
		{"GET", []HTTPMethod{MethodGet}},
		{"get, head,POST", []HTTPMethod{MethodGet, MethodHead, MethodPost}},
		{"POST GET\tGET\nPUT", []HTTPMethod{MethodGet, MethodPost, MethodPut}},
		{"patch, options, trace, connect, delete", []HTTPMethod{MethodConnect, MethodDelete, MethodOptions, MethodPatch, MethodTrace}},
		{"GET, propfind, MKCOL, report, version-control", []HTTPMethod{MethodGet, MethodMkcol, MethodPropfind, MethodReport, MethodVersionControl}},
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

	// Only the methods of the IANA registry are accepted
	for _, raw := range []string{"PURGE", "GET, M-SEARCH", "PRI", "*", "GET; DROP", "GE/T", "POST, <script>", strings.Repeat("A", 33)} {
		if _, err := ParseAllowedMethods(raw); err == nil {
			t.Errorf("%q: expected an error", raw)
		}
	}
}

func TestRegisteredHTTPMethods(t *testing.T) {
	// IANA HTTP Method Registry, https://www.iana.org/assignments/http-methods
	// without "PRI" and "*"
	registry := strings.Fields(`ACL BASELINE-CONTROL BIND CHECKIN CHECKOUT CONNECT COPY DELETE GET
		HEAD LABEL LINK LOCK MERGE MKACTIVITY MKCALENDAR MKCOL MKREDIRECTREF MKWORKSPACE MOVE
		OPTIONS ORDERPATCH PATCH POST PROPFIND PROPPATCH PUT QUERY REBIND REPORT SEARCH TRACE
		UNBIND UNCHECKOUT UNLINK UNLOCK UPDATE UPDATEREDIRECTREF VERSION-CONTROL`)
	if len(RegisteredHTTPMethods) != len(registry) {
		t.Errorf("got %d registered methods, want %d", len(RegisteredHTTPMethods), len(registry))
	}
	for _, method := range registry {
		if !HTTPMethod(method).IsRegistered() {
			t.Errorf("%s is not recognized as a registered method", method)
		}
	}
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace} {
		if !HTTPMethod(method).IsRegistered() {
			t.Errorf("net/http method %s is not recognized as a registered method", method)
		}
	}
	for _, method := range []HTTPMethod{"", "get", "PURGE", "PRI", "*"} {
		if method.IsRegistered() {
			t.Errorf("%q recognized as a registered method", method)
		}
	}
}

func TestIsMethodAllowed(t *testing.T) {
	all := &ProxyEndpoint{}
	for _, method := range []HTTPMethod{MethodGet, MethodDelete, "PROPFIND", "anything"} {
		if !all.IsMethodAllowed(method) {
			t.Errorf("endpoint without allow-list refused %s", method)
		}
	}

	limited := &ProxyEndpoint{AllowedMethods: []HTTPMethod{MethodGet, MethodHead, MethodPost}}
	tests := map[HTTPMethod]bool{
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
		AllowedMethods:       []HTTPMethod{MethodGet, MethodHead, MethodPost},
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
