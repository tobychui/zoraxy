package sso

import (
	"encoding/json"
	"net/http"
	"strings"
)

type OpenIDConfiguration struct {
	Issuer                           string   `json:"issuer"`
	AuthorizationEndpoint            string   `json:"authorization_endpoint"`
	TokenEndpoint                    string   `json:"token_endpoint"`
	JwksUri                          string   `json:"jwks_uri"`
	ResponseTypesSupported           []string `json:"response_types_supported"`
	SubjectTypesSupported            []string `json:"subject_types_supported"`
	IDTokenSigningAlgValuesSupported []string `json:"id_token_signing_alg_values_supported"`
	ClaimsSupported                  []string `json:"claims_supported"`
}

func (h *SSOHandler) HandleDiscoveryRequest(w http.ResponseWriter, r *http.Request) {

	//Prepend https:// if not present
	authBaseURL := h.Config.AuthURL
	if !strings.HasPrefix(authBaseURL, "http://") && !strings.HasPrefix(authBaseURL, "https://") {
		authBaseURL = "https://" + authBaseURL
	}

	//Handle the discovery request
	discovery := OpenIDConfiguration{
		Issuer:                 authBaseURL,
		AuthorizationEndpoint:  authBaseURL + "/oauth2/authorize",
		TokenEndpoint:          authBaseURL + "/oauth2/token",
		JwksUri:                authBaseURL + "/jwks.json",
		ResponseTypesSupported: []string{"code", "token"},
		SubjectTypesSupported:  []string{"public"},
		IDTokenSigningAlgValuesSupported: []string{
			"RS256",
		},
		ClaimsSupported: []string{
			"sub",                //Subject, usually the user ID
			"iss",                //Issuer, usually the server URL
			"aud",                //Audience, usually the client ID
			"exp",                //Expiration Time
			"iat",                //Issued At
			"email",              //Email
			"locale",             //Locale
			"name",               //Full Name
			"nickname",           //Nickname
			"preferred_username", //Preferred Username
			"website",            //Website
		},
	}

	//Write the response
	js, _ := json.Marshal(discovery)
	w.Header().Set("Content-Type", "application/json")
	w.Write(js)
}
