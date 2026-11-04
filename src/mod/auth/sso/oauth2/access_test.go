package oauth2

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "reflect"
    "testing"
)

func TestSplitAllowedGroups(t *testing.T) {
    actual := SplitAllowedGroups(" admins , developers, ,admins , users  ")
    expected := []string{"admins", "developers", "users"}
    if !reflect.DeepEqual(actual, expected) {
        t.Fatalf("got %v, want %v", actual, expected)
    }
}

func TestGroupsAllowed(t *testing.T) {
    tests := []struct {
        user, allowed []string
        want          bool
    }{
        {[]string{"dev"}, nil, true},
        {[]string{"dev"}, []string{"dev", "admins"}, true},
        {[]string{"dev"}, []string{"admins"}, false},
        {nil, []string{"admins"}, false},
        {[]string{"Admins"}, []string{"admins"}, false},
    }
    for _, tc := range tests {
        if got := GroupsAllowed(tc.user, tc.allowed); got != tc.want {
            t.Errorf("GroupsAllowed(%v,%v)=%v, want %v", tc.user, tc.allowed, got, tc.want)
        }
    }
}

func TestFetchIdentity(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.Header.Get("Authorization") != "Bearer test-token" {
            t.Errorf("userinfo request lacks bearer token")
        }
        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte(`{"sub":"abc","preferred_username":"gabe","name":"Gabriel","display_name":"Gabe","picture":"https://example.com/avatar.png","groups":["admins","dev"]}`))
    }))
    defer srv.Close()
    client := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
        req.Header.Set("Authorization", "Bearer test-token")
        return http.DefaultTransport.RoundTrip(req)
    })}
    user, err := FetchIdentity(context.Background(), client, srv.URL)
    if err != nil {
        t.Fatal(err)
    }
    if user.Subject != "abc" || user.Picture != "https://example.com/avatar.png" || user.DisplayName != "Gabe" || !GroupsAllowed(user.Groups, []string{"dev"}) {
        t.Fatalf("unexpected identity %+v", user)
    }
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return fn(req) }

func TestFetchIdentityRejectsMissingSubject(t *testing.T) {
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte(`{"email":"someone@example.com"}`))
    }))
    defer srv.Close()
    _, err := FetchIdentity(context.Background(), srv.Client(), srv.URL)
    if err == nil {
        t.Fatal("expected rejection for missing subject")
    }
}

func TestIdentityHeadersAndCookieRemoval(t *testing.T) {
    req := httptest.NewRequest(http.MethodGet, "https://example.com/", nil)
    req.Header.Set("Remote-User", "fake-admin")
    req.Header.Set("X-Auth-Request-User", "fake-admin")
    req.Header.Set("Cookie", "z-token=secret; application_session=abc; z-verifier=secret2; z-oidc-nonce=challenge")
    StripIdentityHeaders(req.Header)
    if req.Header.Get("Remote-User") != "" || req.Header.Get("X-Auth-Request-User") != "" {
        t.Fatal("untrusted identity headers not removed")
    }
    SetIdentityHeaders(req.Header, &Identity{Subject:"abc", PreferredUsername:"gabe", Groups: []string{"dev"}})
    if req.Header.Get("Remote-User") != "gabe" || req.Header.Get("Remote-Groups") != "dev" {
        t.Fatal("verified identity headers not inserted")
    }
    StripOAuth2Cookies(req, true)
    if req.Header.Get("Cookie") != "application_session=abc" {
        t.Fatalf("wrong cookie filtering: %q", req.Header.Get("Cookie"))
    }
}

func TestFullUserInfoAndIdentityHeaders(t *testing.T) {
    payload := `{"sub":"user-123","preferred_username":"gabe","name":"Gabriel","email":"gabe@example.com","email_verified":true,"given_name":"Gabriel","groups":["admins","developers"],"custom_claim":{"role":"owner","quota":42}}`
    srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte(payload))
    }))
    defer srv.Close()
    identity, err := FetchIdentity(context.Background(), srv.Client(), srv.URL)
    if err != nil {
        t.Fatal(err)
    }
    headers := http.Header{}
    headers.Set("Remote-OIDC-UserInfo", `{"sub":"spoofed"}`)
    headers.Set("Remote-User", "attacker")
    StripIdentityHeaders(headers)
    if headers.Get("Remote-OIDC-UserInfo") != "" || headers.Get("Remote-User") != "" {
        t.Fatal("untrusted headers were not removed")
    }
    SetIdentityHeaders(headers, identity)
    if headers.Get("Remote-User") != "gabe" || headers.Get("Remote-Email") != "gabe@example.com" {
        t.Fatal("expected standard Remote-* identity headers")
    }
    var actual, expected map[string]json.RawMessage
    if err := json.Unmarshal([]byte(headers.Get("Remote-OIDC-UserInfo")), &actual); err != nil {
        t.Fatalf("invalid forwarded UserInfo: %v", err)
    }
    if err := json.Unmarshal([]byte(payload), &expected); err != nil {
        t.Fatal(err)
    }
    if !reflect.DeepEqual(actual, expected) {
        t.Fatalf("complete UserInfo was not forwarded: got %s", headers.Get("Remote-OIDC-UserInfo"))
    }
}
