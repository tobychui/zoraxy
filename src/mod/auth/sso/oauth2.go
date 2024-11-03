package sso

import (
	"context"
	_ "embed"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/go-oauth2/oauth2/v4/errors"
	"github.com/go-oauth2/oauth2/v4/generates"
	"github.com/go-oauth2/oauth2/v4/manage"
	"github.com/go-oauth2/oauth2/v4/models"
	"github.com/go-oauth2/oauth2/v4/server"
	"github.com/go-oauth2/oauth2/v4/store"
	"github.com/go-session/session"
	"imuslab.com/zoraxy/mod/utils"
)

const (
	SSO_SESSION_NAME = "ZoraxySSO"
)

type OAuth2Server struct {
	srv    *server.Server //oAuth server instance
	config *SSOConfig
	parent *SSOHandler
}

//go:embed static/auth.html
var authHtml []byte

//go:embed static/login.html
var loginHtml []byte

// NewOAuth2Server creates a new OAuth2 server instance
func NewOAuth2Server(config *SSOConfig, parent *SSOHandler) (*OAuth2Server, error) {
	manager := manage.NewDefaultManager()
	manager.SetAuthorizeCodeTokenCfg(manage.DefaultAuthorizeCodeTokenCfg)
	// token store
	manager.MustTokenStorage(store.NewFileTokenStore("./conf/sso.db"))
	// generate jwt access token
	manager.MapAccessGenerate(generates.NewAccessGenerate())

	//Load the information of registered app within the OAuth2 server
	clientStore := store.NewClientStore()
	clientStore.Set("myapp", &models.Client{
		ID:     "myapp",
		Secret: "verysecurepassword",
		Domain: "localhost:9094",
	})
	//TODO: LOAD THIS DYNAMICALLY FROM DATABASE
	manager.MapClientStorage(clientStore)

	thisServer := OAuth2Server{
		config: config,
		parent: parent,
	}

	//Create a new oauth server
	srv := server.NewServer(server.NewConfig(), manager)
	srv.SetPasswordAuthorizationHandler(thisServer.PasswordAuthorizationHandler)
	srv.SetUserAuthorizationHandler(thisServer.UserAuthorizeHandler)
	srv.SetInternalErrorHandler(func(err error) (re *errors.Response) {
		log.Println("Internal Error:", err.Error())
		return
	})
	srv.SetResponseErrorHandler(func(re *errors.Response) {
		log.Println("Response Error:", re.Error.Error())
	})

	//Set the access scope handler
	srv.SetAuthorizeScopeHandler(thisServer.AuthorizationScopeHandler)
	//Set the access token expiration handler based on requesting domain / hostname
	srv.SetAccessTokenExpHandler(thisServer.ExpireHandler)
	thisServer.srv = srv
	return &thisServer, nil
}

// Password handler, validate if the given username and password are correct
func (oas *OAuth2Server) PasswordAuthorizationHandler(ctx context.Context, clientID, username, password string) (userID string, err error) {
	//TODO: LOAD THIS DYNAMICALLY FROM DATABASE
	if username == "test" && password == "test" {
		userID = "test"
	}
	return
}

// User Authorization Handler, handle auth request from user
func (oas *OAuth2Server) UserAuthorizeHandler(w http.ResponseWriter, r *http.Request) (userID string, err error) {
	store, err := session.Start(r.Context(), w, r)
	if err != nil {
		return
	}

	uid, ok := store.Get(SSO_SESSION_NAME)
	if !ok {
		if r.Form == nil {
			r.ParseForm()
		}

		store.Set("ReturnUri", r.Form)
		store.Save()

		w.Header().Set("Location", "/oauth2/login")
		w.WriteHeader(http.StatusFound)
		return
	}

	userID = uid.(string)
	store.Delete(SSO_SESSION_NAME)
	store.Save()
	return
}

// AccessTokenExpHandler, set the SSO session length default value
func (oas *OAuth2Server) ExpireHandler(w http.ResponseWriter, r *http.Request) (exp time.Duration, err error) {
	requestHostname := r.Host
	if requestHostname == "" {
		//Use default value
		return time.Hour, nil
	}

	//Get the Registered App Config from parent
	appConfig, ok := oas.parent.Apps[requestHostname]
	if !ok {
		//Use default value
		return time.Hour, nil
	}

	//Use the app's session length
	return time.Second * time.Duration(appConfig.SessionDuration), nil
}

// AuthorizationScopeHandler, handle the scope of the request
func (oas *OAuth2Server) AuthorizationScopeHandler(w http.ResponseWriter, r *http.Request) (scope string, err error) {
	//Get the scope from post or GEt request
	if r.Form == nil {
		if err := r.ParseForm(); err != nil {
			return "none", err
		}
	}

	//Get the hostname of the request
	requestHostname := r.Host
	if requestHostname == "" {
		//No rule set. Use default
		return "none", nil
	}

	//Get the Registered App Config from parent
	appConfig, ok := oas.parent.Apps[requestHostname]
	if !ok {
		//No rule set. Use default
		return "none", nil
	}

	//Check if the scope is set in the request
	if v, ok := r.Form["scope"]; ok {
		//Check if the requested scope is in the appConfig scope
		if utils.StringInArray(appConfig.Scopes, v[0]) {
			return v[0], nil
		}
		return "none", nil
	}

	return "none", nil
}

/* SSO Web Server Toggle Functions */
func (oas *OAuth2Server) RegisterOauthEndpoints(primaryMux *http.ServeMux) {
	primaryMux.HandleFunc("/oauth2/login", oas.loginHandler)
	primaryMux.HandleFunc("/oauth2/auth", oas.authHandler)

	primaryMux.HandleFunc("/oauth2/authorize", func(w http.ResponseWriter, r *http.Request) {
		store, err := session.Start(r.Context(), w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var form url.Values
		if v, ok := store.Get("ReturnUri"); ok {
			form = v.(url.Values)
		}
		r.Form = form

		store.Delete("ReturnUri")
		store.Save()

		err = oas.srv.HandleAuthorizeRequest(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	})

	primaryMux.HandleFunc("/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		err := oas.srv.HandleTokenRequest(w, r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	primaryMux.HandleFunc("/test", func(w http.ResponseWriter, r *http.Request) {
		token, err := oas.srv.ValidationBearerToken(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		data := map[string]interface{}{
			"expires_in": int64(time.Until(token.GetAccessCreateAt().Add(token.GetAccessExpiresIn())).Seconds()),
			"client_id":  token.GetClientID(),
			"user_id":    token.GetUserID(),
		}
		e := json.NewEncoder(w)
		e.SetIndent("", "  ")
		e.Encode(data)
	})
}

func (oas *OAuth2Server) loginHandler(w http.ResponseWriter, r *http.Request) {
	store, err := session.Start(r.Context(), w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if r.Method == "POST" {
		if r.Form == nil {
			if err := r.ParseForm(); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		//Load username and password from form post
		username, err := utils.PostPara(r, "username")
		if err != nil {
			w.Write([]byte("invalid username or password"))
			return
		}

		password, err := utils.PostPara(r, "password")
		if err != nil {
			w.Write([]byte("invalid username or password"))
			return
		}

		//Validate the user
		if !oas.parent.ValidateUsernameAndPassword(username, password) {
			//Wrong password
			w.Write([]byte("invalid username or password"))
			return
		}

		store.Set(SSO_SESSION_NAME, r.Form.Get("username"))
		store.Save()

		w.Header().Set("Location", "/oauth2/auth")
		w.WriteHeader(http.StatusFound)
		return
	} else if r.Method == "GET" {
		//Check if the user is logged in
		if _, ok := store.Get(SSO_SESSION_NAME); ok {
			w.Header().Set("Location", "/oauth2/auth")
			w.WriteHeader(http.StatusFound)
			return
		}
	}
	//User not logged in. Show login page
	w.Write(loginHtml)
}

func (oas *OAuth2Server) authHandler(w http.ResponseWriter, r *http.Request) {
	store, err := session.Start(context.TODO(), w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if _, ok := store.Get(SSO_SESSION_NAME); !ok {
		w.Header().Set("Location", "/oauth2/login")
		w.WriteHeader(http.StatusFound)
		return
	}
	//User logged in. Check if this user have previously authorized the app

	//TODO: Check if the user have previously authorized the app

	//User have not authorized the app. Show the authorization page
	w.Write(authHtml)
}
