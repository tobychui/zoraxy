package email

import (
	"net/smtp"
	"strconv"
)

/*
	Email.go

	This script handle mailing services using SMTP protocol
*/

type Sender struct {
	Hostname   string //E.g. mail.gandi.net
	Port       int    //E.g. 587
	Username   string //Username of the email account
	Password   string //Password of the email account
	SenderAddr string //e.g. admin@arozos.com
}

// Create a new email sender object
func NewEmailSender(hostname string, port int, username string, password string, senderAddr string) *Sender {
	return &Sender{
		Hostname:   hostname,
		Port:       port,
		Username:   username,
		Password:   password,
		SenderAddr: senderAddr,
	}
}

/*
Send a email to a reciving addr
Example Usage:
SendEmail(

	test@example.com,
	"Free donuts",
	"Come get your free donuts on this Sunday!"

)
*/
func (s *Sender) SendEmail(to string, subject string, content string) error {
	//Parse the email content
	msg := []byte("To: " + to + "\n" +
		"From: Zoraxy <" + s.SenderAddr + ">\n" +
		"Subject: " + subject + "\n" +
		"MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\n\n" +
		content + "\n\n")

	//Login to the SMTP server
	//Username can be username (e.g. admin) or email (e.g. admin@example.com), depending on SMTP service provider
	auth := smtp.PlainAuth("", s.Username, s.Password, s.Hostname)

	err := smtp.SendMail(s.Hostname+":"+strconv.Itoa(s.Port), auth, s.SenderAddr, []string{to}, msg)
	if err != nil {
		return err
	}

	return nil
}
