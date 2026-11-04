package oauth2

import (
    "context"
    "encoding/json"
    "errors"
    "fmt"
    "io"
    "net/http"
    "strings"
)

var IdentityHeaderNames = []string{
    "Remote-Subject", "Remote-User", "Remote-Name", "Remote-Display-Name", "Remote-Nickname", "Remote-Picture", "Remote-Email", "Remote-Groups",
    "X-Remote-User", "X-Remote-Name", "X-Remote-Email", "X-Remote-Groups",
    "X-Auth-Request-User", "X-Auth-Request-Email", "X-Auth-Request-Groups",
    "X-Auth-Request-Preferred-Username", "X-Forwarded-User", "X-Forwarded-Email",
    "X-Forwarded-Groups", "X-Authenticated-User", "X-Authenticated-Email",
}

var DefaultForwardedClaims = []string{"sub", "preferred_username", "name", "display_name", "nickname", "picture", "email", "groups"}

// AccessPolicy is provided by each protected proxy endpoint.
type AccessPolicy struct {
    AllowedGroups            []string
    ForwardUserInfo          bool
    ForwardWebSocketIdentity bool
    ForwardedClaims          []string // nil: forward all available claims
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

// FetchIdentity validates a bearer token with the configured OIDC UserInfo endpoint.
// Only successful responses containing a nonempty subject count as authentication.
func FetchIdentity(ctx context.Context, client *http.Client, endpoint string) (*Identity, error) {
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
        PreferredUsername string          `json:"preferred_username"`
        Name              string          `json:"name"`
        DisplayName       string          `json:"display_name"`
        Nickname          string          `json:"nickname"`
        Picture           string          `json:"picture"`
        Email             string          `json:"email"`
        Groups            json.RawMessage `json:"groups"`
    }
    if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&claims); err != nil {
        return nil, fmt.Errorf("invalid userinfo response: %w", err)
    }
    if strings.TrimSpace(claims.Subject) == "" {
        return nil, errors.New("userinfo response has no subject")
    }
    identity := &Identity{
        Subject: claims.Subject, PreferredUsername: claims.PreferredUsername,
        Name: claims.Name, DisplayName: claims.DisplayName, Nickname: claims.Nickname, Picture: claims.Picture, Email: claims.Email,
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
        if cookie.Name == "z-verifier" || cookie.Name == "z-oauth-state" || cookie.Name == "z-oauth-return" ||
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

func claimSelected(claim string, selected []string) bool {
    if selected == nil {
        return true
    }
    for _, item := range selected {
        if item == claim {
            return true
        }
    }
    return false
}

func cleanHeaderValue(input string) string {
    return strings.Map(func(r rune) rune {
        if r < 32 || r == 127 {
            return -1
        }
        return r
    }, input)
}

// SetIdentityHeaders only forwards claims actually returned by the identity provider.
func SetIdentityHeaders(headers http.Header, user *Identity, selected []string) {
    values := []struct{ claim, header, value string }{
        {"sub", "Remote-Subject", user.Subject},
        {"preferred_username", "Remote-User", user.PreferredUsername},
        {"name", "Remote-Name", user.Name},
        {"display_name", "Remote-Display-Name", user.DisplayName},
        {"nickname", "Remote-Nickname", user.Nickname},
        {"picture", "Remote-Picture", user.Picture},
        {"email", "Remote-Email", user.Email},
        {"groups", "Remote-Groups", strings.Join(user.Groups, ",")},
    }
    for _, item := range values {
        if claimSelected(item.claim, selected) && item.value != "" {
            headers.Set(item.header, cleanHeaderValue(item.value))
        }
    }
}

func DefaultClaims() []string {
    return append([]string(nil), DefaultForwardedClaims...)
}
