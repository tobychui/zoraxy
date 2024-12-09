package dynamicproxy

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

func (h *ProxyHandler) handleAutheliaAuthRouting(w http.ResponseWriter, r *http.Request, pe *ProxyEndpoint) error {
	err := handleAutheliaAuth(w, r, pe)
	if err != nil {
		h.Parent.logRequest(r, false, 401, "host", r.URL.Hostname())
	}
	return err
}

func handleAutheliaAuth(w http.ResponseWriter, r *http.Request, pe *ProxyEndpoint) error {

	client := &http.Client{}

	// TODO: provide authelia url by config variable
	req, err := http.NewRequest("POST", "https://authelia.mydomain.com/api/verify", nil)
	if err != nil {
		pe.parent.Option.Logger.PrintAndLog("Authelia", "Unable to create request", err)
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

	resp, err := client.Do(req)
	if err != nil {
		pe.parent.Option.Logger.PrintAndLog("Authelia", "Unable to verify", err)
		w.WriteHeader(401)
		return errors.New("unauthorized")
	}

	if resp.StatusCode != 200 {
		// TODO: provide authelia url by config variable
		redirectURL := "https://authelia.mydomain.com/?rd=" + url.QueryEscape(scheme+"://"+r.Host+r.URL.String()) + "&rm=" + r.Method

		http.Redirect(w, r, redirectURL, http.StatusSeeOther)
		return errors.New("unauthorized")
	}

	return nil
}
