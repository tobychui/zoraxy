package dynamicproxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"imuslab.com/zoraxy/mod/dynamicproxy/errorpage"
)

// TestVirtualDirectoryEndpointHasSameTarget verifies the target-equality check used to decide
// whether a bulk apply/remove may treat an existing virtual directory as one it owns.
func TestVirtualDirectoryEndpointHasSameTarget(t *testing.T) {
	vdir := &VirtualDirectoryEndpoint{
		MatchingPath:        "/outpost.goauthentik.io",
		Domain:              "10.0.0.6:9000/outpost.goauthentik.io",
		RequireTLS:          false,
		SkipCertValidations: false,
	}

	if !vdir.HasSameTarget("10.0.0.6:9000/outpost.goauthentik.io", false, false) {
		t.Errorf("identical target should match")
	}
	if vdir.HasSameTarget("10.0.0.6:9000/other", false, false) {
		t.Errorf("different domain must not match")
	}
	if vdir.HasSameTarget("10.0.0.6:9000/outpost.goauthentik.io", true, false) {
		t.Errorf("different RequireTLS must not match")
	}
	if vdir.HasSameTarget("10.0.0.6:9000/outpost.goauthentik.io", false, true) {
		t.Errorf("different SkipCertValidations must not match")
	}
}

// TestClassifyBulkVdir verifies the per-host decision for both apply (ensure present) and remove
// (ensure absent), including that a differently-configured directory is always a conflict so it
// is never silently overwritten or deleted.
func TestClassifyBulkVdir(t *testing.T) {
	const domain = "10.0.0.6:9000/outpost.goauthentik.io"
	identical := &VirtualDirectoryEndpoint{MatchingPath: "/outpost.goauthentik.io", Domain: domain}
	customized := &VirtualDirectoryEndpoint{MatchingPath: "/outpost.goauthentik.io", Domain: "10.0.0.6:9000/something-else"}

	cases := []struct {
		name          string
		ensurePresent bool
		existing      *VirtualDirectoryEndpoint
		want          BulkVdirAction
	}{
		{"apply: missing -> create", true, nil, BulkVdirCreate},
		{"apply: identical -> skip", true, identical, BulkVdirSkip},
		{"apply: customized -> conflict", true, customized, BulkVdirConflict},
		{"remove: absent -> noop", false, nil, BulkVdirNoop},
		{"remove: identical -> remove", false, identical, BulkVdirRemove},
		{"remove: customized -> conflict", false, customized, BulkVdirConflict},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyBulkVdir(tc.ensurePresent, tc.existing, domain, false, false); got != tc.want {
				t.Errorf("ClassifyBulkVdir(ensurePresent=%v) = %q, want %q", tc.ensurePresent, got, tc.want)
			}
		})
	}
}

// TestVirtualDirectoryBlockAccess verifies a vdir in block access mode responds with the
// configured status code and status code template instead of proxying the request
func TestVirtualDirectoryBlockAccess(t *testing.T) {
	templateDir := t.TempDir()
	pages := errorpage.NewManager(&errorpage.Options{TemplateDir: templateDir})
	if err := pages.WriteTemplate("410.html", []byte("gone {{status_code}}"), false); err != nil {
		t.Fatal(err)
	}

	router := &Router{Option: &RouterOption{ErrorPageManager: pages}}
	endpoint := &ProxyEndpoint{
		DisableLogging:             true,
		DisableStatisticCollection: true,
		parent:                     router,
	}
	handler := &ProxyHandler{Parent: router}

	cases := []struct {
		statusCode   int
		expectedCode int
		expectedBody string
	}{
		{410, 410, "gone 410"},
		{0, 403, ""},   //Not set, default to 403 with built-in page
		{200, 403, ""}, //Not an error code, default to 403
	}
	for _, c := range cases {
		vdir := &VirtualDirectoryEndpoint{MatchingPath: "/blocked/", BlockAccess: true, BlockStatusCode: c.statusCode, parent: endpoint}
		req := httptest.NewRequest(http.MethodGet, "/blocked/secret", nil)
		rec := httptest.NewRecorder()
		handler.vdirRequest(rec, req, vdir)

		if rec.Code != c.expectedCode {
			t.Errorf("status code %d: expected response %d, got %d", c.statusCode, c.expectedCode, rec.Code)
		}
		if c.expectedBody != "" && rec.Body.String() != c.expectedBody {
			t.Errorf("status code %d: unexpected body %q", c.statusCode, rec.Body.String())
		}
	}

	//Blocked vdir must never be treated as the same target in bulk operations
	blocked := &VirtualDirectoryEndpoint{MatchingPath: "/x/", BlockAccess: true}
	if blocked.HasSameTarget("", false, false) {
		t.Errorf("blocked vdir must not match any proxy target")
	}
}
