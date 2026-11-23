package zorxauth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// spoofedRequest returns a request carrying client supplied identity headers
func spoofedRequest() *http.Request {
	r := httptest.NewRequest(http.MethodGet, "http://app.example.com/", nil)
	r.Header.Set("X-Auth-Method", "spoofed")
	r.Header.Set("Remote-User", "admin")
	r.Header.Add("Remote-User-Id", "spoofed-1")
	r.Header.Add("Remote-User-Id", "spoofed-2")
	r.Header["Remote_user_id"] = []string{"spoofed-underscore"}
	r.Header.Set("Remote-Groups", "admins")
	r.Header.Set("Remote-Group-Id", "spoofed-group")
	r.Header.Set("Remote-Group-Name", "admins")
	r.Header.Set("Remote-Email", "admin@example.com")
	r.Header.Set("X-Unrelated", "keep")
	return r
}

func TestSetAuthHeadersReplacesClientIdentity(t *testing.T) {
	ar := &AuthRouter{Options: getDefaultOptions()}
	user := &User{ID: "user-uuid", Username: "alice"}
	r := spoofedRequest()

	ar.setAuthHeaders(r, user)

	want := map[string]string{
		"X-Auth-Method":  "zoraxy_sso",
		"Remote-User":    "alice",
		"Remote-Name":    "alice",
		"Remote-User-Id": "user-uuid",
		"X-Unrelated":    "keep",
	}
	for name, value := range want {
		if got := r.Header.Values(name); len(got) != 1 || got[0] != value {
			t.Errorf("%s = %q, want [%q]", name, got, value)
		}
	}
	// No email and no group: these must be absent rather than passed through
	for _, name := range []string{"Remote-Email", "Remote-Groups", "Remote-Group-Id", "Remote-Group-Name", "Remote_user_id"} {
		if got := r.Header.Values(name); len(got) != 0 {
			t.Errorf("%s should be absent, got %q", name, got)
		}
	}
}

func TestSetAuthHeadersForwardsAssignedGroup(t *testing.T) {
	ar := &AuthRouter{Options: getDefaultOptions()}
	ar.groupPolicies.Store("group-uuid", &GroupPolicy{ID: "group-uuid", Name: "Family, Friends"})
	user := &User{ID: "user-uuid", Username: "alice", Email: "alice@example.com", UseGroupPolicy: true, GroupID: "group-uuid"}
	r := spoofedRequest()

	ar.setAuthHeaders(r, user)

	want := map[string]string{
		"Remote-Email":      "alice@example.com",
		"Remote-Group-Id":   "group-uuid",
		"Remote-Group-Name": "Family, Friends",
		"Remote-Groups":     "Family  Friends", // a comma would read as two groups
	}
	for name, value := range want {
		if got := r.Header.Values(name); len(got) != 1 || got[0] != value {
			t.Errorf("%s = %q, want [%q]", name, got, value)
		}
	}
}

func TestSetAuthHeadersOmitsMissingOrUnusedGroup(t *testing.T) {
	ar := &AuthRouter{Options: getDefaultOptions()}
	ar.groupPolicies.Store("group-uuid", &GroupPolicy{ID: "group-uuid", Name: "Family"})
	users := map[string]*User{
		"deleted group":         {ID: "u1", Username: "alice", UseGroupPolicy: true, GroupID: "gone-uuid"},
		"group policy disabled": {ID: "u2", Username: "bob", UseGroupPolicy: false, GroupID: "group-uuid"},
	}
	for name, user := range users {
		r := spoofedRequest()
		ar.setAuthHeaders(r, user)
		for _, header := range []string{"Remote-Groups", "Remote-Group-Id", "Remote-Group-Name"} {
			if got := r.Header.Values(header); len(got) != 0 {
				t.Errorf("%s: %s should be absent, got %q", name, header, got)
			}
		}
	}
}

func TestSetAuthHeadersUsesCustomNamesAndStillStripsDefaults(t *testing.T) {
	opts := getDefaultOptions()
	opts.UserIDHeader = "X-Zoraxy-Uid"
	opts.GroupIDHeader = "X-Zoraxy-Gid"
	opts.GroupNameHeader = "X-Zoraxy-Group"
	ar := &AuthRouter{Options: opts}
	ar.groupPolicies.Store("group-uuid", &GroupPolicy{ID: "group-uuid", Name: "Family"})
	user := &User{ID: "user-uuid", Username: "alice", UseGroupPolicy: true, GroupID: "group-uuid"}

	r := spoofedRequest()
	r.Header.Set("X-Zoraxy-Uid", "spoofed")
	ar.setAuthHeaders(r, user)

	want := map[string]string{"X-Zoraxy-Uid": "user-uuid", "X-Zoraxy-Gid": "group-uuid", "X-Zoraxy-Group": "Family"}
	for name, value := range want {
		if got := r.Header.Values(name); len(got) != 1 || got[0] != value {
			t.Errorf("%s = %q, want [%q]", name, got, value)
		}
	}
	for _, name := range []string{"Remote-User-Id", "Remote-Group-Id", "Remote-Group-Name"} {
		if got := r.Header.Values(name); len(got) != 0 {
			t.Errorf("default %s should be stripped, got %q", name, got)
		}
	}
}

func TestStripIdentityHeadersKeepsOtherHeaders(t *testing.T) {
	ar := &AuthRouter{Options: getDefaultOptions()}
	r := spoofedRequest()
	r.Header["Remote_user"] = []string{"admin"}

	ar.StripIdentityHeaders(r)

	for name := range r.Header {
		if name != "X-Unrelated" {
			t.Errorf("header %s should have been stripped", name)
		}
	}
	if r.Header.Get("X-Unrelated") != "keep" {
		t.Errorf("unrelated header removed")
	}
}

func TestSetAuthHeadersDropsControlCharacters(t *testing.T) {
	ar := &AuthRouter{Options: getDefaultOptions()}
	ar.groupPolicies.Store("g", &GroupPolicy{ID: "g", Name: "Fam\r\nX-Injected: 1"})
	r := httptest.NewRequest(http.MethodGet, "http://app.example.com/", nil)

	ar.setAuthHeaders(r, &User{ID: "u", Username: "al\nice", UseGroupPolicy: true, GroupID: "g"})

	for name, values := range r.Header {
		for _, v := range values {
			if strings.ContainsAny(v, "\r\n") {
				t.Errorf("%s carries a control character: %q", name, v)
			}
		}
	}
}

func TestValidateIdentityHeaderNames(t *testing.T) {
	names, err := validateIdentityHeaderNames("", " ", "")
	if err != nil || names != (IdentityHeaderNames{DefaultUserIDHeader, DefaultGroupIDHeader, DefaultGroupNameHeader}) {
		t.Fatalf("empty names should select defaults, got %+v, %v", names, err)
	}

	names, err = validateIdentityHeaderNames("x-zoraxy-uid", "X-ZORAXY-GID", "x-zoraxy-group")
	if err != nil || names != (IdentityHeaderNames{"X-Zoraxy-Uid", "X-Zoraxy-Gid", "X-Zoraxy-Group"}) {
		t.Fatalf("custom names should be canonicalized, got %+v, %v", names, err)
	}

	invalid := map[string][3]string{
		"underscore":        {"Remote_User_Id", "", ""},
		"space":             {"Remote User", "", ""},
		"trailing dash":     {"Remote-", "", ""},
		"fixed header":      {"remote-user", "", ""},
		"reserved header":   {"", "cookie", ""},
		"hop-by-hop header": {"", "", "Connection"},
		"duplicate":         {"X-Id", "x-id", ""},
		"default collision": {"", "Remote-User-Id", ""},
		"too long":          {strings.Repeat("a", maxIdentityHeaderNameLength+1), "", ""},
	}
	invalid["quote in name"] = [3]string{`Bad"Name`, "", ""}
	for name, in := range invalid {
		got, err := validateIdentityHeaderNames(in[0], in[1], in[2])
		if err == nil {
			t.Errorf("%s: expected an error, got %+v", name, got)
			continue
		}
		// utils.SendErrorResponse does not escape, so messages must stay JSON safe
		if strings.ContainsAny(err.Error(), "\"\\") {
			t.Errorf("%s: error message is not JSON safe: %s", name, err.Error())
		}
	}
}
