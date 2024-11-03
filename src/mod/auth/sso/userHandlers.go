package sso

/*
	userHandlers.go
	Handlers for SSO user management

	If you are looking for handlers that changes the settings
	of the SSO portal (e.g. authURL or port), please refer to
	handlers.go.
*/

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gofrs/uuid"
	"imuslab.com/zoraxy/mod/auth"
	"imuslab.com/zoraxy/mod/utils"
)

// HandleAddUser handle the request to add a new user to the SSO system
func (s *SSOHandler) HandleAddUser(w http.ResponseWriter, r *http.Request) {
	username, err := utils.PostPara(r, "username")
	if err != nil {
		utils.SendErrorResponse(w, "invalid username given")
		return
	}

	password, err := utils.PostPara(r, "password")
	if err != nil {
		utils.SendErrorResponse(w, "invalid password given")
		return
	}

	newUserId, err := uuid.NewV4()
	if err != nil {
		utils.SendErrorResponse(w, "failed to generate new user ID")
		return
	}

	//Create a new user entry
	thisUserEntry := UserEntry{
		UserID:       newUserId.String(),
		Username:     username,
		PasswordHash: auth.Hash(password),
		TOTPCode:     "",
		Enable2FA:    false,
	}

	js, _ := json.Marshal(thisUserEntry)

	//Create a new user in the database
	err = s.Config.Database.Write("sso_users", newUserId.String(), string(js))
	if err != nil {
		utils.SendErrorResponse(w, "failed to create new user")
		return
	}
	utils.SendOK(w)
}

// Edit user information, only accept change of username, password and enabled subdomain filed
func (s *SSOHandler) HandleEditUser(w http.ResponseWriter, r *http.Request) {
	userID, err := utils.PostPara(r, "user_id")
	if err != nil {
		utils.SendErrorResponse(w, "invalid user ID given")
		return
	}

	if !(s.SSOUserExists(userID)) {
		utils.SendErrorResponse(w, "user not found")
		return
	}

	//Load the user entry from database
	userEntry, err := s.GetSSOUser(userID)
	if err != nil {
		utils.SendErrorResponse(w, "failed to load user entry")
		return
	}

	//Update each of the fields if it is provided
	username, err := utils.PostPara(r, "username")
	if err == nil {
		userEntry.Username = username
	}

	password, err := utils.PostPara(r, "password")
	if err == nil {
		userEntry.PasswordHash = auth.Hash(password)
	}

	//Update the user entry in the database
	js, _ := json.Marshal(userEntry)
	err = s.Config.Database.Write("sso_users", userID, string(js))
	if err != nil {
		utils.SendErrorResponse(w, "failed to update user entry")
		return
	}
	utils.SendOK(w)
}

// HandleRemoveUser remove a user from the SSO system
func (s *SSOHandler) HandleRemoveUser(w http.ResponseWriter, r *http.Request) {
	userID, err := utils.PostPara(r, "user_id")
	if err != nil {
		utils.SendErrorResponse(w, "invalid user ID given")
		return
	}

	if !(s.SSOUserExists(userID)) {
		utils.SendErrorResponse(w, "user not found")
		return
	}

	//Remove the user from the database
	err = s.Config.Database.Delete("sso_users", userID)
	if err != nil {
		utils.SendErrorResponse(w, "failed to remove user")
		return
	}
	utils.SendOK(w)
}

// HandleListUser list all users in the SSO system
func (s *SSOHandler) HandleListUser(w http.ResponseWriter, r *http.Request) {
	ssoUsers, err := s.ListSSOUsers()
	if err != nil {
		utils.SendErrorResponse(w, "failed to list users")
		return
	}
	js, _ := json.Marshal(ssoUsers)
	utils.SendJSONResponse(w, string(js))
}

// HandleAddSubdomain add a subdomain to a user
func (s *SSOHandler) HandleAddSubdomain(w http.ResponseWriter, r *http.Request) {
	userid, err := utils.PostPara(r, "user_id")
	if err != nil {
		utils.SendErrorResponse(w, "invalid user ID given")
		return
	}

	if !(s.SSOUserExists(userid)) {
		utils.SendErrorResponse(w, "user not found")
		return
	}

	UserEntry, err := s.GetSSOUser(userid)
	if err != nil {
		utils.SendErrorResponse(w, "failed to load user entry")
		return
	}

	subdomain, err := utils.PostPara(r, "subdomain")
	if err != nil {
		utils.SendErrorResponse(w, "invalid subdomain given")
		return
	}

	allowAccess, err := utils.PostBool(r, "allow_access")
	if err != nil {
		utils.SendErrorResponse(w, "invalid allow access value given")
		return
	}

	UserEntry.Subdomains[subdomain] = &SubdomainAccessRule{
		Subdomain:   subdomain,
		AllowAccess: allowAccess,
	}

	err = UserEntry.Update()
	if err != nil {
		utils.SendErrorResponse(w, "failed to update user entry")
		return
	}

	utils.SendOK(w)
}

// HandleRemoveSubdomain remove a subdomain from a user
func (s *SSOHandler) HandleRemoveSubdomain(w http.ResponseWriter, r *http.Request) {
	userid, err := utils.PostPara(r, "user_id")
	if err != nil {
		utils.SendErrorResponse(w, "invalid user ID given")
		return
	}

	if !(s.SSOUserExists(userid)) {
		utils.SendErrorResponse(w, "user not found")
		return
	}

	UserEntry, err := s.GetSSOUser(userid)
	if err != nil {
		utils.SendErrorResponse(w, "failed to load user entry")
		return
	}

	subdomain, err := utils.PostPara(r, "subdomain")
	if err != nil {
		utils.SendErrorResponse(w, "invalid subdomain given")
		return
	}

	delete(UserEntry.Subdomains, subdomain)

	err = UserEntry.Update()
	if err != nil {
		utils.SendErrorResponse(w, "failed to update user entry")
		return
	}

	utils.SendOK(w)
}

// HandleEnable2FA enable 2FA for a user
func (s *SSOHandler) HandleEnable2FA(w http.ResponseWriter, r *http.Request) {
	userid, err := utils.PostPara(r, "user_id")
	if err != nil {
		utils.SendErrorResponse(w, "invalid user ID given")
		return
	}

	if !(s.SSOUserExists(userid)) {
		utils.SendErrorResponse(w, "user not found")
		return
	}

	UserEntry, err := s.GetSSOUser(userid)
	if err != nil {
		utils.SendErrorResponse(w, "failed to load user entry")
		return
	}

	UserEntry.Enable2FA = true
	provisionUri, err := UserEntry.ResetTotp(UserEntry.UserID, "Zoraxy-SSO")
	if err != nil {
		utils.SendErrorResponse(w, "failed to reset TOTP")
		return
	}
	//As the ResetTotp function will update the user entry in the database, no need to call Update here

	js, _ := json.Marshal(provisionUri)
	utils.SendJSONResponse(w, string(js))
}

// Handle Disable 2FA for a user
func (s *SSOHandler) HandleDisable2FA(w http.ResponseWriter, r *http.Request) {
	userid, err := utils.PostPara(r, "user_id")
	if err != nil {
		utils.SendErrorResponse(w, "invalid user ID given")
		return
	}

	if !(s.SSOUserExists(userid)) {
		utils.SendErrorResponse(w, "user not found")
		return
	}

	UserEntry, err := s.GetSSOUser(userid)
	if err != nil {
		utils.SendErrorResponse(w, "failed to load user entry")
		return
	}

	UserEntry.Enable2FA = false
	UserEntry.TOTPCode = ""

	err = UserEntry.Update()
	if err != nil {
		utils.SendErrorResponse(w, "failed to update user entry")
		return
	}

	utils.SendOK(w)
}

// HandleVerify2FA verify the 2FA code for a user
func (s *SSOHandler) HandleVerify2FA(w http.ResponseWriter, r *http.Request) (bool, error) {
	userid, err := utils.PostPara(r, "user_id")
	if err != nil {
		return false, errors.New("invalid user ID given")
	}

	if !(s.SSOUserExists(userid)) {
		utils.SendErrorResponse(w, "user not found")
		return false, errors.New("user not found")
	}

	UserEntry, err := s.GetSSOUser(userid)
	if err != nil {
		utils.SendErrorResponse(w, "failed to load user entry")
		return false, errors.New("failed to load user entry")
	}

	totpCode, _ := utils.PostPara(r, "totp_code")

	if !UserEntry.Enable2FA {
		//If 2FA is not enabled, return true
		return true, nil
	}

	if !UserEntry.VerifyTotp(totpCode) {
		return false, nil
	}

	return true, nil
}
