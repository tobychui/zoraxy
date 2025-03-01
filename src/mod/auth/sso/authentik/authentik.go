package authentik

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/utils"
)

type AuthentikRouterOptions struct {
	UseHTTPS     bool   //If the Authentik server is using HTTPS
	AuthentikURL string //The URL of the Authentik server
	Logger       *logger.Logger
	Database     *database.Database
}

type AuthentikRouter struct {
	options *AuthentikRouterOptions
}

// NewAuthentikRouter creates a new AuthentikRouter object
func NewAuthentikRouter(options *AuthentikRouterOptions) *AuthentikRouter {
	options.Database.NewTable("authentik")

	//Read settings from database, if exists
	options.Database.Read("authentik", "authentikURL", &options.AuthentikURL)
	options.Database.Read("authentik", "useHTTPS", &options.UseHTTPS)

	return &AuthentikRouter{
		options: options,
	}
}

// HandleSetAuthentikURLAndHTTPS is the internal handler for setting the Authentik URL and HTTPS
func (ar *AuthentikRouter) HandleSetAuthentikURLAndHTTPS(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		//Return the current settings
		js, _ := json.Marshal(map[string]interface{}{
			"useHTTPS":     ar.options.UseHTTPS,
			"authentikURL": ar.options.AuthentikURL,
		})

		utils.SendJSONResponse(w, string(js))
		return
	} else if r.Method == http.MethodPost {
		//Update the settings
		AuthentikURL, err := utils.PostPara(r, "authentikURL")
		if err != nil {
			utils.SendErrorResponse(w, "authentikURL not found")
			return
		}

		useHTTPS, err := utils.PostBool(r, "useHTTPS")
		if err != nil {
			useHTTPS = false
		}

		//Write changes to runtime
		ar.options.AuthentikURL = AuthentikURL
		ar.options.UseHTTPS = useHTTPS

		//Write changes to database
		ar.options.Database.Write("authentik", "authentikURL", AuthentikURL)
		ar.options.Database.Write("authentik", "useHTTPS", useHTTPS)

		utils.SendOK(w)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

}

// HandleAuthentikAuth is the internal handler for Authentik authentication
// Set useHTTPS to true if your Authentik server is using HTTPS
// Set AuthentikURL to the URL of the Authentik server, e.g. Authentik.example.com
func (ar *AuthentikRouter) HandleAuthentikAuth(w http.ResponseWriter, r *http.Request) error {
	const outpostPrefix = "outpost.goauthentik.io"
	client := &http.Client{}

	if ar.options.AuthentikURL == "" {
		ar.options.Logger.PrintAndLog("Authentik", "Authentik URL not set", nil)
		w.WriteHeader(500)
		w.Write([]byte("500 - Internal Server Error"))
		return errors.New("authentik URL not set")
	}
	protocol := "http"
	if ar.options.UseHTTPS {
		protocol = "https"
	}

	authentikBaseURL := protocol + "://" + ar.options.AuthentikURL
	//Remove tailing slash if any
	authentikBaseURL = strings.TrimSuffix(authentikBaseURL, "/")

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	reqUrl := scheme + "://" + r.Host + r.RequestURI
	// Pass request to outpost if path matches outpost prefix
	if reqPath := strings.TrimPrefix(r.URL.Path, "/"); strings.HasPrefix(reqPath, outpostPrefix) {
		req, err := http.NewRequest(r.Method, authentikBaseURL+r.RequestURI, r.Body)
		if err != nil {
			ar.options.Logger.PrintAndLog("Authentik", "Unable to create request", err)
			w.WriteHeader(401)
			return errors.New("unauthorized")
		}
		req.Header.Set("X-Original-URL", reqUrl)
		req.Header.Set("Host", r.Host)
		for _, cookie := range r.Cookies() {
			req.AddCookie(cookie)
		}
		if resp, err := client.Do(req); err != nil {
			ar.options.Logger.PrintAndLog("Authentik", "Unable to pass request to Authentik outpost", err)
			w.WriteHeader(http.StatusInternalServerError)
			return errors.New("internal server error")
		} else {
			defer resp.Body.Close()
			for k := range resp.Header {
				w.Header().Set(k, resp.Header.Get(k))
			}
			w.WriteHeader(resp.StatusCode)
			if _, err = io.Copy(w, resp.Body); err != nil {
				ar.options.Logger.PrintAndLog("Authentik", "Unable to pass Authentik outpost response to client", err)
				w.WriteHeader(http.StatusInternalServerError)
				return errors.New("internal server error")
			}
		}
		return nil
	}

	//Make a request to Authentik to verify the request
	req, err := http.NewRequest(http.MethodGet, authentikBaseURL+"/"+outpostPrefix+"/auth/nginx", nil)
	if err != nil {
		ar.options.Logger.PrintAndLog("Authentik", "Unable to create request", err)
		w.WriteHeader(401)
		return errors.New("unauthorized")
	}

	req.Header.Set("X-Original-URL", reqUrl)

	// Copy cookies from the incoming request
	for _, cookie := range r.Cookies() {
		req.AddCookie(cookie)
	}

	// Making the verification request
	resp, err := client.Do(req)
	if err != nil {
		ar.options.Logger.PrintAndLog("Authentik", "Unable to verify", err)
		w.WriteHeader(401)
		return errors.New("unauthorized")
	}

	if resp.StatusCode != 200 {
		redirectURL := authentikBaseURL + "/" + outpostPrefix + "/start?rd=" + url.QueryEscape(scheme+"://"+r.Host+r.URL.String())
		http.Redirect(w, r, redirectURL, http.StatusSeeOther)
		return errors.New("unauthorized")
	}

	return nil
}
