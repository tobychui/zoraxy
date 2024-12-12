package authelia

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"imuslab.com/zoraxy/mod/dynamicproxy"
	"imuslab.com/zoraxy/mod/info/logger"
)

type Options struct {
	AutheliaURL string //URL of the Authelia server, e.g. authelia.example.com
	UseHTTPS    bool   //Whether to use HTTPS for the Authelia server
	Logger      logger.Logger
}

type AutheliaHandler struct {
	options *Options
}

func NewAutheliaAuthenticator(options *Options) *AutheliaHandler {
	return &AutheliaHandler{
		options: options,
	}
}

// HandleAutheliaAuthRouting is the handler for Authelia authentication, if the error is not nil, the request will be forwarded to the endpoint
// Do not continue processing or write to the response writer if the error is not nil
func (h *AutheliaHandler) HandleAutheliaAuthRouting(w http.ResponseWriter, r *http.Request, pe *dynamicproxy.ProxyEndpoint) error {
	err := h.handleAutheliaAuth(w, r)
	if err != nil {
		return nil
	}
	return err
}

func (h *AutheliaHandler) handleAutheliaAuth(w http.ResponseWriter, r *http.Request) error {
	client := &http.Client{}

	protocol := "http"
	if h.options.UseHTTPS {
		protocol = "https"
	}

	autheliaBaseURL := protocol + "://" + h.options.AutheliaURL
	//Remove tailing slash if any
	if autheliaBaseURL[len(autheliaBaseURL)-1] == '/' {
		autheliaBaseURL = autheliaBaseURL[:len(autheliaBaseURL)-1]
	}

	//Make a request to Authelia to verify the request
	req, err := http.NewRequest("POST", autheliaBaseURL+"/api/verify", nil)
	if err != nil {
		h.options.Logger.PrintAndLog("Authelia", "Unable to create request", err)
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
		h.options.Logger.PrintAndLog("Authelia", "Unable to verify", err)
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
