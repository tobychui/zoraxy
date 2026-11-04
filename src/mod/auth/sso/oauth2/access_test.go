package oauth2

import (
    "context"
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
    req.Header.Set("Cookie", "z-token=secret; application_session=abc; z-verifier=secret2")
    StripIdentityHeaders(req.Header)
    if req.Header.Get("Remote-User") != "" || req.Header.Get("X-Auth-Request-User") != "" {
        t.Fatal("untrusted identity headers not removed")
    }
    SetIdentityHeaders(req.Header, &Identity{Subject:"abc", PreferredUsername:"gabe", Groups: []string{"dev"}}, []string{"sub", "preferred_username", "groups"})
    if req.Header.Get("Remote-User") != "gabe" || req.Header.Get("Remote-Groups") != "dev" {
        t.Fatal("verified identity headers not inserted")
    }
    StripOAuth2Cookies(req, true)
    if req.Header.Get("Cookie") != "application_session=abc" {
        t.Fatalf("wrong cookie filtering: %q", req.Header.Get("Cookie"))
    }
}

func TestForwardedScopesParsing(t *testing.T) {
    scopes, err := ParseForwardedScopes("openid, profile  email,groups,profile")
    if err != nil {
        t.Fatal(err)
    }
    expectedScopes := []string{"openid", "profile", "email", "groups"}
    if !reflect.DeepEqual(scopes, expectedScopes) {
        t.Fatalf("scopes=%v, want %v", scopes, expectedScopes)
    }
    if _, err := ParseForwardedScopes("profile, unknown_scope"); err == nil {
        t.Fatal("unsupported forwarding scope must be rejected")
    }
    empty, err := ParseForwardedScopes("")
    if err != nil || empty == nil || len(empty) != 0 {
        t.Fatalf("empty list must be an explicit, non-nil list: %v, %v", empty, err)
    }
}

func TestClaimsForScopes(t *testing.T) {
    claims := ClaimsForScopes([]string{"openid", "profile"})
    expected := []string{"sub", "preferred_username", "name", "display_name", "nickname", "picture"}
    if !reflect.DeepEqual(claims, expected) {
        t.Fatalf("claims=%v, want %v", claims, expected)
    }
    if len(ClaimsForScopes([]string{})) != 0 {
        t.Fatal("empty scopes must not forward any claim")
    }
}

func TestScopesForLegacyClaims(t *testing.T) {
    scopes := ScopesForClaims([]string{"sub", "name", "groups"})
    want := []string{"openid", "profile", "groups"}
    if !reflect.DeepEqual(scopes, want) {
        t.Fatalf("scopes=%v, want %v", scopes, want)
    }
    if len(ScopesForClaims(nil)) != 4 {
        t.Fatal("legacy default should represent the standard forwarding scopes")
    }
}

func TestScopeRestrictedHeaders(t *testing.T) {
    header := http.Header{}
    identity := &Identity{
        Subject: "123", PreferredUsername: "test", Name: "Person",
        Email: "test@example.com", Groups: []string{"admins"},
    }
    SetIdentityHeaders(header, identity, ClaimsForScopes([]string{"openid", "email"}))
    if header.Get("Remote-Subject") != "123" || header.Get("Remote-Email") != "test@example.com" {
        t.Fatal("expected claims missing")
    }
    if header.Get("Remote-User") != "" || header.Get("Remote-Groups") != "" {
        t.Fatal("forwarded claims leaked across scope selection")
    }
}
