package sso

import (
	"embed"
	"net/http"

	"github.com/gorilla/sessions"
	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/info/logger"
)

/*
	sso.go

	This file contains the main SSO handler and the SSO configuration
	structure. It also contains the main SSO handler functions.

	SSO web interface are stored in the static folder, which is embedded
	into the binary.
*/

//go:embed static/*
var staticFiles embed.FS //Static files for the SSO portal

type SSOConfig struct {
	SystemUUID       string             //System UUID, should be passed in from main scope
	AuthURL          string             //Authentication subdomain URL, e.g. auth.example.com
	PortalServerPort int                //SSO portal server port
	Database         *database.Database //System master key-value database
	Logger           *logger.Logger
}

// SSOHandler is the main SSO handler structure
type SSOHandler struct {
	cookieStore     *sessions.CookieStore
	ssoPortalServer *http.Server
	ssoPortalMux    *http.ServeMux
	Oauth2Server    *OAuth2Server
	Config          *SSOConfig
	Apps            map[string]RegisteredUpstreamApp
}

// Create a new Zoraxy SSO handler
func NewSSOHandler(config *SSOConfig) (*SSOHandler, error) {
	//Create a cookie store for the SSO handler
	cookieStore := sessions.NewCookieStore([]byte(config.SystemUUID))
	cookieStore.Options = &sessions.Options{
		Path:     "",
		Domain:   "",
		MaxAge:   0,
		Secure:   false,
		HttpOnly: false,
		SameSite: 0,
	}

	config.Database.NewTable("sso_users") //For storing user information
	config.Database.NewTable("sso_conf")  //For storing SSO configuration
	config.Database.NewTable("sso_apps")  //For storing registered apps

	//Create the SSO Handler
	thisHandler := SSOHandler{
		cookieStore: cookieStore,
		Config:      config,
	}

	//Read the app info from database
	thisHandler.Apps = make(map[string]RegisteredUpstreamApp)

	//Create an oauth2 server
	oauth2Server, err := NewOAuth2Server(config, &thisHandler)
	if err != nil {
		return nil, err
	}

	//Register endpoints
	thisHandler.Oauth2Server = oauth2Server
	thisHandler.InitSSOPortal(config.PortalServerPort)

	return &thisHandler, nil
}

func (h *SSOHandler) RestorePreviousRunningState() {
	//Load the previous SSO state
	ssoEnabled := false
	ssoPort := 5488
	ssoAuthURL := ""
	h.Config.Database.Read("sso_conf", "enabled", &ssoEnabled)
	h.Config.Database.Read("sso_conf", "port", &ssoPort)
	h.Config.Database.Read("sso_conf", "authurl", &ssoAuthURL)

	if ssoAuthURL == "" {
		//Cannot enable SSO without auth URL
		ssoEnabled = false
	}

	h.Config.PortalServerPort = ssoPort
	h.Config.AuthURL = ssoAuthURL

	if ssoEnabled {
		go h.StartSSOPortal()
	}
}

// ServeForwardAuth handle the SSO request in interception mode
// Suppose to be called in dynamicproxy.
// Return true if the request is allowed to pass, false if the request is blocked and shall not be further processed
func (h *SSOHandler) ServeForwardAuth(w http.ResponseWriter, r *http.Request) bool {
	//Get the current uri for appending to the auth subdomain
	originalRequestURL := r.RequestURI

	redirectAuthURL := h.Config.AuthURL
	if redirectAuthURL == "" || !h.IsRunning() {
		//Redirect not set or auth server is offlined
		w.Write([]byte("SSO auth URL not set or SSO server offline."))
		//TODO: Use better looking template if exists
		return false
	}

	//Check if the user have the cookie "Zoraxy-SSO" set
	session, err := h.cookieStore.Get(r, "Zoraxy-SSO")
	if err != nil {
		//Redirect to auth subdomain
		http.Redirect(w, r, redirectAuthURL+"/sso/login?m=new&t="+originalRequestURL, http.StatusFound)
		return false
	}

	//Check if the user is logged in
	if session.Values["username"] != true {
		//Redirect to auth subdomain
		http.Redirect(w, r, redirectAuthURL+"/sso/login?m=expired&t="+originalRequestURL, http.StatusFound)
		return false
	}

	//Check if the current request subdomain is allowed
	userName := session.Values["username"].(string)
	user, err := h.GetSSOUser(userName)
	if err != nil {
		//User might have been removed from SSO. Redirect to auth subdomain
		http.Redirect(w, r, redirectAuthURL, http.StatusFound)
		return false
	}

	//Check if the user have access to the current subdomain
	if !user.Subdomains[r.Host].AllowAccess {
		//User is not allowed to access the current subdomain. Sent 403
		http.Error(w, "Forbidden", http.StatusForbidden)
		//TODO: Use better looking template if exists
		return false
	}

	//User is logged in, continue to the next handler
	return true
}

// Log a message with the SSO module tag
func (h *SSOHandler) Log(message string, err error) {
	h.Config.Logger.PrintAndLog("SSO", message, err)
}
