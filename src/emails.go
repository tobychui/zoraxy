package main

import (
	"bytes"
	"encoding/gob"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"imuslab.com/zoraxy/mod/email"
	"imuslab.com/zoraxy/mod/utils"
)

/*
	SMTP Settings and Test Email Handlers
*/

func HandleSMTPSet(w http.ResponseWriter, r *http.Request) {
	hostname, err := utils.PostPara(r, "hostname")
	if err != nil {
		utils.SendErrorResponse(w, "hostname cannot be empty")
		return
	}

	domain, err := utils.PostPara(r, "domain")
	if err != nil {
		utils.SendErrorResponse(w, "domain cannot be empty")
		return
	}

	portString, err := utils.PostPara(r, "port")
	if err != nil {
		utils.SendErrorResponse(w, "port must be a valid integer")
		return
	}

	port, err := strconv.Atoi(portString)
	if err != nil {
		utils.SendErrorResponse(w, "port must be a valid integer")
		return
	}

	username, err := utils.PostPara(r, "username")
	if err != nil {
		utils.SendErrorResponse(w, "username cannot be empty")
		return
	}

	password, err := utils.PostPara(r, "password")
	if err != nil {
		//Empty password. Use old one if exists
		oldConfig := loadSMTPConfig()
		if oldConfig.Password == "" {
			utils.SendErrorResponse(w, "password cannot be empty")
			return
		} else {
			password = oldConfig.Password
		}
	}

	senderAddr, err := utils.PostPara(r, "senderAddr")
	if err != nil {
		utils.SendErrorResponse(w, "senderAddr cannot be empty")
		return
	}

	adminAddr, err := utils.PostPara(r, "adminAddr")
	if err != nil {
		utils.SendErrorResponse(w, "adminAddr cannot be empty")
		return
	}

	//Set the email sender properties
	thisEmailSender := email.Sender{
		Hostname:   strings.TrimSpace(hostname),
		Domain:     strings.TrimSpace(domain),
		Port:       port,
		Username:   strings.TrimSpace(username),
		Password:   strings.TrimSpace(password),
		SenderAddr: strings.TrimSpace(senderAddr),
	}

	//Write this into database
	setSMTPConfig(&thisEmailSender)

	//Update the current EmailSender
	EmailSender = &thisEmailSender

	//Set the admin address of password reset
	setSMTPAdminAddress(adminAddr)

	//Reply ok
	utils.SendOK(w)
}

func HandleSMTPGet(w http.ResponseWriter, r *http.Request) {
	// Create a buffer to store the encoded value
	var buf bytes.Buffer

	// Encode the original object into the buffer
	encoder := gob.NewEncoder(&buf)
	err := encoder.Encode(*EmailSender)
	if err != nil {
		utils.SendErrorResponse(w, "Internal encode error")
		return
	}

	// Decode the buffer into a new object
	var copied email.Sender
	decoder := gob.NewDecoder(&buf)
	err = decoder.Decode(&copied)
	if err != nil {
		utils.SendErrorResponse(w, "Internal decode error")
		return
	}

	copied.Password = ""

	js, _ := json.Marshal(copied)
	utils.SendJSONResponse(w, string(js))
}

func HandleAdminEmailGet(w http.ResponseWriter, r *http.Request) {
	js, _ := json.Marshal(loadSMTPAdminAddr())
	utils.SendJSONResponse(w, string(js))
}

func HandleTestEmailSend(w http.ResponseWriter, r *http.Request) {
	adminEmailAccount := loadSMTPAdminAddr()
	if adminEmailAccount == "" {
		utils.SendErrorResponse(w, "Management account not set")
		return
	}

	err := EmailSender.SendEmail(adminEmailAccount,
		"SMTP Testing Email | Zoraxy", "This is a test email sent by Zoraxy. Please do not reply to this email.<br>Zoraxy からのテストメールです。このメールには返信しないでください。<br>這是由 Zoraxy 發送的測試電子郵件。請勿回覆此郵件。<br>Ceci est un email de test envoyé par Zoraxy. Merci de ne pas répondre à cet email.<br>Dies ist eine Test-E-Mail, die von Zoraxy gesendet wurde. Bitte antworten Sie nicht auf diese E-Mail.")

	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

/*
	SMTP config

	The following handle SMTP configs
*/

func setSMTPConfig(config *email.Sender) error {
	return sysdb.Write("smtp", "config", config)
}

func loadSMTPConfig() *email.Sender {
	if sysdb.KeyExists("smtp", "config") {
		thisEmailSender := email.Sender{
			Port: 587,
		}
		err := sysdb.Read("smtp", "config", &thisEmailSender)
		if err != nil {
			return &email.Sender{
				Port: 587,
			}
		}
		return &thisEmailSender
	} else {
		//Not set. Return an empty one
		return &email.Sender{
			Port: 587,
		}
	}
}

func setSMTPAdminAddress(adminAddr string) error {
	return sysdb.Write("smtp", "admin", adminAddr)
}

// Load SMTP admin address. Return empty string if not set
func loadSMTPAdminAddr() string {
	adminAddr := ""
	if sysdb.KeyExists("smtp", "admin") {
		err := sysdb.Read("smtp", "admin", &adminAddr)
		if err != nil {
			return ""
		}
		return adminAddr
	} else {
		return ""
	}
}

/*
	Admin Account Reset
*/

var (
	accountResetEmailDelay   int64  = 30      //Delay between each account reset email, default 30s
	tokenValidDuration       int64  = 15 * 60 //Duration of the token, default 15 minutes
	lastAccountResetEmail    int64  = 0       //Timestamp for last sent account reset email
	passwordResetAccessToken string = ""      //Access token for resetting password
)

func HandleAdminAccountResetEmail(w http.ResponseWriter, r *http.Request) {
	if EmailSender.Username == "" || EmailSender.Domain == "" {
		//Reset account not setup
		utils.SendErrorResponse(w, "Reset account not setup.")
		return
	}

	if loadSMTPAdminAddr() == "" {
		utils.SendErrorResponse(w, "Reset account not setup.")
	}

	//Check if the delay expired
	if lastAccountResetEmail+accountResetEmailDelay > time.Now().Unix() {
		//Too frequent
		utils.SendErrorResponse(w, "You cannot send another account reset email in cooldown time")
		return
	}

	passwordResetAccessToken = uuid.New().String()

	//SMTP info exists. Send reset account email
	lastAccountResetEmail = time.Now().Unix()
	EmailSender.SendEmail(loadSMTPAdminAddr(), "Management Account Reset | Zoraxy",
		"Enter the following reset token to reset your password on your Zoraxy router.<br>"+passwordResetAccessToken+"<br><br> This is an automated generated email. DO NOT REPLY TO THIS EMAIL.")

	utils.SendOK(w)
}

func HandleNewPasswordSetup(w http.ResponseWriter, r *http.Request) {
	if passwordResetAccessToken == "" {
		//Not initiated
		utils.SendErrorResponse(w, "Invalid usage")
		return
	}

	username, err := utils.PostPara(r, "username")
	if err != nil {
		utils.SendErrorResponse(w, "Invalid username given")
		return
	}
	token, err := utils.PostPara(r, "token")
	if err != nil {
		utils.SendErrorResponse(w, "Invalid token given")
		return
	}
	newPassword, err := utils.PostPara(r, "newpw")
	if err != nil {
		utils.SendErrorResponse(w, "Invalid new password given")
		return
	}

	token = strings.TrimSpace(token)
	username = strings.TrimSpace(username)

	//Validate the token
	if token != passwordResetAccessToken {
		utils.SendErrorResponse(w, "Invalid Token")
		return
	}

	//Check if time expired
	if lastAccountResetEmail+tokenValidDuration < time.Now().Unix() {
		//Expired
		utils.SendErrorResponse(w, "Token expired")
		return
	}

	//Check if user exists
	if !authAgent.UserExists(username) {
		//Invalid admin account name
		utils.SendErrorResponse(w, "Invalid Username")
		return
	}

	//Delete the user account
	authAgent.UnregisterUser(username)

	//Ok. Set the new password
	err = authAgent.CreateUserAccount(username, newPassword, "")
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}
