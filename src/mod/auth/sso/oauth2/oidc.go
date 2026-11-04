package oauth2

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

func hasOAuth2Scope(scopes []string, requested string) bool {
	for _, scope := range scopes {
		if scope == requested {
			return true
		}
	}
	return false
}

// verifyOIDCLogin authenticates ID token claims before trusting the access
// token. The access token remains in z-token and is used for fresh UserInfo
// requests on every subsequent protected request.
func verifyOIDCLogin(ctx context.Context, config *oauth2.Config, token *oauth2.Token, wellKnownURL, userInfoURL, expectedNonce string) error {
	if wellKnownURL == "" {
		return errors.New("OIDC requires a configured discovery URL")
	}
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return errors.New("OIDC token response is missing id_token")
	}
	if expectedNonce == "" {
		return errors.New("missing expected OIDC nonce")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnownURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetching OIDC discovery: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("OIDC discovery returned HTTP %d", resp.StatusCode)
	}
	var metadata OIDCDiscoveryDocument
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&metadata); err != nil {
		return fmt.Errorf("invalid OIDC discovery: %w", err)
	}
	if strings.TrimSpace(metadata.Issuer) == "" || strings.TrimSpace(metadata.JwksURI) == "" {
		return errors.New("OIDC discovery missing issuer or jwks_uri")
	}

	// Permit only asymmetric signature algorithms. An ID token signed using
	// 'none' or an unexpected symmetric algorithm must never be accepted.
	allowedAlgs := make([]string, 0, len(metadata.IDTokenSigningAlgValuesSupported))
	for _, alg := range metadata.IDTokenSigningAlgValuesSupported {
		switch alg {
		case "RS256", "RS384", "RS512", "PS256", "PS384", "PS512", "ES256", "ES384", "ES512", "EdDSA":
			allowedAlgs = append(allowedAlgs, alg)
		}
	}
	if len(allowedAlgs) == 0 {
		allowedAlgs = []string{"RS256"}
	}
	verifier := oidc.NewVerifier(metadata.Issuer, oidc.NewRemoteKeySet(ctx, metadata.JwksURI), &oidc.Config{
		ClientID: config.ClientID,
		SupportedSigningAlgs: allowedAlgs,
	})
	idToken, err := verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return fmt.Errorf("invalid OIDC ID token: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(idToken.Nonce), []byte(expectedNonce)) != 1 {
		return errors.New("OIDC ID token nonce mismatch")
	}
	if idToken.AccessTokenHash != "" {
		if err := idToken.VerifyAccessToken(token.AccessToken); err != nil {
			return fmt.Errorf("OIDC access token binding failed: %w", err)
		}
	}

	identity, err := FetchIdentity(ctx, config.Client(ctx, token), userInfoURL)
	if err != nil {
		return fmt.Errorf("OIDC UserInfo verification failed: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(identity.Subject), []byte(idToken.Subject)) != 1 {
		return errors.New("OIDC ID token and UserInfo subjects differ")
	}
	return nil
}
