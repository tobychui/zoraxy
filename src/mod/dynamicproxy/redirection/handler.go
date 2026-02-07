package redirection

import (
	"errors"
	"net/http"
	"strings"
)

/*
	handler.go

	This script store the handlers use for handling
	redirection request
*/

// detectDeviceType determines if the request is from a mobile or desktop device
// based on the User-Agent header
func detectDeviceType(r *http.Request) string {
	userAgent := strings.ToLower(r.Header.Get("User-Agent"))
	
	// List of common mobile device indicators
	mobileIndicators := []string{
		"mobile", "android", "iphone", "ipad", "ipod", "blackberry",
		"windows phone", "webos", "opera mini", "opera mobi",
	}
	
	for _, indicator := range mobileIndicators {
		if strings.Contains(userAgent, indicator) {
			return "mobile"
		}
	}
	
	return "desktop"
}

// Check if a request URL is a redirectable URI and device type matches
func (t *RuleTable) IsRedirectable(r *http.Request) bool {
	requestPath := r.Host + r.URL.Path
	rr := t.MatchRedirectRule(requestPath)
	
	if rr == nil {
		return false
	}
	
	// Check if device type matches
	if rr.DeviceType == "" || rr.DeviceType == "all" {
		return true
	}
	
	deviceType := detectDeviceType(r)
	return rr.DeviceType == deviceType
}

// Handle the redirect request, return after calling this function to prevent
// multiple write to the response writer
// Return the status code of the redirection handling
func (t *RuleTable) HandleRedirect(w http.ResponseWriter, r *http.Request) int {
	requestPath := r.Host + r.URL.Path
	rr := t.MatchRedirectRule(requestPath)
	if rr != nil {
		redirectTarget := rr.TargetURL

		if rr.ForwardChildpath {
			//Remove the first / in the path if the redirect target already have tailing slash
			if strings.HasSuffix(redirectTarget, "/") {
				redirectTarget += strings.TrimPrefix(r.URL.Path, "/")
			} else {
				redirectTarget += r.URL.Path
			}

			if r.URL.RawQuery != "" {
				redirectTarget += "?" + r.URL.RawQuery
			}
		}

		if !strings.HasPrefix(redirectTarget, "http://") && !strings.HasPrefix(redirectTarget, "https://") {
			redirectTarget = "http://" + redirectTarget
		}

		http.Redirect(w, r, redirectTarget, rr.StatusCode)
		return rr.StatusCode
	} else {
		//Invalid usage
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("500 - Internal Server Error"))
		t.log("Target request URL do not have matching redirect rule. Check with IsRedirectable before calling HandleRedirect!", errors.New("invalid usage"))
		return 500
	}
}
