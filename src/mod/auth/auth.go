package auth

/*

	author: tobychui
*/

import (
	"crypto/rand"
	"crypto/sha512"
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"encoding/hex"
	"log"

	"github.com/gorilla/sessions"
	db "imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/utils"
)

type AuthAgent struct {
	//Session related
	SessionName             string
	SessionStore            *sessions.CookieStore
	Database                *db.Database
	LoginRedirectionHandler func(http.ResponseWriter, *http.Request)
}

type AuthEndpoints struct {
	Login         string
	Logout        string
	Register      string
	CheckLoggedIn string
	Autologin     string
}

//Constructor
func NewAuthenticationAgent(sessionName string, key []byte, sysdb *db.Database, allowReg bool, loginRedirectionHandler func(http.ResponseWriter, *http.Request)) *AuthAgent {
	store := sessions.NewCookieStore(key)
	err := sysdb.NewTable("auth")
	if err != nil {
		log.Println("Failed to create auth database. Terminating.")
		panic(err)
	}

	//Create a new AuthAgent object
	newAuthAgent := AuthAgent{
		SessionName:             sessionName,
		SessionStore:            store,
		Database:                sysdb,
		LoginRedirectionHandler: loginRedirectionHandler,
	}

	//Return the authAgent
	return &newAuthAgent
}

func GetSessionKey(sysdb *db.Database) (string, error) {
	sysdb.NewTable("auth")
	sessionKey := ""
	if !sysdb.KeyExists("auth", "sessionkey") {
		key := make([]byte, 32)
		rand.Read(key)
		sessionKey = string(key)
		sysdb.Write("auth", "sessionkey", sessionKey)
		log.Println("[Auth] New authentication session key generated")
	} else {
		log.Println("[Auth] Authentication session key loaded from database")
		err := sysdb.Read("auth", "sessionkey", &sessionKey)
		if err != nil {
			return "", errors.New("database read error. Is the database file corrupted?")
		}
	}
	return sessionKey, nil
}

//This function will handle an http request and redirect to the given login address if not logged in
func (a *AuthAgent) HandleCheckAuth(w http.ResponseWriter, r *http.Request, handler func(http.ResponseWriter, *http.Request)) {
	if a.CheckAuth(r) {
		//User already logged in
		handler(w, r)
	} else {
		//User not logged in
		a.LoginRedirectionHandler(w, r)
	}
}

//Handle login request, require POST username and password
func (a *AuthAgent) HandleLogin(w http.ResponseWriter, r *http.Request) {

	//Get username from request using POST mode
	username, err := utils.PostPara(r, "username")
	if err != nil {
		//Username not defined
		log.Println("[Auth] " + r.RemoteAddr + " trying to login with username: " + username)
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
		// Set user as authenticated
		a.LoginUserByRequest(w, r, username, rememberme)

		//Print the login message to console
		log.Println(username + " logged in.")
		utils.SendOK(w)
	} else {
		//Password incorrect
		log.Println(username + " login request rejected: " + rejectionReason)

		utils.SendErrorResponse(w, rejectionReason)
		return
	}
}

func (a *AuthAgent) ValidateUsernameAndPassword(username string, password string) bool {
	succ, _ := a.ValidateUsernameAndPasswordWithReason(username, password)
	return succ
}

//validate the username and password, return reasons if the auth failed
func (a *AuthAgent) ValidateUsernameAndPasswordWithReason(username string, password string) (bool, string) {
	hashedPassword := Hash(password)
	var passwordInDB string
	err := a.Database.Read("auth", "passhash/"+username, &passwordInDB)
	if err != nil {
		//User not found or db exception
		log.Println("[Auth] " + username + " login with incorrect password")
		return false, "Invalid username or password"
	}

	if passwordInDB == hashedPassword {
		return true, ""
	} else {
		return false, "Invalid username or password"
	}
}

//Login the user by creating a valid session for this user
func (a *AuthAgent) LoginUserByRequest(w http.ResponseWriter, r *http.Request, username string, rememberme bool) {
	session, _ := a.SessionStore.Get(r, a.SessionName)

	session.Values["authenticated"] = true
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
	session.Save(r, w)
}

//Handle logout, reply OK after logged out. WILL NOT DO REDIRECTION
func (a *AuthAgent) HandleLogout(w http.ResponseWriter, r *http.Request) {
	username, err := a.GetUserName(w, r)
	if username != "" {
		log.Println(username + " logged out.")
	}
	// Revoke users authentication
	err = a.Logout(w, r)
	if err != nil {
		utils.SendErrorResponse(w, "Logout failed")
		return
	}

	w.Write([]byte("OK"))
}

func (a *AuthAgent) Logout(w http.ResponseWriter, r *http.Request) error {
	session, err := a.SessionStore.Get(r, a.SessionName)
	if err != nil {
		return err
	}
	session.Values["authenticated"] = false
	session.Values["username"] = nil
	session.Save(r, w)
	return nil
}

//Get the current session username from request
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

//Get the current session user email from request
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

//Check if the user has logged in, return true / false in JSON
func (a *AuthAgent) CheckLogin(w http.ResponseWriter, r *http.Request) {
	if a.CheckAuth(r) {
		utils.SendJSONResponse(w, "true")
	} else {
		utils.SendJSONResponse(w, "false")
	}
}

//Handle new user register. Require POST username, password, group.
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
	log.Println("[Auth] New user " + newusername + " added to system.")
}

//Handle new user register without confirmation email. Require POST username, password, group.
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
	log.Println("[Auth] Admin account created: " + newusername)
}

//Check authentication from request header's session value
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

//Handle de-register of users. Require POST username.
//THIS FUNCTION WILL NOT CHECK FOR PERMISSION. PLEASE USE WITH PERMISSION HANDLER
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
	log.Println("[Auth] User " + username + " has been removed from the system.")
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
	return nil
}

//Get the number of users in the system
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
		log.Println("There are no user in the database.")
	}
	return usercount
}

//List all username within the system
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

//Check if the given username exists
func (a *AuthAgent) UserExists(username string) bool {
	userpasswordhash := ""
	err := a.Database.Read("auth", "passhash/"+username, &userpasswordhash)
	if err != nil || userpasswordhash == "" {
		return false
	}
	return true
}

//Update the session expire time given the request header.
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

//Create user account
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

	return nil
}

//Hash the given raw string into sha512 hash
func Hash(raw string) string {
	h := sha512.New()
	h.Write([]byte(raw))
	return hex.EncodeToString(h.Sum(nil))
}
