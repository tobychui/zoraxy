package zorxauth

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"net/http"
	"strings"
	"time"

	"github.com/pquerna/otp/totp"
	"imuslab.com/zoraxy/mod/utils"
)

//go:embed user.html
var userPortalHTML []byte

// handleUserPortal serves the user self-service portal page.
// The user must already hold a valid SSO gateway session cookie.
func (gs *GatewayServer) handleUserPortal(w http.ResponseWriter, r *http.Request) {
	if gs.ServeGatewayDisabled(w, r) {
		return
	}

	authenticated, _ := gs.router.RequestIsAuthenticatedInSSO(w, r)
	if !authenticated {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(userPortalHTML)
}

// handleUserPortalAPI dispatches /user/api/* requests.
func (gs *GatewayServer) handleUserPortalAPI(w http.ResponseWriter, r *http.Request) {
	if gs.ServeGatewayDisabled(w, r) {
		return
	}

	authenticated, username := gs.router.RequestIsAuthenticatedInSSO(w, r)
	if !authenticated {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "Unauthorized",
		})
		return
	}

	sub := strings.TrimPrefix(r.URL.Path, "/user/api/")

	switch sub {
	case "info":
		gs.handleUserInfo(w, r, username)
	case "change-password":
		gs.handleChangePassword(w, r, username)
	case "2fa/setup":
		gs.handleSetupTOTP(w, r, username)
	case "2fa/enable":
		gs.handleEnableTOTP(w, r, username)
	case "2fa/disable":
		gs.handleDisableTOTP(w, r, username)
	default:
		http.NotFound(w, r)
	}
}

// handleUserInfo returns the current user's non-sensitive profile.
func (gs *GatewayServer) handleUserInfo(w http.ResponseWriter, r *http.Request, username string) {
	if r.Method != http.MethodGet {
		utils.SendErrorResponse(w, "method not allowed")
		return
	}

	u, err := gs.router.getUserByUsername(username)
	if err != nil {
		utils.SendErrorResponse(w, "user not found")
		return
	}

	js, _ := json.Marshal(map[string]interface{}{
		"username":  u.Username,
		"email":     u.Email,
		"enable2fa": u.Enable2FA,
	})
	utils.SendJSONResponse(w, string(js))
}

// handleChangePassword verifies the current password and stores the new hash.
func (gs *GatewayServer) handleChangePassword(w http.ResponseWriter, r *http.Request, username string) {
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "method not allowed")
		return
	}

	currentPassword, err := utils.PostPara(r, "current_password")
	if err != nil {
		utils.SendErrorResponse(w, "current_password is required")
		return
	}

	newPassword, err := utils.PostPara(r, "new_password")
	if err != nil {
		utils.SendErrorResponse(w, "new_password is required")
		return
	}

	if len(newPassword) < 8 {
		utils.SendErrorResponse(w, "new password must be at least 8 characters")
		return
	}

	if !gs.router.ValidateUsername(username, currentPassword) {
		utils.SendErrorResponse(w, "current password is incorrect")
		return
	}

	u, err := gs.router.getUserByUsername(username)
	if err != nil {
		utils.SendErrorResponse(w, "user not found")
		return
	}

	u.PasswordHash = hashPassword(newPassword)
	if err := gs.router.saveUser(u, u.Username); err != nil {
		utils.SendErrorResponse(w, "failed to save new password: "+err.Error())
		return
	}

	utils.SendOK(w)
}

// handleSetupTOTP generates a new TOTP secret and returns the QR code.
// The secret is stored as pending until the user confirms it with handleEnableTOTP.
func (gs *GatewayServer) handleSetupTOTP(w http.ResponseWriter, r *http.Request, username string) {
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "method not allowed")
		return
	}

	u, err := gs.router.getUserByUsername(username)
	if err != nil {
		utils.SendErrorResponse(w, "user not found")
		return
	}

	if u.Enable2FA {
		utils.SendErrorResponse(w, "2FA is already enabled; disable it first to re-enroll")
		return
	}

	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "Zoraxy Auth",
		AccountName: username,
	})
	if err != nil {
		utils.SendErrorResponse(w, "failed to generate TOTP secret")
		return
	}

	gs.router.pendingTOTPSetup.Store(username, &PendingTOTPSetup{
		Secret: key.Secret(),
		Expiry: time.Now().Add(10 * time.Minute),
	})
	time.AfterFunc(10*time.Minute, func() {
		gs.router.pendingTOTPSetup.Delete(username)
	})

	// Render QR code PNG and base64-encode it for inline display
	var qrDataURL string
	if img, imgErr := key.Image(200, 200); imgErr == nil {
		var buf bytes.Buffer
		if encErr := png.Encode(&buf, img); encErr == nil {
			qrDataURL = "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
		}
	}

	js, _ := json.Marshal(map[string]interface{}{
		"secret":           key.Secret(),
		"provisioning_uri": key.URL(),
		"qr_image":         qrDataURL,
	})
	utils.SendJSONResponse(w, string(js))
}

// handleEnableTOTP validates the TOTP code against the pending secret and activates 2FA.
func (gs *GatewayServer) handleEnableTOTP(w http.ResponseWriter, r *http.Request, username string) {
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "method not allowed")
		return
	}

	code, err := utils.PostPara(r, "totp_code")
	if err != nil {
		utils.SendErrorResponse(w, "totp_code is required")
		return
	}

	pendingObj, exists := gs.router.pendingTOTPSetup.Load(username)
	if !exists {
		utils.SendErrorResponse(w, "no pending 2FA setup found; please start setup again")
		return
	}

	pending, ok := pendingObj.(*PendingTOTPSetup)
	if !ok || time.Now().After(pending.Expiry) {
		gs.router.pendingTOTPSetup.Delete(username)
		utils.SendErrorResponse(w, "2FA setup session expired; please start setup again")
		return
	}

	if !totp.Validate(code, pending.Secret) {
		utils.SendErrorResponse(w, "invalid TOTP code; please try again")
		return
	}

	u, err := gs.router.getUserByUsername(username)
	if err != nil {
		utils.SendErrorResponse(w, "user not found")
		return
	}

	u.TOTPSecret = pending.Secret
	u.Enable2FA = true
	if err := gs.router.saveUser(u, u.Username); err != nil {
		utils.SendErrorResponse(w, "failed to enable 2FA: "+err.Error())
		return
	}

	gs.router.pendingTOTPSetup.Delete(username)
	utils.SendOK(w)
}

// handleDisableTOTP verifies the current TOTP code and removes 2FA from the account.
func (gs *GatewayServer) handleDisableTOTP(w http.ResponseWriter, r *http.Request, username string) {
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "method not allowed")
		return
	}

	code, err := utils.PostPara(r, "totp_code")
	if err != nil {
		utils.SendErrorResponse(w, "totp_code is required")
		return
	}

	u, err := gs.router.getUserByUsername(username)
	if err != nil {
		utils.SendErrorResponse(w, "user not found")
		return
	}

	if !u.Enable2FA {
		utils.SendErrorResponse(w, "2FA is not currently enabled")
		return
	}

	if !totp.Validate(code, u.TOTPSecret) {
		utils.SendErrorResponse(w, "invalid TOTP code")
		return
	}

	u.Enable2FA = false
	u.TOTPSecret = ""
	if err := gs.router.saveUser(u, u.Username); err != nil {
		utils.SendErrorResponse(w, "failed to disable 2FA: "+err.Error())
		return
	}

	utils.SendOK(w)
}
