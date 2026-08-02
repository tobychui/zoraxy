package auth

/*

	author: tobychui
*/

import (
	"bytes"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"maps"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/protocol/webauthncose"
	webauthnlib "github.com/go-webauthn/webauthn/webauthn"
	"github.com/gorilla/sessions"
	"github.com/pquerna/otp/totp"
	db "imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/utils"
)

type AuthAgent struct {
	//Session related
	SessionName             string
	SessionStore            *sessions.CookieStore
	Database                *db.Database
	LoginRedirectionHandler func(http.ResponseWriter, *http.Request)
	Logger                  *logger.Logger
}

type AuthEndpoints struct {
	Login         string
	Logout        string
	Register      string
	CheckLoggedIn string
	Autologin     string
}

// WebAuthn credential storage struct
type WebAuthnCredential struct {
	ID             []byte   `json:"id"`
	PublicKey      []byte   `json:"publicKey"`
	AAGUID         []byte   `json:"aaguid"`
	SignCount      uint32   `json:"signCount"`
	Transports     []string `json:"transports"`
	BackupEligible bool     `json:"backupEligible"`
	BackupState    bool     `json:"backupState"`
	Name           string   `json:"name"`
	CreatedAt      int64    `json:"createdAt"`
	LastUsedAt     int64    `json:"lastUsedAt"`
}

// toWebAuthnCredential converts a stored WebAuthnCredential back to the webauthn.Credential type.
func (c *WebAuthnCredential) toWebAuthnCredential() webauthnlib.Credential {
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
			AAGUID:    c.AAGUID,
			SignCount: c.SignCount,
		},
	}
}

// PendingWebAuthnRegistration holds WebAuthn session state during registration.
type PendingWebAuthnRegistration struct {
	Username    string
	SessionData *webauthnlib.SessionData
	Expiry      time.Time
}

// PendingWebAuthnAuth holds WebAuthn session state during authentication.
type PendingWebAuthnAuth struct {
	SessionData *webauthnlib.SessionData
	RedirectURL string
	Expiry      time.Time
}

// sessionDataWrapper is a JSON-serializable copy of webauthnlib.SessionData.
// SessionData contains unexported fields (protocol types) that can't round-trip through
// map[string]any database storage. So it serializes it to JSON bytes instead.
type sessionDataWrapper struct {
	Challenge            string           `json:"challenge"`
	RelyingPartyID       string           `json:"rpid"`
	UserID               []byte           `json:"userid"`
	AllowedCredentialIDs [][]byte         `json:"allowedcredentialids"`
	UserVerification     string           `json:"userverification"`
	CredParams           []map[string]any `json:"credparams"`
	Mediation            string           `json:"mediation"`
	Extensions           map[string]any   `json:"extensions"`
	Expires              int64            `json:"expires"`
}

// serializeSessionData converts *webauthnlib.SessionData. See the comment on sessionDataWrapper for details.
func serializeSessionData(sd *webauthnlib.SessionData) ([]byte, error) {
	credParams := make([]map[string]any, len(sd.CredParams))
	for i, cp := range sd.CredParams {
		credParams[i] = map[string]any{
			"type": cp.Type,
			"alg":  cp.Algorithm,
		}
	}
	expires := sd.Expires.Unix()

	wrapper := sessionDataWrapper{
		Challenge:            sd.Challenge,
		RelyingPartyID:       sd.RelyingPartyID,
		UserID:               sd.UserID,
		AllowedCredentialIDs: sd.AllowedCredentialIDs,
		UserVerification:     string(sd.UserVerification),
		CredParams:           credParams,
		Mediation:            string(sd.Mediation),
		Expires:              expires,
	}
	if sd.Extensions != nil {
		wrapper.Extensions = map[string]any{}
		maps.Copy(wrapper.Extensions, sd.Extensions)
	}
	return json.Marshal(wrapper)
}

// deserializeSessionData converts JSON bytes back to *webauthnlib.SessionData.
func deserializeSessionData(data []byte) (*webauthnlib.SessionData, error) {
	var wrapper sessionDataWrapper
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, err
	}

	// Restore UserVerificationRequirement from string
	var userVer protocol.UserVerificationRequirement
	switch wrapper.UserVerification {
	case "required":
		userVer = protocol.VerificationRequired
	case "preferred":
		userVer = protocol.VerificationPreferred
	case "discouraged":
		userVer = protocol.VerificationDiscouraged
	default:
		userVer = protocol.VerificationPreferred
	}

	// Restore CredentialMediationRequirement from string
	var mediation protocol.CredentialMediationRequirement
	switch wrapper.Mediation {
	case "optional":
		mediation = protocol.MediationOptional
	case "conditional":
		mediation = protocol.MediationConditional
	case "silent":
		mediation = protocol.MediationSilent
	default:
		mediation = protocol.MediationDefault
	}

	// Restore CredParams
	credParams := make([]protocol.CredentialParameter, len(wrapper.CredParams))
	for i, cp := range wrapper.CredParams {
		alg, _ := cp["alg"].(float64)
		credParams[i] = protocol.CredentialParameter{
			Type:      protocol.CredentialType(cp["type"].(string)),
			Algorithm: webauthncose.COSEAlgorithmIdentifier(alg),
		}
	}

	sd := &webauthnlib.SessionData{
		Challenge:            wrapper.Challenge,
		RelyingPartyID:       wrapper.RelyingPartyID,
		UserID:               wrapper.UserID,
		AllowedCredentialIDs: wrapper.AllowedCredentialIDs,
		UserVerification:     userVer,
		CredParams:           credParams,
		Mediation:            mediation,
		Expires:              time.Unix(wrapper.Expires, 0),
	}
	if wrapper.Extensions != nil {
		sd.Extensions = make(protocol.AuthenticationExtensions)
		maps.Copy(sd.Extensions, wrapper.Extensions)
	}
	return sd, nil
}

// webAuthnUser wraps the AuthAgent and username to satisfy the webauthn.User interface.
type webAuthnUser struct {
	aAgent   *AuthAgent
	Username string
}

func (w *webAuthnUser) WebAuthnID() []byte          { return []byte(w.Username) }
func (w *webAuthnUser) WebAuthnName() string        { return w.Username }
func (w *webAuthnUser) WebAuthnDisplayName() string { return w.Username }
func (w *webAuthnUser) WebAuthnCredentials() []webauthnlib.Credential {
	var creds []webauthnlib.Credential
	var rawCreds []WebAuthnCredential
	err := w.aAgent.Database.Read("auth", fmt.Sprintf("webauthn/creds/%s", w.Username), &rawCreds)
	if err == nil {
		creds = make([]webauthnlib.Credential, len(rawCreds))
		for i, c := range rawCreds {
			creds[i] = c.toWebAuthnCredential()
		}
	}
	return creds
}

// newWebAuthnFromRequest creates a WebAuthn instance configured for the origin of the current request.
func newWebAuthnFromRequest(r *http.Request) (*webauthnlib.WebAuthn, error) {
	host := r.Host
	if fwdHost := r.Header.Get("X-Forwarded-Host"); fwdHost != "" {
		host = strings.Split(fwdHost, ",")[0]
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
		RPDisplayName: "Zoraxy",
		RPID:          rpID,
		RPOrigins:     []string{origin},
	})
}

// Constructor
func NewAuthenticationAgent(sessionName string, key []byte, sysdb *db.Database, allowReg bool, systemLogger *logger.Logger, loginRedirectionHandler func(http.ResponseWriter, *http.Request)) *AuthAgent {
	store := sessions.NewCookieStore(key)
	err := sysdb.NewTable("auth")
	if err != nil {
		systemLogger.Println("Failed to create auth database. Terminating.")
		panic(err)
	}

	//Create a new AuthAgent object
	newAuthAgent := AuthAgent{
		SessionName:             sessionName,
		SessionStore:            store,
		Database:                sysdb,
		LoginRedirectionHandler: loginRedirectionHandler,
		Logger:                  systemLogger,
	}

	//Return the authAgent
	return &newAuthAgent
}

func GetSessionKey(sysdb *db.Database, logger *logger.Logger) (string, error) {
	sysdb.NewTable("auth")
	sessionKey := ""
	if !sysdb.KeyExists("auth", "sessionkey") {
		key := make([]byte, 32)
		rand.Read(key)
		sessionKey = string(key)
		sysdb.Write("auth", "sessionkey", sessionKey)
		logger.PrintAndLog("auth", "New authentication session key generated", nil)
	} else {
		logger.PrintAndLog("auth", "Authentication session key loaded from database", nil)
		err := sysdb.Read("auth", "sessionkey", &sessionKey)
		if err != nil {
			return "", errors.New("database read error. Is the database file corrupted?")
		}
	}
	return sessionKey, nil
}

// This function will handle an http request and redirect to the given login address if not logged in
func (a *AuthAgent) HandleCheckAuth(w http.ResponseWriter, r *http.Request, handler func(http.ResponseWriter, *http.Request)) {
	if a.CheckAuth(r) {
		//User already logged in
		handler(w, r)
	} else {
		//User not logged in
		a.LoginRedirectionHandler(w, r)
	}
}

// Handle login request, require POST username and password
func (a *AuthAgent) HandleLogin(w http.ResponseWriter, r *http.Request) {

	//Get username from request using POST mode
	username, err := utils.PostPara(r, "username")
	if err != nil {
		//Username not defined
		a.Logger.PrintAndLog("auth", r.RemoteAddr+" trying to login with username: "+username, nil)
		utils.SendErrorResponse(w, "Username not defined or empty.")
		return
	}

	//Get password from request using POST mode
	password, err := utils.PostPara(r, "password")
	if err != nil {
		//Password not defined
		utils.SendErrorResponse(w, "Password not defined or empty.")
		return
	}

	//Get rememberme settings
	rememberme := false
	rmbme, _ := utils.PostPara(r, "rmbme")
	if rmbme == "true" {
		rememberme = true
	}

	//Check the database and see if this user is in the database
	passwordCorrect, rejectionReason := a.ValidateUsernameAndPasswordWithReason(username, password)
	//The database contain this user information. Check its password if it is correct
	if passwordCorrect {
		//Password correct
		if a.IsTOTPEnabled(username) {
			// Set session data but keep authenticated=false while waiting for TOTP
			session := a.PrepareUserSession(w, r, username, rememberme)
			session.Values["totp_partial"] = true
			session.Values["authenticated"] = false
			session.Save(r, w)

			utils.SendJSONResponse(w, `{"totp_required":true,"username":"`+username+`"}`)
			a.Logger.PrintAndLog("auth", username+" password verified, waiting for TOTP code.", nil)
			return
		}

		// No TOTP - fully authenticate
		session := a.PrepareUserSession(w, r, username, rememberme)
		session.Values["authenticated"] = true
		session.Save(r, w)

		a.Logger.PrintAndLog("auth", username+" logged in.", nil)
		utils.SendOK(w)
	} else {
		//Password incorrect
		a.Logger.PrintAndLog("auth", username+" login request rejected: "+rejectionReason, nil)

		utils.SendErrorResponse(w, rejectionReason)
		return
	}
}

func (a *AuthAgent) ValidateUsernameAndPassword(username string, password string) bool {
	succ, _ := a.ValidateUsernameAndPasswordWithReason(username, password)
	return succ
}

// validate the username and password, return reasons if the auth failed
func (a *AuthAgent) ValidateUsernameAndPasswordWithReason(username string, password string) (bool, string) {
	hashedPassword := Hash(password)
	var passwordInDB string
	err := a.Database.Read("auth", "passhash/"+username, &passwordInDB)
	if err != nil {
		//User not found or db exception
		a.Logger.PrintAndLog("auth", username+" login with incorrect password", nil)
		return false, "Invalid username or password"
	}

	if passwordInDB == hashedPassword {
		return true, ""
	} else {
		return false, "Invalid username or password"
	}
}

// Prepare valid session with user parameters. Caller is expected to commit the data with session.Save() call.
func (a *AuthAgent) PrepareUserSession(w http.ResponseWriter, r *http.Request, username string, rememberme bool) *sessions.Session {
	session, _ := a.SessionStore.Get(r, a.SessionName)

	session.Values["username"] = username
	session.Values["rememberMe"] = rememberme

	//Check if remember me is clicked. If yes, set the maxage to 1 week.
	if rememberme {
		session.Options = &sessions.Options{
			MaxAge: 3600 * 24 * 7, //One week
			Path:   "/",
		}
	} else {
		session.Options = &sessions.Options{
			MaxAge: 3600 * 1, //One hour
			Path:   "/",
		}
	}
	return session
}

// Handle logout, reply OK after logged out. WILL NOT DO REDIRECTION
func (a *AuthAgent) HandleLogout(w http.ResponseWriter, r *http.Request) {
	username, err := a.GetUserName(w, r)
	if err != nil {
		utils.SendErrorResponse(w, "user not logged in")
		return
	}
	if username != "" {
		a.Logger.PrintAndLog("auth", username+" logged out", nil)
	}
	// Revoke users authentication
	err = a.Logout(w, r)
	if err != nil {
		utils.SendErrorResponse(w, "Logout failed")
		return
	}

	utils.SendOK(w)
}

func (a *AuthAgent) Logout(w http.ResponseWriter, r *http.Request) error {
	session, err := a.SessionStore.Get(r, a.SessionName)
	if err != nil {
		return err
	}
	session.Values["authenticated"] = false
	session.Values["username"] = nil
	session.Values["totp_partial"] = false
	session.Options.MaxAge = -1
	return session.Save(r, w)
}

// Get the current session username from request
func (a *AuthAgent) GetUserName(w http.ResponseWriter, r *http.Request) (string, error) {
	if a.CheckAuth(r) {
		//This user has logged in.
		session, _ := a.SessionStore.Get(r, a.SessionName)
		return session.Values["username"].(string), nil
	} else {
		//This user has not logged in.
		return "", errors.New("user not logged in")
	}
}

// Get the current session user email from request
func (a *AuthAgent) GetUserEmail(w http.ResponseWriter, r *http.Request) (string, error) {
	if a.CheckAuth(r) {
		//This user has logged in.
		session, _ := a.SessionStore.Get(r, a.SessionName)
		username := session.Values["username"].(string)
		userEmail := ""
		err := a.Database.Read("auth", "email/"+username, &userEmail)
		if err != nil {
			return "", err
		}

		return userEmail, nil
	} else {
		//This user has not logged in.
		return "", errors.New("user not logged in")
	}
}

// Check if the user has logged in, return true / false in JSON
func (a *AuthAgent) CheckLogin(w http.ResponseWriter, r *http.Request) {
	if a.CheckAuth(r) {
		utils.SendJSONResponse(w, "true")
	} else {
		utils.SendJSONResponse(w, "false")
	}
}

// Handle new user register. Require POST username, password, group.
func (a *AuthAgent) HandleRegister(w http.ResponseWriter, r *http.Request, callback func(string, string)) {
	//Get username from request
	newusername, err := utils.PostPara(r, "username")
	if err != nil {
		utils.SendErrorResponse(w, "Missing 'username' paramter")
		return
	}

	//Get password from request
	password, err := utils.PostPara(r, "password")
	if err != nil {
		utils.SendErrorResponse(w, "Missing 'password' paramter")
		return
	}

	//Get email from request
	email, err := utils.PostPara(r, "email")
	if err != nil {
		utils.SendErrorResponse(w, "Missing 'email' paramter")
		return
	}

	_, err = mail.ParseAddress(email)
	if err != nil {
		utils.SendErrorResponse(w, "Invalid or malformed email")
		return
	}

	//Ok to proceed create this user
	err = a.CreateUserAccount(newusername, password, email)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Do callback if exists
	if callback != nil {
		callback(newusername, email)
	}

	//Return to the client with OK
	utils.SendOK(w)
	a.Logger.PrintAndLog("auth", "New user "+newusername+" added to system.", nil)
}

// Handle new user register without confirmation email. Require POST username, password, group.
func (a *AuthAgent) HandleRegisterWithoutEmail(w http.ResponseWriter, r *http.Request, callback func(string, string)) {
	//Get username from request
	newusername, err := utils.PostPara(r, "username")
	if err != nil {
		utils.SendErrorResponse(w, "Missing 'username' paramter")
		return
	}

	//Get password from request
	password, err := utils.PostPara(r, "password")
	if err != nil {
		utils.SendErrorResponse(w, "Missing 'password' paramter")
		return
	}

	//Ok to proceed create this user
	err = a.CreateUserAccount(newusername, password, "")
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Do callback if exists
	if callback != nil {
		callback(newusername, "")
	}

	//Return to the client with OK
	utils.SendOK(w)
	a.Logger.PrintAndLog("auth", "Admin account created: "+newusername, nil)
}

// Check authentication from request header's session value
func (a *AuthAgent) CheckAuth(r *http.Request) bool {
	session, err := a.SessionStore.Get(r, a.SessionName)
	if err != nil {
		return false
	}

	// Check if user is authenticated
	if auth, ok := session.Values["authenticated"].(bool); !ok || !auth {
		return false
	}

	return true
}

// CheckAuthOrPartial checks if user has at least partial authentication.
// As in "password verified but may still need to give valid TOTP"
func (a *AuthAgent) CheckAuthOrPartial(r *http.Request) bool {
	session, err := a.SessionStore.Get(r, a.SessionName)
	if err != nil {
		a.Logger.PrintAndLog("auth", "error during session reading.", err)
		return false
	}

	if auth, ok := session.Values["authenticated"].(bool); ok && auth {
		return true
	}
	if partial, ok := session.Values["totp_partial"].(bool); ok && partial {
		return true
	}

	return false
}

func (a *AuthAgent) IsTOTPEnabled(username string) bool {
	var enabled string
	err := a.Database.Read("auth", fmt.Sprintf("totp/enabled/%s", username), &enabled)
	if err != nil {
		a.Logger.PrintAndLog("auth", "error during database read.", err)
		return false
	}
	return enabled == "true"
}

func (a *AuthAgent) GenerateTOTP(username string) (secret string, qrCodeDataURL string, err error) {
	if a.IsTOTPEnabled(username) {
		return "", "", errors.New("TOTP is already enabled for this user")
	}

	// Generate a new TOTP key (includes secret + QR URL)
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "Zoraxy Cluster Gateway",
		AccountName: username,
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to generate TOTP key: %w", err)
	}

	secret = key.Secret()
	err = a.Database.Write("auth", fmt.Sprintf("totp/secret/%s", username), secret)
	if err != nil {
		return "", "", fmt.Errorf("failed to save TOTP secret: %w", err)
	}

	qr, err := key.Image(200, 200)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate QR image: %w", err)
	}

	// Encode as base64 data URL
	var buf bytes.Buffer
	pngEncoder := png.Encoder{
		CompressionLevel: png.BestCompression,
	}
	err = pngEncoder.Encode(&buf, qr)
	if err != nil {
		return "", "", fmt.Errorf("failed to encode QR image: %w", err)
	}

	qrCodeDataURL = "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())

	return secret, qrCodeDataURL, nil
}

func (a *AuthAgent) VerifyTOTPCode(username, code string) bool {
	var secret string
	err := a.Database.Read("auth", fmt.Sprintf("totp/secret/%s", username), &secret)
	if err != nil {
		a.Logger.PrintAndLog("auth", "error during database read.", err)
		return false
	}

	valid := totp.Validate(code, secret)
	return valid
}

// VerifyTOTPCodeAndEnable verifies a TOTP code and enables 2FA for the user
func (a *AuthAgent) VerifyTOTPCodeAndEnable(username, code string) (bool, error) {
	var secret string
	err := a.Database.Read("auth", fmt.Sprintf("totp/secret/%s", username), &secret)
	if err != nil {
		a.Logger.PrintAndLog("auth", "error during database read.", err)
		return false, errors.New("TOTP secret not found")
	}

	if !totp.Validate(code, secret) {
		return false, errors.New("invalid TOTP code")
	}

	err = a.Database.Write("auth", fmt.Sprintf("totp/enabled/%s", username), "true")
	if err != nil {
		a.Logger.PrintAndLog("auth", "error during database write.", err)
		return false, fmt.Errorf("failed to enable TOTP: %w", err)
	}

	return true, nil
}

func (a *AuthAgent) DisableTOTP(username string) error {
	if !a.IsTOTPEnabled(username) {
		return errors.New("TOTP is not enabled for this user")
	}

	err := a.Database.Write("auth", fmt.Sprintf("totp/enabled/%s", username), "false")
	if err != nil {
		return fmt.Errorf("failed to disable TOTP: %w", err)
	}

	// Also clear the secret if it exists
	err = a.Database.Delete("auth", fmt.Sprintf("totp/secret/%s", username))
	if err != nil {
		// TOTP is technically disabled at this point.
		// Leftover secret should not cause any trouble, so removing in on a best-effort basis.
		a.Logger.PrintAndLog("auth", "error during database write.", err)
	}

	return nil
}

// UpdateTOTPSession marks the session as fully authenticated after TOTP verification
func (a *AuthAgent) UpdateTOTPSession(w http.ResponseWriter, r *http.Request, username string) error {
	session, err := a.SessionStore.Get(r, a.SessionName)
	if err != nil {
		return err
	}

	session.Values["authenticated"] = true
	session.Values["username"] = username

	// Preserve remember me setting
	rememberme := session.Values["rememberMe"].(bool)
	if rememberme {
		session.Options = &sessions.Options{
			MaxAge: 3600 * 24 * 7,
			Path:   "/",
		}
	} else {
		session.Options = &sessions.Options{
			MaxAge: 3600 * 1,
			Path:   "/",
		}
	}

	return session.Save(r, w)
}

// HandleTOTPStatus returns the current TOTP status for the logged-in user
func (a *AuthAgent) HandleTOTPStatus(w http.ResponseWriter, r *http.Request) {
	username, err := a.GetUserName(w, r)
	if err != nil {
		utils.SendErrorResponse(w, "Login required")
		return
	}

	enabled := a.IsTOTPEnabled(username)
	status := "disabled"
	if enabled {
		status = "enabled"
	}
	utils.SendJSONResponse(w, `{"status":"`+status+`"}`)
}

// HandleTOTPGenerate generates a new TOTP secret and QR code for the logged-in user
func (a *AuthAgent) HandleTOTPGenerate(w http.ResponseWriter, r *http.Request) {
	username, err := a.GetUserName(w, r)
	if err != nil {
		utils.SendErrorResponse(w, "Login required")
		return
	}

	secret, qrCode, err := a.GenerateTOTP(username)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendJSONResponse(w, `{"secret":"`+secret+`","qrCode":"`+qrCode+`","message":"Scan the QR code with your authenticator app, then enter the code to enable 2FA"}`)
}

// HandleTOTPVerify verifies the TOTP code during setup and enables 2FA
func (a *AuthAgent) HandleTOTPVerify(w http.ResponseWriter, r *http.Request) {
	username, err := a.GetUserName(w, r)
	if err != nil {
		utils.SendErrorResponse(w, "Login required")
		return
	}

	code, err := utils.PostPara(r, "code")
	if err != nil {
		utils.SendErrorResponse(w, "Verification code required")
		return
	}

	success, err := a.VerifyTOTPCodeAndEnable(username, code)
	if err != nil || !success {
		utils.SendErrorResponse(w, "Invalid verification code")
		return
	}

	utils.SendOK(w)
}

// HandleTOTPDisable disables TOTP for the logged-in user. Requires password + current TOTP code.
func (a *AuthAgent) HandleTOTPDisable(w http.ResponseWriter, r *http.Request) {
	username, err := a.GetUserName(w, r)
	if err != nil {
		utils.SendErrorResponse(w, "Login required")
		return
	}

	if !a.IsTOTPEnabled(username) {
		utils.SendErrorResponse(w, "TOTP is not enabled")
		return
	}

	password, err := utils.PostPara(r, "password")
	if err != nil {
		utils.SendErrorResponse(w, "Password confirmation required")
		return
	}

	code, err := utils.PostPara(r, "totpCode")
	if err != nil {
		utils.SendErrorResponse(w, "TOTP code required")
		return
	}

	passwordCorrect, _ := a.ValidateUsernameAndPasswordWithReason(username, password)
	if !passwordCorrect {
		utils.SendErrorResponse(w, "Invalid password")
		return
	}

	if !a.VerifyTOTPCode(username, code) {
		utils.SendErrorResponse(w, "Invalid TOTP code")
		return
	}

	err = a.DisableTOTP(username)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

// HandleTOTPVerifyCode handles TOTP verification during login step 2.
func (a *AuthAgent) HandleTOTPVerifyCode(w http.ResponseWriter, r *http.Request) {
	// Check partial auth
	if !a.CheckAuthOrPartial(r) {
		utils.SendErrorResponse(w, "Authentication required")
		return
	}

	session, _ := a.SessionStore.Get(r, a.SessionName)
	username := session.Values["username"].(string)

	code, err := utils.PostPara(r, "code")
	if err != nil {
		utils.SendErrorResponse(w, "Verification code required")
		return
	}

	if !a.VerifyTOTPCode(username, code) {
		a.Logger.PrintAndLog("auth", username+" gave invalid TOTP verification code", err)
		utils.SendErrorResponse(w, "Invalid verification code")
		return
	}

	err = a.UpdateTOTPSession(w, r, username)
	if err != nil {
		utils.SendErrorResponse(w, "Failed to complete authentication")
		return
	}

	a.Logger.PrintAndLog("auth", username+" completed 2FA verification.", nil)
	utils.SendOK(w)
}

// Handle de-register of users. Require POST username.
// THIS FUNCTION WILL NOT CHECK FOR PERMISSION. PLEASE USE WITH PERMISSION HANDLER
func (a *AuthAgent) HandleUnregister(w http.ResponseWriter, r *http.Request) {
	//Check if the user is logged in
	if !a.CheckAuth(r) {
		//This user has not logged in
		utils.SendErrorResponse(w, "Login required to remove user from the system.")
		return
	}

	//Get username from request
	username, err := utils.PostPara(r, "username")
	if err != nil {
		utils.SendErrorResponse(w, "Missing 'username' paramter")
		return
	}

	err = a.UnregisterUser(username)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Return to the client with OK
	utils.SendOK(w)
	a.Logger.PrintAndLog("auth", "User "+username+" has been removed from the system", nil)
}

func (a *AuthAgent) UnregisterUser(username string) error {
	//Check if the user exists in the system database.
	if !a.Database.KeyExists("auth", "passhash/"+username) {
		//This user do not exists.
		return errors.New("this user does not exists")
	}

	//OK! Remove the user from the database
	a.Database.Delete("auth", "passhash/"+username)
	a.Database.Delete("auth", "email/"+username)
	a.Database.Delete("auth", "totp/secret/"+username)
	a.Database.Delete("auth", "totp/enabled/"+username)
	a.Database.Delete("auth", fmt.Sprintf("webauthn/creds/%s", username))
	return nil
}

// Get the number of users in the system
func (a *AuthAgent) GetUserCounts() int {
	entries, _ := a.Database.ListTable("auth")
	usercount := 0
	for _, keypairs := range entries {
		if strings.Contains(string(keypairs[0]), "passhash/") {
			//This is a user registry
			usercount++
		}
	}

	if usercount == 0 {
		a.Logger.PrintAndLog("auth", "There are no user in the database", nil)
	}
	return usercount
}

// List all username within the system
func (a *AuthAgent) ListUsers() []string {
	entries, _ := a.Database.ListTable("auth")
	results := []string{}
	for _, keypairs := range entries {
		if strings.Contains(string(keypairs[0]), "passhash/") {
			username := strings.Split(string(keypairs[0]), "/")[1]
			results = append(results, username)
		}
	}
	return results
}

// Check if the given username exists
func (a *AuthAgent) UserExists(username string) bool {
	userpasswordhash := ""
	err := a.Database.Read("auth", "passhash/"+username, &userpasswordhash)
	if err != nil || userpasswordhash == "" {
		return false
	}
	return true
}

// Update the session expire time given the request header.
func (a *AuthAgent) UpdateSessionExpireTime(w http.ResponseWriter, r *http.Request) bool {
	session, _ := a.SessionStore.Get(r, a.SessionName)
	if session.Values["authenticated"].(bool) {
		//User authenticated. Extend its expire time
		rememberme := session.Values["rememberMe"].(bool)
		//Extend the session expire time
		if rememberme {
			session.Options = &sessions.Options{
				MaxAge: 3600 * 24 * 7, //One week
				Path:   "/",
			}
		} else {
			session.Options = &sessions.Options{
				MaxAge: 3600 * 1, //One hour
				Path:   "/",
			}
		}
		session.Save(r, w)
		return true
	} else {
		return false
	}
}

// Create user account
func (a *AuthAgent) CreateUserAccount(newusername string, password string, email string) error {
	//Check user already exists
	if a.UserExists(newusername) {
		return errors.New("user with same name already exists")
	}

	key := newusername
	hashedPassword := Hash(password)
	err := a.Database.Write("auth", "passhash/"+key, hashedPassword)
	if err != nil {
		return err
	}

	if email != "" {
		err = a.Database.Write("auth", "email/"+key, email)
		if err != nil {
			return err
		}
	}

	err = a.Database.Write("auth", "totp/enabled/"+key, "false")
	if err != nil {
		return err
	}

	// Initialize empty WebAuthn credentials array
	err = a.Database.Write("auth", fmt.Sprintf("webauthn/creds/%s", key), []WebAuthnCredential{})
	if err != nil {
		return err
	}

	return nil
}

// Hash the given raw string into sha512 hash
func Hash(raw string) string {
	h := sha512.New()
	h.Write([]byte(raw))
	return hex.EncodeToString(h.Sum(nil))
}

// ─── WebAuthn Handlers ─────────────────────────────────────────────────────

// HandleWebAuthnRegisterBegin starts the WebAuthn registration process for the logged-in user.
func (a *AuthAgent) HandleWebAuthnRegisterBegin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "method not allowed")
		return
	}

	username, err := a.GetUserName(w, r)
	if err != nil {
		utils.SendErrorResponse(w, "Login required")
		return
	}

	wa, err := newWebAuthnFromRequest(r)
	if err != nil {
		utils.SendErrorResponse(w, "webauthn init failed: "+err.Error())
		return
	}

	user := &webAuthnUser{Username: username}
	options, sessionData, err := wa.BeginRegistration(user)
	if err != nil {
		utils.SendErrorResponse(w, "begin registration failed: "+err.Error())
		return
	}

	// Store pending registration with a short expiry
	token := generateSessionToken()
	// Serialize session data to JSON bytes for reliable database storage
	sessionJSON, err := serializeSessionData(sessionData)
	if err != nil {
		a.Logger.PrintAndLog("auth", "failed to serialize webauthn session data", err)
		utils.SendErrorResponse(w, "failed to prepare registration: "+err.Error())
		return
	}
	a.Database.Write("auth", fmt.Sprintf("webauthn/pending_reg/%s", token), map[string]any{
		"username":    username,
		"sessionData": sessionJSON,
		"expiry":      time.Now().Add(5 * time.Minute).Format(time.RFC3339),
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"token":   token,
		"options": options,
	})
}

// HandleWebAuthnRegisterComplete completes WebAuthn registration after the user's browser responds.
func (a *AuthAgent) HandleWebAuthnRegisterComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "method not allowed")
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		utils.SendErrorResponse(w, "missing token parameter")
		return
	}

	// Load and verify pending registration
	var pendingData map[string]any
	err := a.Database.Read("auth", fmt.Sprintf("webauthn/pending_reg/%s", token), &pendingData)
	if err != nil {
		utils.SendErrorResponse(w, "registration session not found or expired")
		return
	}

	expiryStr, ok := pendingData["expiry"].(string)
	if !ok {
		a.Database.Delete("auth", fmt.Sprintf("webauthn/pending_reg/%s", token))
		utils.SendErrorResponse(w, "registration session expired")
		return
	}
	expiryTime, err := time.Parse(time.RFC3339, expiryStr)
	if err != nil || time.Now().After(expiryTime) {
		a.Database.Delete("auth", fmt.Sprintf("webauthn/pending_reg/%s", token))
		utils.SendErrorResponse(w, "registration session expired")
		return
	}

	username, ok := pendingData["username"].(string)
	if !ok {
		utils.SendErrorResponse(w, "invalid registration session")
		return
	}

	wa, err := newWebAuthnFromRequest(r)
	if err != nil {
		utils.SendErrorResponse(w, "webauthn init failed")
		return
	}

	// Parse the stored session data
	sessionDataBytes, err := base64.StdEncoding.DecodeString(pendingData["sessionData"].(string))
	if err != nil {
		utils.SendErrorResponse(w, "session data decode error")
		return
	}
	if len(sessionDataBytes) == 0 {
		utils.SendErrorResponse(w, "invalid session data")
		return
	}
	sessionData, err := deserializeSessionData(sessionDataBytes)
	if err != nil {
		a.Logger.PrintAndLog("auth", "failed to deserialize webauthn session data", err)
		utils.SendErrorResponse(w, "session data parse error: "+err.Error())
		return
	}

	user := &webAuthnUser{Username: username}

	// The frontend sends the credential as JSON with base64url-encoded fields.
	// Parse it, decode the fields, then verify using the webauthn library.
	var credPayload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&credPayload); err != nil {
		utils.SendErrorResponse(w, "failed to parse credential data")
		return
	}

	// Decode base64url fields to raw bytes
	decodeBase64url := func(val any) ([]byte, error) {
		s, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("expected string, got %T", val)
		}
		// Replace URL-safe characters with standard base64
		s = strings.ReplaceAll(s, "-", "+")
		s = strings.ReplaceAll(s, "_", "/")
		// Add padding if needed
		if rem := len(s) % 4; rem != 0 {
			s += strings.Repeat("=", 4-rem)
		}
		return base64.StdEncoding.DecodeString(s)
	}

	rawId, err := decodeBase64url(credPayload["rawId"])
	if err != nil {
		utils.SendErrorResponse(w, "invalid rawId in credential")
		return
	}
	clientDataJSON, err := decodeBase64url(credPayload["response"].(map[string]any)["clientDataJSON"])
	if err != nil {
		utils.SendErrorResponse(w, "invalid clientDataJSON")
		return
	}
	attestationObject, err := decodeBase64url(credPayload["response"].(map[string]any)["attestationObject"])
	if err != nil {
		utils.SendErrorResponse(w, "invalid attestationObject")
		return
	}

	// Reconstruct the raw WebAuthn response for parsing.
	// Use RawURLEncoding so that the go-webauthn library's URLEncodedBase64.UnmarshalJSON
	// (which uses base64.RawURLEncoding.Decode internally) can parse the fields.
	// Also derive "id" from rawId when the frontend sends an empty id (common for platform authenticators).
	id := credPayload["id"]
	if idStr, _ := id.(string); idStr == "" {
		id = base64.RawURLEncoding.EncodeToString(rawId)
	}
	rawResponse := map[string]any{
		"id":    id,
		"rawId": base64.RawURLEncoding.EncodeToString(rawId),
		"type":  "public-key",
		"response": map[string]any{
			"clientDataJSON":    base64.RawURLEncoding.EncodeToString(clientDataJSON),
			"attestationObject": base64.RawURLEncoding.EncodeToString(attestationObject),
		},
	}

	// Build a fake request with the raw response as body so protocol.ParseCredentialCreationResponse can parse it
	rawJSON, _ := json.Marshal(rawResponse)
	reqBody := bytes.NewReader(rawJSON)
	fakeReq := &http.Request{
		Body:   io.NopCloser(reqBody),
		Header: http.Header{},
	}

	parsedResponse, err := protocol.ParseCredentialCreationResponse(fakeReq)
	if err != nil {
		a.Logger.PrintAndLog("auth", "failed to parse credential creation response", err)
		utils.SendErrorResponse(w, "registration verification failed: "+err.Error())
		return
	}

	credential, err := wa.CreateCredential(user, *sessionData, parsedResponse)
	if err != nil {
		a.Logger.PrintAndLog("auth", "webauthn create credential failed", err)
		utils.SendErrorResponse(w, "registration verification failed: "+err.Error())
		return
	}

	// Clean up pending registration
	a.Database.Delete("auth", fmt.Sprintf("webauthn/pending_reg/%s", token))

	// Parse credential name if provided
	credName := strings.TrimSpace(r.URL.Query().Get("name"))
	if credName == "" {
		credName = fmt.Sprintf("Passkey %d", a.countUserWebAuthnCredentials(username)+1)
	}

	// Store transports
	transports := make([]string, len(credential.Transport))
	for i, t := range credential.Transport {
		transports[i] = string(t)
	}

	// Create credential record
	newCred := WebAuthnCredential{
		ID:             credential.ID,
		PublicKey:      credential.PublicKey,
		AAGUID:         credential.Authenticator.AAGUID,
		SignCount:      credential.Authenticator.SignCount,
		Transports:     transports,
		BackupEligible: credential.Flags.BackupEligible,
		BackupState:    credential.Flags.BackupState,
		Name:           credName,
		CreatedAt:      time.Now().Unix(),
		LastUsedAt:     time.Now().Unix(),
	}

	// Load existing credentials, add new one, and save
	var existingCreds []WebAuthnCredential
	err = a.Database.Read("auth", fmt.Sprintf("webauthn/creds/%s", username), &existingCreds)
	if err != nil && err.Error() != "key not found" {
		// If the key doesn't exist, start with empty slice
		existingCreds = []WebAuthnCredential{}
	}
	existingCreds = append(existingCreds, newCred)

	err = a.Database.Write("auth", fmt.Sprintf("webauthn/creds/%s", username), existingCreds)
	if err != nil {
		utils.SendErrorResponse(w, "failed to save credential: "+err.Error())
		return
	}

	a.Logger.PrintAndLog("auth", fmt.Sprintf("User %s registered WebAuthn passkey: %s", username, credName), nil)
	utils.SendOK(w)
}

// HandleWebAuthnList returns the list of registered WebAuthn passkeys for the logged-in user.
func (a *AuthAgent) HandleWebAuthnList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.SendErrorResponse(w, "method not allowed")
		return
	}

	username, err := a.GetUserName(w, r)
	if err != nil {
		utils.SendErrorResponse(w, "Login required")
		return
	}

	var credentials []WebAuthnCredential
	err = a.Database.Read("auth", fmt.Sprintf("webauthn/creds/%s", username), &credentials)
	if err != nil {
		credentials = []WebAuthnCredential{}
	}

	type passkeyInfo struct {
		ID         string   `json:"id"`
		Name       string   `json:"name"`
		CreatedAt  int64    `json:"createdAt"`
		LastUsedAt int64    `json:"lastUsedAt"`
		Transports []string `json:"transports"`
		BackedUp   bool     `json:"backedUp"`
	}

	result := make([]passkeyInfo, len(credentials))
	for i, c := range credentials {
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

// HandleWebAuthnRemove removes a specific WebAuthn passkey credential.
func (a *AuthAgent) HandleWebAuthnRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "method not allowed")
		return
	}

	username, err := a.GetUserName(w, r)
	if err != nil {
		utils.SendErrorResponse(w, "Login required")
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

	var credentials []WebAuthnCredential
	err = a.Database.Read("auth", fmt.Sprintf("webauthn/creds/%s", username), &credentials)
	if err != nil {
		utils.SendErrorResponse(w, "no credentials found")
		return
	}

	var updated []WebAuthnCredential
	removed := false
	for _, c := range credentials {
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

	err = a.Database.Write("auth", fmt.Sprintf("webauthn/creds/%s", username), updated)
	if err != nil {
		utils.SendErrorResponse(w, "failed to remove credential: "+err.Error())
		return
	}

	a.Logger.PrintAndLog("auth", fmt.Sprintf("User %s removed WebAuthn passkey", username), nil)
	utils.SendOK(w)
}

// HandleWebAuthnAuthBegin starts a discoverable-credential WebAuthn authentication flow.
func (a *AuthAgent) HandleWebAuthnAuthBegin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "method not allowed")
		return
	}

	wa, err := newWebAuthnFromRequest(r)
	if err != nil {
		utils.SendErrorResponse(w, "webauthn init failed: "+err.Error())
		return
	}

	// Use discoverable credentials (no username required)
	options, sessionData, err := wa.BeginDiscoverableLogin()
	if err != nil {
		utils.SendErrorResponse(w, "begin passkey login failed: "+err.Error())
		return
	}

	// Store pending authentication
	token := generateSessionToken()
	// Serialize session data to JSON bytes for reliable database storage
	sessionJSON, err := serializeSessionData(sessionData)
	if err != nil {
		a.Logger.PrintAndLog("auth", "failed to serialize webauthn session data", err)
		utils.SendErrorResponse(w, "failed to prepare auth: "+err.Error())
		return
	}
	a.Database.Write("auth", fmt.Sprintf("webauthn/pending_auth/%s", token), map[string]any{
		"sessionData": sessionJSON,
		"expiry":      time.Now().Add(5 * time.Minute).Format(time.RFC3339),
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"token":   token,
		"options": options,
	})
}

// pendingAuthSession holds a deserialized WebAuthn authentication session.
type pendingAuthSession struct {
	webauthnlib.SessionData
	expiry time.Time
}

// loadPendingAuthSession loads and validates a pending auth session from the database.
// Returns nil (without error) when the token is missing or expired.
func (a *AuthAgent) loadPendingAuthSession(token string) (*pendingAuthSession, error) {
	var pendingData map[string]any
	if err := a.Database.Read("auth", fmt.Sprintf("webauthn/pending_auth/%s", token), &pendingData); err != nil {
		return nil, err
	}

	expiryStr, ok := pendingData["expiry"].(string)
	if !ok {
		a.Database.Delete("auth", fmt.Sprintf("webauthn/pending_auth/%s", token))
		return nil, errors.New("session expired")
	}
	expiryTime, err := time.Parse(time.RFC3339, expiryStr)
	if err != nil || time.Now().After(expiryTime) {
		a.Database.Delete("auth", fmt.Sprintf("webauthn/pending_auth/%s", token))
		return nil, errors.New("session expired")
	}

	var sessionDataBytes []byte
	sessionDataBytes, err = base64.StdEncoding.DecodeString(pendingData["sessionData"].(string))
	if err != nil {
		return nil, errors.New("session data decode error")
	}
	if len(sessionDataBytes) == 0 {
		return nil, errors.New("invalid session data")
	}

	sd, err := deserializeSessionData(sessionDataBytes)
	if err != nil {
		a.Logger.PrintAndLog("auth", "failed to deserialize webauthn session data", err)
		return nil, err
	}

	return &pendingAuthSession{SessionData: *sd, expiry: expiryTime}, nil
}

// buildAssertionRequest reconstructs a WebAuthn assertion payload into an http.Request
// suitable for protocol.ParseCredentialRequestResponse.
func buildAssertionRequest(id, rawId, clientDataJSON, authenticatorData, signature []byte, userHandle []byte) *http.Request {
	resp := map[string]any{
		"clientDataJSON":    base64.RawURLEncoding.EncodeToString(clientDataJSON),
		"authenticatorData": base64.RawURLEncoding.EncodeToString(authenticatorData),
		"signature":         base64.RawURLEncoding.EncodeToString(signature),
	}
	if userHandle != nil {
		resp["userHandle"] = base64.RawURLEncoding.EncodeToString(userHandle)
	}

	payload := map[string]any{
		"id":       string(id),
		"rawId":    base64.RawURLEncoding.EncodeToString(rawId),
		"type":     "public-key",
		"response": resp,
	}

	rawJSON, _ := json.Marshal(payload)
	return &http.Request{
		Body:   io.NopCloser(bytes.NewReader(rawJSON)),
		Header: http.Header{},
	}
}

// HandleWebAuthnAuthComplete completes WebAuthn authentication after the user's browser responds.
func (a *AuthAgent) HandleWebAuthnAuthComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "method not allowed")
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		utils.SendErrorResponse(w, "missing token parameter")
		return
	}

	// First, check if auth session exists
	session, err := a.loadPendingAuthSession(token)
	if err != nil {
		utils.SendErrorResponse(w, "no valid auth session found")
		return
	}

	// If it does, setup WebAuthn and decode the assertion payload
	wa, err := newWebAuthnFromRequest(r)
	if err != nil {
		utils.SendErrorResponse(w, "webauthn init failed")
		return
	}

	var assertPayload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&assertPayload); err != nil {
		utils.SendErrorResponse(w, "failed to parse assertion data")
		return
	}

	// Base64 trickery in the decodeBase64url is due to how browser part encodes the payload
	// Error are omitted here as the actual contents will be validated via protocol.ParseCredentialRequestResponse
	resp := assertPayload["response"].(map[string]any)
	rawId, _ := a.decodeBase64url(assertPayload["rawId"])
	clientDataJSON, _ := a.decodeBase64url(resp["clientDataJSON"])
	authenticatorData, _ := a.decodeBase64url(resp["authenticatorData"])
	signature, _ := a.decodeBase64url(resp["signature"])
	var userHandle []byte
	if uh := resp["userHandle"]; uh != nil {
		userHandle, _ = a.decodeBase64url(uh)
	}

	id := assertPayload["id"].(string)
	if id == "" {
		id = base64.RawURLEncoding.EncodeToString(rawId)
	}

	parsedAssertion, err := protocol.ParseCredentialRequestResponse(buildAssertionRequest(
		[]byte(id), rawId, clientDataJSON, authenticatorData, signature, userHandle,
	))
	if err != nil {
		a.Logger.PrintAndLog("auth", "failed to parse credential assertion response", err)
		utils.SendErrorResponse(w, "auth verification failed: "+err.Error())
		return
	}

	// Finally, validate the passkey and user data
	foundUsername := ""
	_, credential, err := wa.ValidatePasskeyLogin(func(rawID, userHandle []byte) (webauthnlib.User, error) {
		if len(userHandle) > 0 {
			foundUsername = string(userHandle)
			if !a.UserExists(foundUsername) {
				return nil, errors.New("user not found")
			}
			return &webAuthnUser{aAgent: a, Username: foundUsername}, nil
		}
		return nil, errors.New("passkey not recognized")
	}, session.SessionData, parsedAssertion)
	if err != nil {
		a.Database.Delete("auth", fmt.Sprintf("webauthn/pending_auth/%s", token))
		w.WriteHeader(http.StatusUnauthorized)
		utils.SendJSONResponse(w, `{"success": false, "error": "Passkey verification failed"}`)
		return
	}

	// Clean up and update session
	a.Database.Delete("auth", fmt.Sprintf("webauthn/pending_auth/%s", token))
	if foundUsername == "" {
		utils.SendErrorResponse(w, "User not found")
		return
	}

	var credentials []WebAuthnCredential
	a.Database.Read("auth", fmt.Sprintf("webauthn/creds/%s", foundUsername), &credentials)
	for i, c := range credentials {
		if bytes.Equal(c.ID, credential.ID) {
			credentials[i].SignCount = credential.Authenticator.SignCount
			credentials[i].BackupState = credential.Flags.BackupState
			credentials[i].LastUsedAt = time.Now().Unix()
			break
		}
	}
	if len(credentials) > 0 {
		a.Database.Write("auth", fmt.Sprintf("webauthn/creds/%s", foundUsername), credentials)
	}

	// Forcing "remember me" toggle to off here.
	// WebAuthn authentication with discoverable credentials does not really create much of a hassle
	// if it makes you relogin sometimes since the process really only requires you to tap your passkey/scanner.
	// Reducing the lifetime of a session increases security, tho.
	sess := a.PrepareUserSession(w, r, foundUsername, false)
	sess.Values["authenticated"] = true
	if err := sess.Save(r, w); err != nil {
		utils.SendErrorResponse(w, "Failed to complete authentication")
		return
	}

	a.Logger.PrintAndLog("auth", fmt.Sprintf("User %s logged in via WebAuthn passkey", foundUsername), nil)
	utils.SendJSONResponse(w, `{"success":true,"username":"`+foundUsername+`"}`)
}

// decodeBase64url decodes a base64url-encoded string to bytes.
func (a *AuthAgent) decodeBase64url(val any) ([]byte, error) {
	s, ok := val.(string)
	if !ok || s == "" {
		return []byte{}, nil
	}
	s = strings.ReplaceAll(s, "-", "+")
	s = strings.ReplaceAll(s, "_", "/")
	if rem := len(s) % 4; rem != 0 {
		s += strings.Repeat("=", 4-rem)
	}
	return base64.StdEncoding.DecodeString(s)
}

// countUserWebAuthnCredentials returns the number of WebAuthn credentials for a user.
func (a *AuthAgent) countUserWebAuthnCredentials(username string) int {
	var credentials []WebAuthnCredential
	err := a.Database.Read("auth", fmt.Sprintf("webauthn/creds/%s", username), &credentials)
	if err != nil {
		return 0
	}
	return len(credentials)
}

// generateSessionToken generates a random session token for WebAuthn pending state.
func generateSessionToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}
