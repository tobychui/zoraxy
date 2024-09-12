package sso

/*
	handlers.go

	This file contains the handlers for the SSO module.
	If you are looking for handlers for SSO user management,
	please refer to userHandlers.go.
*/

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gofrs/uuid"
	"imuslab.com/zoraxy/mod/utils"
)

// HandleSSOStatus handle the request to get the status of the SSO portal server
func (s *SSOHandler) HandleSSOStatus(w http.ResponseWriter, r *http.Request) {
	type SSOStatus struct {
		Enabled             bool
		SSOInterceptEnabled bool
		ListeningPort       int
		AuthURL             string
	}

	status := SSOStatus{
		Enabled: s.ssoPortalServer != nil,
		//SSOInterceptEnabled: s.ssoInterceptEnabled,
		ListeningPort: s.Config.PortalServerPort,
		AuthURL:       s.Config.AuthURL,
	}

	js, _ := json.Marshal(status)
	utils.SendJSONResponse(w, string(js))
}

// HandleStartSSOPortal handle the request to start the SSO portal server
func (s *SSOHandler) HandleStartSSOPortal(w http.ResponseWriter, r *http.Request) {
	err := s.StartSSOPortal()
	if err != nil {
		s.Log("Failed to start SSO portal server", err)
		utils.SendErrorResponse(w, "failed to start SSO portal server")
		return
	}
	//Write current state to database
	err = s.Config.Database.Write("sso_conf", "enabled", true)
	if err != nil {
		utils.SendErrorResponse(w, "failed to update SSO state")
		return
	}
	utils.SendOK(w)
}

// HandleStopSSOPortal handle the request to stop the SSO portal server
func (s *SSOHandler) HandleStopSSOPortal(w http.ResponseWriter, r *http.Request) {
	err := s.ssoPortalServer.Close()
	if err != nil {
		s.Log("Failed to stop SSO portal server", err)
		utils.SendErrorResponse(w, "failed to stop SSO portal server")
		return
	}
	s.ssoPortalServer = nil

	//Write current state to database
	err = s.Config.Database.Write("sso_conf", "enabled", false)
	if err != nil {
		utils.SendErrorResponse(w, "failed to update SSO state")
		return
	}

	//Clear the cookie store and restart the server
	err = s.RestartSSOServer()
	if err != nil {
		utils.SendErrorResponse(w, "failed to restart SSO server")
		return
	}
	utils.SendOK(w)
}

// HandlePortChange handle the request to change the SSO portal server port
func (s *SSOHandler) HandlePortChange(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		//Return the current port
		js, _ := json.Marshal(s.Config.PortalServerPort)
		utils.SendJSONResponse(w, string(js))
		return
	}

	port, err := utils.PostInt(r, "port")
	if err != nil {
		utils.SendErrorResponse(w, "invalid port given")
		return
	}

	s.Config.PortalServerPort = port

	//Write to the database
	err = s.Config.Database.Write("sso_conf", "port", port)
	if err != nil {
		utils.SendErrorResponse(w, "failed to update port")
		return
	}

	//Clear the cookie store and restart the server
	err = s.RestartSSOServer()
	if err != nil {
		utils.SendErrorResponse(w, "failed to restart SSO server")
		return
	}
	utils.SendOK(w)
}

// HandleSetAuthURL handle the request to change the SSO auth URL
// This is the URL that the SSO portal server will redirect to for authentication
// e.g. auth.yourdomain.com
func (s *SSOHandler) HandleSetAuthURL(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		//Return the current auth URL
		js, _ := json.Marshal(s.Config.AuthURL)
		utils.SendJSONResponse(w, string(js))
		return
	}

	//Get the auth URL
	authURL, err := utils.PostPara(r, "auth_url")
	if err != nil {
		utils.SendErrorResponse(w, "invalid auth URL given")
		return
	}

	s.Config.AuthURL = authURL

	//Write to the database
	err = s.Config.Database.Write("sso_conf", "authurl", authURL)
	if err != nil {
		utils.SendErrorResponse(w, "failed to update auth URL")
		return
	}

	//Clear the cookie store and restart the server
	err = s.RestartSSOServer()
	if err != nil {
		utils.SendErrorResponse(w, "failed to restart SSO server")
		return
	}
	utils.SendOK(w)
}

// HandleRegisterApp handle the request to register a new app to the SSO portal
func (s *SSOHandler) HandleRegisterApp(w http.ResponseWriter, r *http.Request) {
	appName, err := utils.PostPara(r, "app_name")
	if err != nil {
		utils.SendErrorResponse(w, "invalid app name given")
		return
	}

	id, err := utils.PostPara(r, "app_id")
	if err != nil {
		//If id is not given, use the app name with a random UUID
		newID, err := uuid.NewV4()
		if err != nil {
			utils.SendErrorResponse(w, "failed to generate new app ID")
			return
		}
		id = strings.ReplaceAll(appName, " ", "") + "-" + newID.String()
	}

	//Check if the given appid is already in use
	if _, ok := s.Apps[id]; ok {
		utils.SendErrorResponse(w, "app ID already in use")
		return
	}

	/*
		Process the app domain
		An app can have multiple domains, separated by commas
		Usually the app domain is the proxy rule that points to the app
		For example, if the app is hosted at app.yourdomain.com, the app domain is app.yourdomain.com
	*/
	appDomain, err := utils.PostPara(r, "app_domain")
	if err != nil {
		utils.SendErrorResponse(w, "invalid app URL given")
		return
	}

	appURLs := strings.Split(appDomain, ",")
	//Remove padding and trailing spaces in each URL
	for i := range appURLs {
		appURLs[i] = strings.TrimSpace(appURLs[i])
	}

	//Create a new app entry
	thisAppEntry := RegisteredUpstreamApp{
		ID:              id,
		Secret:          "",
		Domain:          appURLs,
		Scopes:          []string{},
		SessionDuration: 3600,
	}

	js, _ := json.Marshal(thisAppEntry)

	//Create a new app in the database
	err = s.Config.Database.Write("sso_apps", appName, string(js))
	if err != nil {
		utils.SendErrorResponse(w, "failed to create new app")
		return
	}

	//Also add the app to runtime config
	s.Apps[appName] = thisAppEntry

	utils.SendOK(w)
}

// HandleAppRemove handle the request to remove an app from the SSO portal
func (s *SSOHandler) HandleAppRemove(w http.ResponseWriter, r *http.Request) {
	appID, err := utils.PostPara(r, "app_id")
	if err != nil {
		utils.SendErrorResponse(w, "invalid app ID given")
		return
	}

	//Check if the app actually exists
	if _, ok := s.Apps[appID]; !ok {
		utils.SendErrorResponse(w, "app not found")
		return
	}
	delete(s.Apps, appID)

	//Also remove it from the database
	err = s.Config.Database.Delete("sso_apps", appID)
	if err != nil {
		s.Log("Failed to remove app from database", err)
	}

}
