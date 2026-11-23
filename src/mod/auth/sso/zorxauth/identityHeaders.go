package zorxauth

/*
	identityHeaders.go

	Identity headers forwarded to the upstream after a successful Zoraxy Auth login.

	Fixed names (Authelia compatible):
	  X-Auth-Method      zoraxy_sso
	  Remote-User        username
	  Remote-Name        username
	  Remote-Email       email, only when the user has one
	  Remote-Groups      assigned group policy name, only when a valid group is assigned

	Configurable names (AuthRouterOptions, empty means the default):
	  Remote-User-Id     User.ID, kept across renames, new for a deleted and re-created user
	  Remote-Group-Id    assigned group policy ID, only when a valid group is assigned
	  Remote-Group-Name  assigned group policy name, only when a valid group is assigned

	Every one of them is removed from the incoming request first, so the upstream
	only ever sees values derived from the authenticated Zoraxy Auth user.
*/

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

const (
	DefaultUserIDHeader    = "Remote-User-Id"
	DefaultGroupIDHeader   = "Remote-Group-Id"
	DefaultGroupNameHeader = "Remote-Group-Name"
)

// fixedIdentityHeaders are always controlled by Zoraxy Auth
var fixedIdentityHeaders = []string{"X-Auth-Method", "Remote-User", "Remote-Name", "Remote-Email", "Remote-Groups"}

// reservedHeaderNames cannot be used as an identity header because overwriting
// them would break proxying, sessions or client IP detection
var reservedHeaderNames = []string{
	"Host", "Cookie", "Set-Cookie", "Authorization", "Proxy-Authorization",
	"Connection", "Keep-Alive", "Proxy-Connection", "Transfer-Encoding", "Te", "Trailer", "Upgrade",
	"Content-Length", "Content-Type", "Content-Encoding",
	"Forwarded", "X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Real-Ip",
}

// headerNamePattern allows letters and digits separated by single dashes.
// Underscores are refused since many servers drop or merge them with dashes.
var headerNamePattern = regexp.MustCompile(`^[A-Za-z0-9]+(-[A-Za-z0-9]+)*$`)

const maxIdentityHeaderNameLength = 64

// IdentityHeaderNames are the effective names of the configurable identity headers
type IdentityHeaderNames struct {
	UserID    string `json:"userIdHeader"`
	GroupID   string `json:"groupIdHeader"`
	GroupName string `json:"groupNameHeader"`
}

// identityHeaderNames returns the configured identity header names with defaults applied
func (ar *AuthRouter) identityHeaderNames() IdentityHeaderNames {
	names := IdentityHeaderNames{
		UserID:    DefaultUserIDHeader,
		GroupID:   DefaultGroupIDHeader,
		GroupName: DefaultGroupNameHeader,
	}
	if ar.Options == nil {
		return names
	}
	if v := strings.TrimSpace(ar.Options.UserIDHeader); v != "" {
		names.UserID = v
	}
	if v := strings.TrimSpace(ar.Options.GroupIDHeader); v != "" {
		names.GroupID = v
	}
	if v := strings.TrimSpace(ar.Options.GroupNameHeader); v != "" {
		names.GroupName = v
	}
	return names
}

// normalizeIdentityHeaderName validates one configurable header name and returns
// its canonical form. An empty value selects defaultName.
// Error messages only echo names that passed headerNamePattern, since
// utils.SendErrorResponse does not escape the message into its JSON.
func normalizeIdentityHeaderName(raw, defaultName string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return defaultName, nil
	}
	if len(name) > maxIdentityHeaderNameLength || !headerNamePattern.MatchString(name) {
		return "", fmt.Errorf("invalid header name: use letters, digits and single dashes only, up to %d characters", maxIdentityHeaderNameLength)
	}
	name = http.CanonicalHeaderKey(name)
	for _, reserved := range fixedIdentityHeaders {
		if strings.EqualFold(name, reserved) {
			return "", fmt.Errorf("header name %s is already used by Zoraxy Auth", name)
		}
	}
	for _, reserved := range reservedHeaderNames {
		if strings.EqualFold(name, reserved) {
			return "", fmt.Errorf("header name %s is reserved", name)
		}
	}
	return name, nil
}

// validateIdentityHeaderNames validates and canonicalizes all configurable identity header names
func validateIdentityHeaderNames(userID, groupID, groupName string) (IdentityHeaderNames, error) {
	var names IdentityHeaderNames
	var err error
	if names.UserID, err = normalizeIdentityHeaderName(userID, DefaultUserIDHeader); err != nil {
		return names, err
	}
	if names.GroupID, err = normalizeIdentityHeaderName(groupID, DefaultGroupIDHeader); err != nil {
		return names, err
	}
	if names.GroupName, err = normalizeIdentityHeaderName(groupName, DefaultGroupNameHeader); err != nil {
		return names, err
	}
	if strings.EqualFold(names.UserID, names.GroupID) ||
		strings.EqualFold(names.UserID, names.GroupName) ||
		strings.EqualFold(names.GroupID, names.GroupName) {
		return names, errors.New("identity header names must be different from each other")
	}
	return names, nil
}

// headerMatchKey folds a header name so spellings with '_' match the '-' form,
// which CGI style backends treat as the same header
func headerMatchKey(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
}

// identityHeaderKeys returns the match keys of every header Zoraxy Auth controls.
// The default names are included even when custom names are configured, so an
// upstream still reading a default name never sees a client supplied value.
func (ar *AuthRouter) identityHeaderKeys() map[string]bool {
	names := ar.identityHeaderNames()
	all := append([]string{
		DefaultUserIDHeader, DefaultGroupIDHeader, DefaultGroupNameHeader,
		names.UserID, names.GroupID, names.GroupName,
	}, fixedIdentityHeaders...)

	keys := make(map[string]bool, len(all))
	for _, name := range all {
		keys[headerMatchKey(name)] = true
	}
	return keys
}

// StripIdentityHeaders removes every Zoraxy Auth identity header from the request,
// including repeated values and '_' spellings. Use it wherever a request
// reaches the upstream without passing Zoraxy Auth (e.g. auth-exempt routes).
func (ar *AuthRouter) StripIdentityHeaders(r *http.Request) {
	keys := ar.identityHeaderKeys()
	for name := range r.Header {
		if keys[headerMatchKey(name)] {
			delete(r.Header, name)
		}
	}
}

// headerSafeValue removes control characters so a stored value can never split
// or invalidate a forwarded header (the transport rejects such requests)
func headerSafeValue(value string) string {
	return sanitizeBrandingText(value)
}

// setAuthHeaders replaces the identity headers of the request with the values of the authenticated user
func (ar *AuthRouter) setAuthHeaders(r *http.Request, user *User) {
	ar.StripIdentityHeaders(r)
	names := ar.identityHeaderNames()

	r.Header.Set("X-Auth-Method", "zoraxy_sso")
	r.Header.Set("Remote-User", headerSafeValue(user.Username)) //Authelia compatibility
	r.Header.Set("Remote-Name", headerSafeValue(user.Username)) //Authelia compatibility
	if email := headerSafeValue(user.Email); email != "" {
		r.Header.Set("Remote-Email", email) //Authelia compatibility
	}
	if userID := headerSafeValue(user.ID); userID != "" {
		r.Header.Set(names.UserID, userID)
	}

	group := ar.getAssignedGroupPolicy(user)
	if group == nil {
		return
	}
	if groupID := headerSafeValue(group.ID); groupID != "" {
		r.Header.Set(names.GroupID, groupID)
	}
	if groupName := headerSafeValue(group.Name); groupName != "" {
		r.Header.Set(names.GroupName, groupName)
	}
	// Authelia's Remote-Groups is a comma separated list, so a comma inside
	// the name must not be read as a second group
	if groups := strings.TrimSpace(strings.ReplaceAll(headerSafeValue(group.Name), ",", " ")); groups != "" {
		r.Header.Set("Remote-Groups", groups) //Authelia compatibility
	}
}
