package oauth2

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestVerifyOIDCLogin(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const nonce = "expected-login-nonce"
	const clientID = "zoraxy-test-client"
	const subject = "user-123"
	var userInfoSubject = subject
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"issuer": srv.URL,
				"jwks_uri": srv.URL + "/jwks",
				"id_token_signing_alg_values_supported": []string{"RS256"},
			})
		case "/jwks":
			n := base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes())
			e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes())
			json.NewEncoder(w).Encode(map[string]interface{}{
				"keys": []interface{}{map[string]string{"kty":"RSA","kid":"test-key","alg":"RS256","use":"sig","n":n,"e":e}},
			})
		case "/userinfo":
			if r.Header.Get("Authorization") != "Bearer test-access-token" {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"sub": userInfoSubject})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	signIDToken := func(sub, tokenNonce, audience string, expiry time.Time) string {
		t.Helper()
		header, _ := json.Marshal(map[string]string{"alg":"RS256","typ":"JWT","kid":"test-key"})
		claims, _ := json.Marshal(map[string]interface{}{
			"iss":srv.URL, "sub":sub, "aud":audience, "exp":expiry.Unix(),
			"iat":time.Now().Add(-time.Minute).Unix(), "nonce":tokenNonce,
		})
		payload := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
		h := sha256.Sum256([]byte(payload))
		signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, h[:])
		if err != nil {
			t.Fatal(err)
		}
		return payload + "." + base64.RawURLEncoding.EncodeToString(signature)
	}
	conf := &oauth2.Config{ClientID: clientID}
	tests := []struct{
		name string
		subject, nonce, audience, userInfoSubject string
		expiry time.Time
		wantError string
	}{
		{"valid", subject, nonce, clientID, subject, time.Now().Add(time.Hour), ""},
		{"different-subject", subject, nonce, clientID, "other-user", time.Now().Add(time.Hour), "subjects differ"},
		{"wrong-nonce", subject, "different-nonce", clientID, subject, time.Now().Add(time.Hour), "nonce mismatch"},
		{"wrong-audience", subject, nonce, "other-client", subject, time.Now().Add(time.Hour), "invalid OIDC ID token"},
		{"expired", subject, nonce, clientID, subject, time.Now().Add(-time.Hour), "invalid OIDC ID token"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			userInfoSubject = tc.userInfoSubject
			jwt := signIDToken(tc.subject, tc.nonce, tc.audience, tc.expiry)
			accessToken := (&oauth2.Token{AccessToken: "test-access-token"}).WithExtra(map[string]interface{}{"id_token":jwt})
			err := verifyOIDCLogin(context.Background(), conf, accessToken, srv.URL+"/.well-known/openid-configuration", srv.URL+"/userinfo", nonce)
			if tc.wantError == "" && err != nil {
				t.Fatalf("unexpected verification failure: %v", err)
			}
			if tc.wantError != "" && (err == nil || !strings.Contains(err.Error(), tc.wantError)) {
				t.Fatalf("expected %q, got %v", tc.wantError, err)
			}
		})
	}
	missing := &oauth2.Token{AccessToken: "test-access-token"}
	if err := verifyOIDCLogin(context.Background(), conf, missing, srv.URL+"/.well-known/openid-configuration", srv.URL+"/userinfo", nonce); err == nil {
		t.Fatal("missing ID token must fail")
	}
}

func TestHasOAuth2Scope(t *testing.T) {
	if !hasOAuth2Scope([]string{"openid","profile","email"}, "openid") {
		t.Fatal("openid scope not recognized")
	}
	if hasOAuth2Scope([]string{"profile","email"}, "openid") {
		t.Fatal("ordinary OAuth2 login should not require OIDC")
	}
}
