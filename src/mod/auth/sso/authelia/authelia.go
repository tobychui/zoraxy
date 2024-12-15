package authelia

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/utils"
)

type AutheliaRouterOptions struct {
	UseHTTPS    bool   //If the Authelia server is using HTTPS
	AutheliaURL string //The URL of the Authelia server
	Logger      *logger.Logger
	Database    *database.Database
}

type AutheliaRouter struct {
	options *AutheliaRouterOptions
}

// NewAutheliaRouter creates a new AutheliaRouter object
func NewAutheliaRouter(options *AutheliaRouterOptions) *AutheliaRouter {
	options.Database.NewTable("authelia")

	//Read settings from database, if exists
	options.Database.Read("authelia", "autheliaURL", &options.AutheliaURL)
	options.Database.Read("authelia", "useHTTPS", &options.UseHTTPS)

	return &AutheliaRouter{
		options: options,
	}
}

// HandleSetAutheliaURLAndHTTPS is the internal handler for setting the Authelia URL and HTTPS
func (ar *AutheliaRouter) HandleSetAutheliaURLAndHTTPS(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		//Return the current settings
		js, _ := json.Marshal(map[string]interface{}{
			"useHTTPS":    ar.options.UseHTTPS,
			"autheliaURL": ar.options.AutheliaURL,
		})

		utils.SendJSONResponse(w, string(js))
		return
	} else if r.Method == http.MethodPost {
		//Update the settings
		autheliaURL, err := utils.PostPara(r, "autheliaURL")
		if err != nil {
			utils.SendErrorResponse(w, "autheliaURL not found")
			return
		}

		useHTTPS, err := utils.PostBool(r, "useHTTPS")
		if err != nil {
			useHTTPS = false
		}

		//Write changes to runtime
		ar.options.AutheliaURL = autheliaURL
		ar.options.UseHTTPS = useHTTPS

		//Write changes to database
		ar.options.Database.Write("authelia", "autheliaURL", autheliaURL)
		ar.options.Database.Write("authelia", "useHTTPS", useHTTPS)

		utils.SendOK(w)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

}

// handleAutheliaAuth is the internal handler for Authelia authentication
// Set useHTTPS to true if your authelia server is using HTTPS
// Set autheliaURL to the URL of the Authelia server, e.g. authelia.example.com
func (ar *AutheliaRouter) HandleAutheliaAuth(w http.ResponseWriter, r *http.Request) error {
	client := &http.Client{}

	if ar.options.AutheliaURL == "" {
		ar.options.Logger.PrintAndLog("Authelia", "Authelia URL not set", nil)
		w.WriteHeader(500)
		w.Write([]byte("500 - Internal Server Error"))
		return errors.New("authelia URL not set")
	}
	protocol := "http"
	if ar.options.UseHTTPS {
		protocol = "https"
	}

	autheliaBaseURL := protocol + "://" + ar.options.AutheliaURL
	//Remove tailing slash if any
	if autheliaBaseURL[len(autheliaBaseURL)-1] == '/' {
		autheliaBaseURL = autheliaBaseURL[:len(autheliaBaseURL)-1]
	}

	//Make a request to Authelia to verify the request
	req, err := http.NewRequest("POST", autheliaBaseURL+"/api/verify", nil)
	if err != nil {
		ar.options.Logger.PrintAndLog("Authelia", "Unable to create request", err)
		w.WriteHeader(401)
		return errors.New("unauthorized")
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	req.Header.Add("X-Original-URL", fmt.Sprintf("%s://%s", scheme, r.Host))

	// Copy cookies from the incoming request
	for _, cookie := range r.Cookies() {
		req.AddCookie(cookie)
	}

	// Making the verification request
	resp, err := client.Do(req)
	if err != nil {
		ar.options.Logger.PrintAndLog("Authelia", "Unable to verify", err)
		w.WriteHeader(401)
		return errors.New("unauthorized")
	}

	if resp.StatusCode != 200 {
		redirectURL := autheliaBaseURL + "/?rd=" + url.QueryEscape(scheme+"://"+r.Host+r.URL.String()) + "&rm=" + r.Method
		http.Redirect(w, r, redirectURL, http.StatusSeeOther)
		return errors.New("unauthorized")
	}

	return nil
}
