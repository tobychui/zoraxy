package sso

import (
	"encoding/json"
	"time"

	"github.com/xlzd/gotp"
	"imuslab.com/zoraxy/mod/auth"
)

/*
	users.go

	This file contains the user structure and user management
	functions for the SSO module.

	If you are looking for handlers, please refer to handlers.go.
*/

type SubdomainAccessRule struct {
	Subdomain   string
	AllowAccess bool
}

type UserEntry struct {
	UserID       string                          //User ID, in UUIDv4 format
	Username     string                          //Username
	PasswordHash string                          //Password hash
	TOTPCode     string                          //2FA TOTP code
	Enable2FA    bool                            //Enable 2FA for this user
	Subdomains   map[string]*SubdomainAccessRule //Subdomain and access rule
	parent       *SSOHandler                     //Parent SSO handler
}

func (s *SSOHandler) SSOUserExists(userid string) bool {
	//Check if the user exists in the database
	var userEntry UserEntry
	err := s.Config.Database.Read("sso_users", userid, &userEntry)
	return err == nil
}

func (s *SSOHandler) GetSSOUser(userid string) (UserEntry, error) {
	//Load the user entry from database
	var userEntry UserEntry
	err := s.Config.Database.Read("sso_users", userid, &userEntry)
	if err != nil {
		return UserEntry{}, err
	}
	userEntry.parent = s
	return userEntry, nil
}

func (s *SSOHandler) ListSSOUsers() ([]*UserEntry, error) {
	entries, err := s.Config.Database.ListTable("sso_users")
	if err != nil {
		return nil, err
	}
	ssoUsers := []*UserEntry{}
	for _, keypairs := range entries {
		group := new(UserEntry)
		json.Unmarshal(keypairs[1], &group)
		group.parent = s
		ssoUsers = append(ssoUsers, group)
	}

	return ssoUsers, nil
}

// Validate the username and password
func (s *SSOHandler) ValidateUsernameAndPassword(username string, password string) bool {
	//Validate the username and password
	var userEntry UserEntry
	err := s.Config.Database.Read("sso_users", username, &userEntry)
	if err != nil {
		return false
	}

	//TODO: Remove after testing
	if (username == "test") && (password == "test") {
		return true
	}
	return userEntry.VerifyPassword(password)
}

func (s *UserEntry) VerifyPassword(password string) bool {
	return s.PasswordHash == auth.Hash(password)
}

// Write changes in the user entry back to the database
func (u *UserEntry) Update() error {
	js, _ := json.Marshal(u)
	err := u.parent.Config.Database.Write("sso_users", u.UserID, string(js))
	if err != nil {
		return err
	}
	return nil
}

// Reset and update the TOTP code for the current user
// Return the provision uri of the new TOTP code for Google Authenticator
func (u *UserEntry) ResetTotp(accountName string, issuerName string) (string, error) {
	u.TOTPCode = gotp.RandomSecret(16)
	totp := gotp.NewDefaultTOTP(u.TOTPCode)
	err := u.Update()
	if err != nil {
		return "", err
	}
	return totp.ProvisioningUri(accountName, issuerName), nil
}

// Verify the TOTP code at current time
func (u *UserEntry) VerifyTotp(enteredCode string) bool {
	totp := gotp.NewDefaultTOTP(u.TOTPCode)
	return totp.Verify(enteredCode, time.Now().Unix())
}
