package oauth2

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNewOAuth2ConfUsesConservativeDiscoveryScopes(t *testing.T) {
	options := &OAuth2RouterOptions{
		OAuth2WellKnownUrl: "https://issuer.example/.well-known/openid-configuration",
		OAuth2ServerURL:    "https://issuer.example/authorize",
		OAuth2TokenURL:     "https://issuer.example/token",
		OAuth2UserInfoUrl:  "https://issuer.example/userinfo",
	}
	config, err := newOAuth2Conf("https://app.example/internal/oauth2", options)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(config.Scopes, defaultOIDCScopes) {
		t.Fatalf("got scopes %v, want %v", config.Scopes, defaultOIDCScopes)
	}

	options.OAuth2Scopes = "openid custom"
	config, err = newOAuth2Conf("https://app.example/internal/oauth2", options)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(config.Scopes, []string{"openid", "custom"}) {
		t.Fatalf("explicit scopes were replaced: %v", config.Scopes)
	}
}

func TestNewOAuth2ReturnCookieBoundsSerializedSize(t *testing.T) {
	expires := time.Unix(1_800_000_000, 0)
	longPath := "/search?q=" + strings.Repeat("%20", 900)
	cookie := newOAuth2ReturnCookie(longPath, true, expires)
	if got := len(cookie.String()); got > maxOAuth2CookieSize {
		t.Fatalf("serialized cookie is %d bytes, limit is %d", got, maxOAuth2CookieSize)
	}
	if cookie.Value != "%2F" {
		t.Fatalf("oversized return path did not fall back to root: %q", cookie.Value)
	}

	short := newOAuth2ReturnCookie("/dashboard?tab=users", false, expires)
	if short.Value == "%2F" {
		t.Fatal("valid short return path unexpectedly fell back to root")
	}
}
