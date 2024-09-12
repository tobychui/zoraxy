package sso

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"imuslab.com/zoraxy/mod/utils"
)

/*
	server.go

	This is the web server for the SSO portal. It contains the
	HTTP server and the handlers for the SSO portal.

	If you are looking for handlers that changes the settings
	of the SSO portale or user management, please refer to
	handlers.go.

*/

func (h *SSOHandler) InitSSOPortal(portalServerPort int) {
	//Create a new web server for the SSO portal
	pmux := http.NewServeMux()
	fs := http.FileServer(http.FS(staticFiles))
	pmux.Handle("/", fs)

	//Register API endpoint for the SSO portal
	pmux.HandleFunc("/sso/login", h.HandleLogin)

	//Register OAuth2 endpoints
	h.Oauth2Server.RegisterOauthEndpoints(pmux)

	h.ssoPortalMux = pmux
}

// StartSSOPortal start the SSO portal server
// This function will block the main thread, call it in a goroutine
func (h *SSOHandler) StartSSOPortal() error {
	h.ssoPortalServer = &http.Server{
		Addr:    ":" + strconv.Itoa(h.Config.PortalServerPort),
		Handler: h.ssoPortalMux,
	}
	err := h.ssoPortalServer.ListenAndServe()
	if err != nil {
		h.Log("Failed to start SSO portal server", err)
	}
	return err
}

// StopSSOPortal stop the SSO portal server
func (h *SSOHandler) StopSSOPortal() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := h.ssoPortalServer.Shutdown(ctx)
	if err != nil {
		h.Log("Failed to stop SSO portal server", err)
		return err
	}
	return nil
}

// StartSSOPortal start the SSO portal server
func (h *SSOHandler) RestartSSOServer() error {
	err := h.StopSSOPortal()
	if err != nil {
		return err
	}
	go h.StartSSOPortal()
	return nil
}

func (h *SSOHandler) IsRunning() bool {
	return h.ssoPortalServer != nil
}

// HandleLogin handle the login request
func (h *SSOHandler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	//Handle the login request
	username, err := utils.PostPara(r, "username")
	if err != nil {
		utils.SendErrorResponse(w, "invalid username or password")
		return
	}

	password, err := utils.PostPara(r, "password")
	if err != nil {
		utils.SendErrorResponse(w, "invalid username or password")
		return
	}

	rememberMe, err := utils.PostBool(r, "remember_me")
	if err != nil {
		rememberMe = false
	}

	//Check if the user exists
	userEntry, err := h.GetSSOUser(username)
	if err != nil {
		utils.SendErrorResponse(w, "user not found")
		return
	}

	//Check if the password is correct
	if !userEntry.VerifyPassword(password) {
		utils.SendErrorResponse(w, "incorrect password")
		return
	}

	//Create a new session for the user
	session, _ := h.cookieStore.Get(r, "Zoraxy-SSO")
	session.Values["username"] = username
	if rememberMe {
		session.Options.MaxAge = 86400 * 15 //15 days
	} else {
		session.Options.MaxAge = 3600 //1 hour
	}
	session.Save(r, w) //Save the session

	utils.SendOK(w)
}
