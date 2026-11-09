package oauth2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

var IdentityHeaderNames = []string{
	"Remote-Subject", "Remote-User", "Remote-Name", "Remote-Display-Name", "Remote-Nickname", "Remote-Picture", "Remote-Email", "Remote-Groups", "Remote-OIDC-UserInfo",
	"X-Remote-User", "X-Remote-Name", "X-Remote-Email", "X-Remote-Groups",
	"X-Auth-Request-User", "X-Auth-Request-Email", "X-Auth-Request-Groups",
	"X-Auth-Request-Preferred-Username", "X-Forwarded-User", "X-Forwarded-Email",
	"X-Forwarded-Groups", "X-Authenticated-User", "X-Authenticated-Email",
}

// AccessPolicy is provided by each protected proxy endpoint.
type AccessPolicy struct {
	AllowedGroups            []string
	ForwardUserInfo          bool
	ForwardWebSocketIdentity bool
	RemoveTokenCookie        bool
}

type Identity struct {
	Subject           string
	PreferredUsername string
	Name              string
	DisplayName       string
	Nickname          string
	Picture           string
	Email             string
	Groups            []string
	UserInfo          json.RawMessage
}

type verifiedIdentityHeadersKey struct{}

// HasVerifiedIdentityHeaders is true only after this request's OAuth2 identity
// was validated, authorized, and written to its headers. Incoming headers alone
// (including on an authentication bypass) never establish a verified identity.
func HasVerifiedIdentityHeaders(r *http.Request) bool {
	verified, _ := r.Context().Value(verifiedIdentityHeadersKey{}).(bool)
	return verified
}

// SplitAllowedGroups normalizes comma-separated group identifiers without changing case.
func SplitAllowedGroups(input string) []string {
	result := []string{}
	seen := make(map[string]bool)
	for _, item := range strings.Split(input, ",") {
		name := strings.TrimSpace(item)
		if name != "" && !seen[name] {
			seen[name] = true
			result = append(result, name)
		}
	}
	return result
}

func GroupsAllowed(userGroups, allowedGroups []string) bool {
	if len(allowedGroups) == 0 {
		return true
	}
	for _, allowed := range allowedGroups {
		for _, actual := range userGroups {
			if actual == allowed {
				return true
			}
		}
	}
	return false
}

// FetchIdentity validates OIDC UserInfo. OIDC requires a nonempty sub claim so
// the result can be bound to the verified ID token.
func FetchIdentity(ctx context.Context, client *http.Client, endpoint string) (*Identity, error) {
	return fetchIdentity(ctx, client, endpoint, true)
}

// FetchOAuth2Identity preserves compatibility with ordinary OAuth2 providers,
// whose validation response may use a provider-specific id field instead of
// the OIDC sub claim. A successful endpoint response still has to be valid JSON.
func FetchOAuth2Identity(ctx context.Context, client *http.Client, endpoint string) (*Identity, error) {
	return fetchIdentity(ctx, client, endpoint, false)
}

func fetchIdentity(ctx context.Context, client *http.Client, endpoint string, requireSubject bool) (*Identity, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo returned HTTP %d", resp.StatusCode)
	}

	var claims struct {
		Subject           string          `json:"sub"`
		ID                json.RawMessage `json:"id"`
		PreferredUsername string          `json:"preferred_username"`
		Name              string          `json:"name"`
		DisplayName       string          `json:"display_name"`
		Nickname          string          `json:"nickname"`
		Picture           string          `json:"picture"`
		Email             string          `json:"email"`
		Groups            json.RawMessage `json:"groups"`
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil {
		return nil, fmt.Errorf("reading userinfo response: %w", err)
	}
	if len(raw) > 1<<20 {
		return nil, errors.New("userinfo response exceeds 1 MiB")
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return nil, fmt.Errorf("invalid userinfo response: %w", err)
	}
	if err := json.Unmarshal(compact.Bytes(), &claims); err != nil {
		return nil, fmt.Errorf("invalid userinfo response: %w", err)
	}
	subject := strings.TrimSpace(claims.Subject)
	if subject == "" && !requireSubject && len(claims.ID) > 0 && string(claims.ID) != "null" {
		var stringID string
		if err := json.Unmarshal(claims.ID, &stringID); err == nil {
			subject = strings.TrimSpace(stringID)
		} else {
			var numberID json.Number
			if err := json.Unmarshal(claims.ID, &numberID); err == nil {
				subject = numberID.String()
			}
		}
	}
	if requireSubject && subject == "" {
		return nil, errors.New("userinfo response has no subject")
	}
	identity := &Identity{
		Subject: subject, PreferredUsername: claims.PreferredUsername,
		Name: claims.Name, DisplayName: claims.DisplayName, Nickname: claims.Nickname, Picture: claims.Picture, Email: claims.Email,
		UserInfo: append(json.RawMessage(nil), compact.Bytes()...),
	}
	if len(claims.Groups) > 0 && string(claims.Groups) != "null" {
		if err := json.Unmarshal(claims.Groups, &identity.Groups); err != nil {
			var single string
			if err := json.Unmarshal(claims.Groups, &single); err != nil {
				return nil, errors.New("userinfo groups claim must be an array of strings or a string")
			}
			identity.Groups = SplitAllowedGroups(single)
		}
	}
	return identity, nil
}

// StripIdentityHeaders removes both the headers we generate and commonly used
// alternative identity headers that an untrusted client might otherwise forge.
func StripIdentityHeaders(headers http.Header) {
	for _, name := range IdentityHeaderNames {
		headers.Del(name)
	}
}

// StripOAuth2Cookies removes OAuth2-internal cookies before proxying upstream.
// The browser continues sending the cookie to Zoraxy for session validation.
func StripOAuth2Cookies(r *http.Request, removeToken bool) {
	cookies := r.Cookies()
	kept := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		if isOAuth2SessionCookie(cookie.Name) || cookie.Name == "z-verifier" || cookie.Name == "z-oauth-state" || cookie.Name == "z-oauth-return" || cookie.Name == "z-oidc-nonce" ||
			(removeToken && cookie.Name == "z-token") {
			continue
		}
		kept = append(kept, cookie.String())
	}
	r.Header.Del("Cookie")
	if len(kept) > 0 {
		r.Header.Set("Cookie", strings.Join(kept, "; "))
	}
}

func cleanHeaderValue(input string) string {
	return strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, input)
}

// SetIdentityHeaders forwards the complete verified UserInfo JSON plus familiar identity headers.
func SetIdentityHeaders(headers http.Header, user *Identity) {
	if len(user.UserInfo) > 0 {
		headers.Set("Remote-OIDC-UserInfo", string(user.UserInfo))
	}
	values := []struct{ header, value string }{
		{"Remote-Subject", user.Subject},
		{"Remote-User", user.PreferredUsername},
		{"Remote-Name", user.Name},
		{"Remote-Display-Name", user.DisplayName},
		{"Remote-Nickname", user.Nickname},
		{"Remote-Picture", user.Picture},
		{"Remote-Email", user.Email},
		{"Remote-Groups", strings.Join(user.Groups, ",")},
	}
	for _, item := range values {
		if item.value != "" {
			headers.Set(item.header, cleanHeaderValue(item.value))
		}
	}
}
