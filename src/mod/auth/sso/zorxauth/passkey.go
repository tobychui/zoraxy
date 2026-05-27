package zorxauth

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	webauthnlib "github.com/go-webauthn/webauthn/webauthn"
	"imuslab.com/zoraxy/mod/utils"
)

// toWebAuthnCredential converts a stored PasskeyCredential back to the webauthn.Credential type.
func (c *PasskeyCredential) toWebAuthnCredential() webauthnlib.Credential {
	transports := make([]protocol.AuthenticatorTransport, len(c.Transports))
	for i, t := range c.Transports {
		transports[i] = protocol.AuthenticatorTransport(t)
	}
	return webauthnlib.Credential{
		ID:        c.ID,
		PublicKey: c.PublicKey,
		Transport: transports,
		Flags: webauthnlib.CredentialFlags{
			BackupEligible: c.BackupEligible,
			BackupState:    c.BackupState,
		},
		Authenticator: webauthnlib.Authenticator{
			AAGUID:       c.AAGUID,
			SignCount:    c.SignCount,
			CloneWarning: c.CloneWarning,
		},
	}
}

// PendingPasskeyRegistration holds WebAuthn session state while the user completes a registration ceremony.
type PendingPasskeyRegistration struct {
	Username    string
	SessionData *webauthnlib.SessionData
	Expiry      time.Time
}

// PendingPasskeyAuth holds WebAuthn session state while the user completes a login ceremony.
type PendingPasskeyAuth struct {
	SessionData    *webauthnlib.SessionData
	RedirectTarget string
	Expiry         time.Time
}

// webAuthnUser wraps User to satisfy the webauthn.User interface.
type webAuthnUser struct {
	u *User
}

func (w *webAuthnUser) WebAuthnID() []byte         { return []byte(w.u.ID) }
func (w *webAuthnUser) WebAuthnName() string        { return w.u.Username }
func (w *webAuthnUser) WebAuthnDisplayName() string { return w.u.Username }
func (w *webAuthnUser) WebAuthnCredentials() []webauthnlib.Credential {
	creds := make([]webauthnlib.Credential, len(w.u.PasskeyCredentials))
	for i, c := range w.u.PasskeyCredentials {
		creds[i] = c.toWebAuthnCredential()
	}
	return creds
}

// newWebAuthnFromRequest creates a WebAuthn instance configured for the origin of the current request.
// RPID = bare hostname (no port); origin = scheme + host (with port if non-standard).
func newWebAuthnFromRequest(r *http.Request) (*webauthnlib.WebAuthn, error) {
	host := r.Host
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		host = strings.Split(fwdHost, ",")[0] // first value if comma-separated
		host = strings.TrimSpace(host)
	}

	// Strip port for RPID (bracket-safe for IPv6)
	rpID := host
	if i := strings.LastIndex(host, ":"); i > strings.LastIndex(host, "]") {
		rpID = host[:i]
	}

	proto := "https"
	if r.TLS == nil && !strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		proto = "http"
	}
	origin := fmt.Sprintf("%s://%s", proto, host)

	return webauthnlib.New(&webauthnlib.Config{
		RPDisplayName: "Zoraxy Auth",
		RPID:          rpID,
		RPOrigins:     []string{origin},
	})
}

/* ── Registration (user portal – must be authenticated) ──────────────────── */

// handlePasskeyRegisterBegin generates a WebAuthn registration challenge.
// POST /user/api/passkey/register/begin
func (gs *GatewayServer) handlePasskeyRegisterBegin(w http.ResponseWriter, r *http.Request, username string) {
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "method not allowed")
		return
	}

	u, err := gs.router.getUserByUsername(username)
	if err != nil {
		utils.SendErrorResponse(w, "user not found")
		return
	}

	wa, err := newWebAuthnFromRequest(r)
	if err != nil {
		utils.SendErrorResponse(w, "webauthn init failed: "+err.Error())
		return
	}

	options, sessionData, err := wa.BeginRegistration(&webAuthnUser{u: u})
	if err != nil {
		utils.SendErrorResponse(w, "begin registration failed: "+err.Error())
		return
	}

	token := gs.router.generateSessionToken()
	gs.router.pendingPasskeyReg.Store(token, &PendingPasskeyRegistration{
		Username:    username,
		SessionData: sessionData,
		Expiry:      time.Now().Add(5 * time.Minute),
	})
	time.AfterFunc(5*time.Minute, func() { gs.router.pendingPasskeyReg.Delete(token) })

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":   token,
		"options": options,
	})
}

// handlePasskeyRegisterComplete verifies the attestation and persists the new credential.
// POST /user/api/passkey/register/complete?token=<token>&name=<display-name>
// Body: PublicKeyCredential JSON (application/json)
func (gs *GatewayServer) handlePasskeyRegisterComplete(w http.ResponseWriter, r *http.Request, username string) {
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "method not allowed")
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		utils.SendErrorResponse(w, "missing token parameter")
		return
	}

	pendingObj, ok := gs.router.pendingPasskeyReg.Load(token)
	if !ok {
		utils.SendErrorResponse(w, "registration session not found or expired")
		return
	}
	pending := pendingObj.(*PendingPasskeyRegistration)
	if pending.Username != username || time.Now().After(pending.Expiry) {
		gs.router.pendingPasskeyReg.Delete(token)
		utils.SendErrorResponse(w, "registration session expired")
		return
	}

	u, err := gs.router.getUserByUsername(username)
	if err != nil {
		utils.SendErrorResponse(w, "user not found")
		return
	}

	wa, err := newWebAuthnFromRequest(r)
	if err != nil {
		utils.SendErrorResponse(w, "webauthn init failed")
		return
	}

	credential, err := wa.FinishRegistration(&webAuthnUser{u: u}, *pending.SessionData, r)
	if err != nil {
		utils.SendErrorResponse(w, "registration verification failed: "+err.Error())
		return
	}
	gs.router.pendingPasskeyReg.Delete(token)

	credName := strings.TrimSpace(r.URL.Query().Get("name"))
	if credName == "" {
		credName = fmt.Sprintf("Passkey %d", len(u.PasskeyCredentials)+1)
	}

	transports := make([]string, len(credential.Transport))
	for i, t := range credential.Transport {
		transports[i] = string(t)
	}

	u.PasskeyCredentials = append(u.PasskeyCredentials, PasskeyCredential{
		ID:             credential.ID,
		PublicKey:      credential.PublicKey,
		AAGUID:         credential.Authenticator.AAGUID,
		SignCount:      credential.Authenticator.SignCount,
		CloneWarning:   credential.Authenticator.CloneWarning,
		Transports:     transports,
		BackupEligible: credential.Flags.BackupEligible,
		BackupState:    credential.Flags.BackupState,
		Name:           credName,
		CreatedAt:      time.Now().Unix(),
		LastUsedAt:     time.Now().Unix(),
	})
	if err := gs.router.saveUser(u, u.Username); err != nil {
		utils.SendErrorResponse(w, "failed to save credential: "+err.Error())
		return
	}

	utils.SendOK(w)
}

// handlePasskeyList returns the list of passkeys registered to the current user (no secret material).
// GET /user/api/passkey/list
func (gs *GatewayServer) handlePasskeyList(w http.ResponseWriter, r *http.Request, username string) {
	if r.Method != http.MethodGet {
		utils.SendErrorResponse(w, "method not allowed")
		return
	}

	u, err := gs.router.getUserByUsername(username)
	if err != nil {
		utils.SendErrorResponse(w, "user not found")
		return
	}

	type passkeyInfo struct {
		ID         string   `json:"id"` // base64url encoded
		Name       string   `json:"name"`
		CreatedAt  int64    `json:"createdAt"`
		LastUsedAt int64    `json:"lastUsedAt"`
		Transports []string `json:"transports"`
		BackedUp   bool     `json:"backedUp"`
	}

	result := make([]passkeyInfo, len(u.PasskeyCredentials))
	for i, c := range u.PasskeyCredentials {
		result[i] = passkeyInfo{
			ID:         base64.RawURLEncoding.EncodeToString(c.ID),
			Name:       c.Name,
			CreatedAt:  c.CreatedAt,
			LastUsedAt: c.LastUsedAt,
			Transports: c.Transports,
			BackedUp:   c.BackupState,
		}
	}

	js, _ := json.Marshal(result)
	utils.SendJSONResponse(w, string(js))
}

// handlePasskeyRemove removes a specific passkey credential.
// POST /user/api/passkey/remove   body: id=<base64url-credential-id>
func (gs *GatewayServer) handlePasskeyRemove(w http.ResponseWriter, r *http.Request, username string) {
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "method not allowed")
		return
	}

	idStr, err := utils.PostPara(r, "id")
	if err != nil || idStr == "" {
		utils.SendErrorResponse(w, "id is required")
		return
	}

	rawID, err := base64.RawURLEncoding.DecodeString(idStr)
	if err != nil {
		utils.SendErrorResponse(w, "invalid credential id encoding")
		return
	}

	u, err := gs.router.getUserByUsername(username)
	if err != nil {
		utils.SendErrorResponse(w, "user not found")
		return
	}

	var updated []PasskeyCredential
	removed := false
	for _, c := range u.PasskeyCredentials {
		if bytes.Equal(c.ID, rawID) {
			removed = true
			continue
		}
		updated = append(updated, c)
	}
	if !removed {
		utils.SendErrorResponse(w, "credential not found")
		return
	}

	u.PasskeyCredentials = updated
	if err := gs.router.saveUser(u, u.Username); err != nil {
		utils.SendErrorResponse(w, "failed to remove passkey: "+err.Error())
		return
	}

	utils.SendOK(w)
}

/* ── Authentication (gateway – unauthenticated) ──────────────────────────── */

// handlePasskeyAuthBegin starts a discoverable-credential login ceremony.
// POST /passkey/auth/begin   body: redirect=<url>
func (gs *GatewayServer) handlePasskeyAuthBegin(w http.ResponseWriter, r *http.Request) {
	if gs.ServeGatewayDisabled(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "method not allowed")
		return
	}

	redirectTarget := r.FormValue("redirect")
	if redirectTarget == "" {
		redirectTarget = gs.router.Options.FallbackRedirectURL
		if redirectTarget == "" {
			redirectTarget = "about:blank"
		}
	}

	wa, err := newWebAuthnFromRequest(r)
	if err != nil {
		utils.SendErrorResponse(w, "webauthn init failed: "+err.Error())
		return
	}

	options, sessionData, err := wa.BeginDiscoverableLogin()
	if err != nil {
		utils.SendErrorResponse(w, "begin passkey login failed: "+err.Error())
		return
	}

	token := gs.router.generateSessionToken()
	gs.router.pendingPasskeyAuth.Store(token, &PendingPasskeyAuth{
		SessionData:    sessionData,
		RedirectTarget: redirectTarget,
		Expiry:         time.Now().Add(5 * time.Minute),
	})
	time.AfterFunc(5*time.Minute, func() { gs.router.pendingPasskeyAuth.Delete(token) })

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":   token,
		"options": options,
	})
}

// handlePasskeyAuthComplete verifies the assertion and issues a gateway session.
// POST /passkey/auth/complete?token=<token>
// Body: PublicKeyCredential assertion JSON (application/json)
func (gs *GatewayServer) handlePasskeyAuthComplete(w http.ResponseWriter, r *http.Request) {
	if gs.ServeGatewayDisabled(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "method not allowed")
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		utils.SendErrorResponse(w, "missing token parameter")
		return
	}

	pendingObj, ok := gs.router.pendingPasskeyAuth.Load(token)
	if !ok {
		utils.SendErrorResponse(w, "auth session not found or expired")
		return
	}
	pending := pendingObj.(*PendingPasskeyAuth)
	if time.Now().After(pending.Expiry) {
		gs.router.pendingPasskeyAuth.Delete(token)
		utils.SendErrorResponse(w, "auth session expired")
		return
	}

	wa, err := newWebAuthnFromRequest(r)
	if err != nil {
		utils.SendErrorResponse(w, "webauthn init failed")
		return
	}

	// Use a closure to capture the found user during the discoverable login handler.
	var foundUser *User
	credential, err := wa.FinishDiscoverableLogin(func(rawID, userHandle []byte) (webauthnlib.User, error) {
		// Prefer userHandle (our stored user UUID) for O(1) lookup.
		if len(userHandle) > 0 {
			u, lookupErr := gs.router.getUserByID(string(userHandle))
			if lookupErr == nil {
				foundUser = u
				return &webAuthnUser{u: u}, nil
			}
		}
		// Fall back to scanning all users by credential ID.
		u, lookupErr := gs.router.getUserByCredentialID(rawID)
		if lookupErr != nil {
			return nil, fmt.Errorf("passkey not recognized")
		}
		foundUser = u
		return &webAuthnUser{u: u}, nil
	}, *pending.SessionData, r)

	if err != nil {
		gs.router.pendingPasskeyAuth.Delete(token)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Passkey verification failed",
		})
		return
	}
	gs.router.pendingPasskeyAuth.Delete(token)

	if foundUser == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "User not found",
		})
		return
	}

	// Update sign count and last-used timestamp in storage.
	for i, c := range foundUser.PasskeyCredentials {
		if bytes.Equal(c.ID, credential.ID) {
			foundUser.PasskeyCredentials[i].SignCount = credential.Authenticator.SignCount
			foundUser.PasskeyCredentials[i].CloneWarning = credential.Authenticator.CloneWarning
			foundUser.PasskeyCredentials[i].BackupState = credential.Flags.BackupState
			foundUser.PasskeyCredentials[i].LastUsedAt = time.Now().Unix()
			break
		}
	}
	_ = gs.router.saveUser(foundUser, foundUser.Username)

	// Validate host access using the redirect target.
	redirectTarget := pending.RedirectTarget
	parsedTarget, parseErr := url.Parse(redirectTarget)
	if parseErr != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Invalid redirect target",
		})
		return
	}
	host := parsedTarget.Hostname()
	port := parsedTarget.Port()
	targetProtocol := parsedTarget.Scheme
	if targetProtocol == "" {
		targetProtocol = "http"
	}

	if !gs.router.ValidateUserAccessToHost(foundUser.Username, host) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "Access to the requested host is not allowed for this user",
		})
		return
	}

	// Issue a gateway session and cookie.
	cookieDuration := gs.router.Options.CookieDuration
	isSecure := r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")

	sessionToken := gs.router.generateSessionToken()
	expiryTime := time.Now().Add(time.Duration(cookieDuration) * time.Second)
	gs.router.gatewaySessionStore.Store(sessionToken, &GatewaySession{
		Username: foundUser.Username,
		Expiry:   expiryTime,
	})
	time.AfterFunc(time.Duration(cookieDuration)*time.Second, func() {
		gs.router.gatewaySessionStore.Delete(sessionToken)
	})

	http.SetCookie(w, &http.Cookie{
		Name:     gs.router.Options.CookieName,
		Value:    sessionToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   cookieDuration,
	})

	sessionId := gs.router.generateValidationCodeForSession(foundUser.Username)
	hostWithPort := host
	if port != "" {
		hostWithPort = host + ":" + port
	}
	sessionSetURL := fmt.Sprintf("%s://%s/%s", targetProtocol, hostWithPort, strings.TrimPrefix(gs.router.Options.SSOSessionSetURL, "/"))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":        true,
		"sessionId":      sessionId,
		"redirectTarget": redirectTarget,
		"sessionSetURL":  sessionSetURL,
	})
}
