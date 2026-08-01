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
	"errors"
	"fmt"
	"image/png"
	"net/http"
	"net/mail"
	"strings"

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

	return nil
}

// Hash the given raw string into sha512 hash
func Hash(raw string) string {
	h := sha512.New()
	h.Write([]byte(raw))
	return hex.EncodeToString(h.Sum(nil))
}
