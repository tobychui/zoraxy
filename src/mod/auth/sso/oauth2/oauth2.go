package oauth2

import (
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/oauth2"
	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/utils"
	"net/http"
	"net/url"
	"strings"
)

type OAuth2RouterOptions struct {
	OAuth2ServerURL    string //The URL of the OAuth 2.0 server server
	OAuth2TokenURL     string //The URL of the OAuth 2.0 token server
	OAuth2RedirectUrl  string //The redirect URL of the OAuth 2.0 token server
	OAuth2ClientId     string //The client id for OAuth 2.0 Application
	OAuth2ClientSecret string //The client secret for OAuth 2.0 Application
	OAuth2WellKnownUrl string //The well-known url for OAuth 2.0 server
	OAuth2UserInfoUrl  string //The URL of the OAuth 2.0 user info endpoint
	OAuth2Scopes       string //The scopes for OAuth 2.0 Application
	Logger             *logger.Logger
	Database           *database.Database
}

type OIDCDiscoveryDocument struct {
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	ClaimsSupported                   []string `json:"claims_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	IDTokenSigningAlgValuesSupported  []string `json:"id_token_signing_alg_values_supported"`
	Issuer                            string   `json:"issuer"`
	JwksURI                           string   `json:"jwks_uri"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	ScopesSupported                   []string `json:"scopes_supported"`
	SubjectTypesSupported             []string `json:"subject_types_supported"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported"`
	UserinfoEndpoint                  string   `json:"userinfo_endpoint"`
}

type OAuth2Router struct {
	options *OAuth2RouterOptions
}

// NewOAuth2Router creates a new OAuth2Router object
func NewOAuth2Router(options *OAuth2RouterOptions) *OAuth2Router {
	options.Database.NewTable("oauth2")

	//Read settings from database, if exists
	options.Database.Read("oauth2", "oauth2WellKnownUrl", &options.OAuth2WellKnownUrl)
	options.Database.Read("oauth2", "oauth2ServerUrl", &options.OAuth2ServerURL)
	options.Database.Read("oauth2", "oauth2TokenUrl", &options.OAuth2TokenURL)
	options.Database.Read("oauth2", "oauth2ClientId", &options.OAuth2ClientId)
	options.Database.Read("oauth2", "oauth2ClientSecret", &options.OAuth2ClientSecret)
	options.Database.Read("oauth2", "oauth2RedirectURL", &options.OAuth2RedirectUrl)
	options.Database.Read("oauth2", "oauth2UserInfoUrl", &options.OAuth2UserInfoUrl)
	options.Database.Read("oauth2", "oauth2Scopes", &options.OAuth2Scopes)

	return &OAuth2Router{
		options: options,
	}
}

// HandleSetOAuth2Settings is the internal handler for setting the OAuth URL and HTTPS
func (ar *OAuth2Router) HandleSetOAuth2Settings(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		//Return the current settings
		js, _ := json.Marshal(map[string]interface{}{
			"oauth2WellKnownUrl": ar.options.OAuth2WellKnownUrl,
			"oauth2ServerUrl":    ar.options.OAuth2ServerURL,
			"oauth2TokenUrl":     ar.options.OAuth2TokenURL,
			"oauth2Scopes":       ar.options.OAuth2Scopes,
			"oauth2ClientSecret": ar.options.OAuth2ClientSecret,
			"oauth2RedirectURL":  ar.options.OAuth2RedirectUrl,
			"oauth2UserInfoURL":  ar.options.OAuth2UserInfoUrl,
			"oauth2ClientId":     ar.options.OAuth2ClientId,
		})

		utils.SendJSONResponse(w, string(js))
		return
	} else if r.Method == http.MethodPost {
		//Update the settings
		var oauth2ServerUrl, oauth2TokenURL, oauth2Scopes, oauth2UserInfoUrl string
		oauth2WellKnownUrl, err := utils.PostPara(r, "oauth2WellKnownUrl")
		if err != nil {
			oauth2ServerUrl, err = utils.PostPara(r, "oauth2ServerUrl")
			if err != nil {
				utils.SendErrorResponse(w, "oauth2ServerUrl not found")
				return
			}

			oauth2TokenURL, err = utils.PostPara(r, "oauth2TokenUrl")
			if err != nil {
				utils.SendErrorResponse(w, "oauth2TokenUrl not found")
				return
			}

			oauth2Scopes, err = utils.PostPara(r, "oauth2Scopes")
			if err != nil {
				utils.SendErrorResponse(w, "oauth2Scopes not found")
				return
			}

			oauth2UserInfoUrl, err = utils.PostPara(r, "oauth2UserInfoUrl")
			if err != nil {
				utils.SendErrorResponse(w, "oauth2UserInfoUrl not found")
				return
			}
		}

		oauth2RedirectUrl, err := utils.PostPara(r, "oauth2RedirectUrl")
		if err != nil {
			utils.SendErrorResponse(w, "oauth2RedirectUrl not found")
			return
		}

		oauth2ClientId, err := utils.PostPara(r, "oauth2ClientId")
		if err != nil {
			utils.SendErrorResponse(w, "oauth2ClientId not found")
			return
		}

		oauth2ClientSecret, err := utils.PostPara(r, "oauth2ClientSecret")
		if err != nil {
			utils.SendErrorResponse(w, "oauth2ClientSecret not found")
			return
		}

		//Write changes to runtime
		ar.options.OAuth2WellKnownUrl = oauth2WellKnownUrl
		ar.options.OAuth2ServerURL = oauth2ServerUrl
		ar.options.OAuth2TokenURL = oauth2TokenURL
		ar.options.OAuth2RedirectUrl = oauth2RedirectUrl
		ar.options.OAuth2UserInfoUrl = oauth2UserInfoUrl
		ar.options.OAuth2ClientId = oauth2ClientId
		ar.options.OAuth2ClientSecret = oauth2ClientSecret
		ar.options.OAuth2Scopes = oauth2Scopes

		//Write changes to database
		ar.options.Database.Write("oauth2", "oauth2WellKnownUrl", oauth2WellKnownUrl)
		ar.options.Database.Write("oauth2", "oauth2ServerUrl", oauth2ServerUrl)
		ar.options.Database.Write("oauth2", "oauth2TokenUrl", oauth2TokenURL)
		ar.options.Database.Write("oauth2", "oauth2RedirectUrl", oauth2RedirectUrl)
		ar.options.Database.Write("oauth2", "oauth2UserInfoUrl", oauth2UserInfoUrl)
		ar.options.Database.Write("oauth2", "oauth2ClientId", oauth2ClientId)
		ar.options.Database.Write("oauth2", "oauth2ClientSecret", oauth2ClientSecret)
		ar.options.Database.Write("oauth2", "oauth2Scopes", oauth2Scopes)

		utils.SendOK(w)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

}

func (ar *OAuth2Router) fetchOAuth2Configuration(config *oauth2.Config) (*oauth2.Config, error) {
	req, err := http.NewRequest("GET", ar.options.OAuth2WellKnownUrl, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{}
	if resp, err := client.Do(req); err != nil {
		return nil, err
	} else {
		defer resp.Body.Close()

		oidcDiscoveryDocument := OIDCDiscoveryDocument{}
		if err := json.NewDecoder(resp.Body).Decode(&oidcDiscoveryDocument); err != nil {
			return nil, err
		}

		if len(config.Scopes) == 0 {
			config.Scopes = oidcDiscoveryDocument.ScopesSupported
		}

		if config.Endpoint.AuthURL == "" {
			config.Endpoint.AuthURL = oidcDiscoveryDocument.AuthorizationEndpoint
		}

		if config.Endpoint.TokenURL == "" {
			config.Endpoint.TokenURL = oidcDiscoveryDocument.TokenEndpoint
		}

		if ar.options.OAuth2UserInfoUrl == "" {
			ar.options.OAuth2UserInfoUrl = oidcDiscoveryDocument.UserinfoEndpoint
		}

	}
	return config, nil
}

func (ar *OAuth2Router) newOAuth2Conf(redirectUrl string) (*oauth2.Config, error) {
	config := &oauth2.Config{
		ClientID:     ar.options.OAuth2ClientId,
		ClientSecret: ar.options.OAuth2ClientSecret,
		RedirectURL:  redirectUrl,
		Endpoint: oauth2.Endpoint{
			AuthURL:  ar.options.OAuth2ServerURL,
			TokenURL: ar.options.OAuth2TokenURL,
		},
	}
	if ar.options.OAuth2Scopes != "" {
		config.Scopes = strings.Split(ar.options.OAuth2Scopes, ",")
	}
	if ar.options.OAuth2WellKnownUrl != "" && (config.Endpoint.AuthURL == "" || config.Endpoint.TokenURL == "" ||
		ar.options.OAuth2UserInfoUrl == "") {
		return ar.fetchOAuth2Configuration(config)
	}
	return config, nil
}

// HandleOAuth2Auth is the internal handler for OAuth authentication
// Set useHTTPS to true if your OAuth server is using HTTPS
// Set OAuthURL to the URL of the OAuth server, e.g. OAuth.example.com
func (ar *OAuth2Router) HandleOAuth2Auth(w http.ResponseWriter, r *http.Request) error {
	const callbackPrefix = "/internal/oauth2"
	const tokenCookie = "z-token"
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	reqUrl := scheme + "://" + r.Host + r.RequestURI
	oauthConfig, err := ar.newOAuth2Conf(scheme + "://" + r.Host + callbackPrefix)
	if err != nil {
		ar.options.Logger.PrintAndLog("OAuth2Router", "Failed to fetch OIDC configuration:", err)
		w.WriteHeader(500)
		return errors.New("failed to fetch OIDC configuration")
	}

	if oauthConfig.Endpoint.AuthURL == "" || oauthConfig.Endpoint.TokenURL == "" || ar.options.OAuth2UserInfoUrl == "" {
		ar.options.Logger.PrintAndLog("OAuth2Router", "Invalid OAuth2 configuration", nil)
		w.WriteHeader(500)
		return errors.New("invalid OAuth2 configuration")
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	if r.Method == http.MethodGet && strings.HasPrefix(r.RequestURI, callbackPrefix) && code != "" && state != "" {
		ctx := context.Background()
		token, err := oauthConfig.Exchange(ctx, code)
		if err != nil {
			ar.options.Logger.PrintAndLog("OAuth2", "Token exchange failed", err)
			w.WriteHeader(401)
			return errors.New("unauthorized")
		}

		if !token.Valid() {
			ar.options.Logger.PrintAndLog("OAuth2", "Invalid token", err)
			w.WriteHeader(401)
			return errors.New("unauthorized")
		}

		cookie := http.Cookie{Name: tokenCookie, Value: token.AccessToken, Path: "/"}
		if scheme == "https" {
			cookie.Secure = true
			cookie.SameSite = http.SameSiteLaxMode
		}
		w.Header().Add("Set-Cookie", cookie.String())
		http.Redirect(w, r, state, http.StatusTemporaryRedirect)
		return errors.New("authorized")
	}
	unauthorized := false
	cookie, err := r.Cookie(tokenCookie)
	if err == nil {
		if cookie.Value == "" {
			unauthorized = true
		} else {
			ctx := context.Background()
			client := oauthConfig.Client(ctx, &oauth2.Token{AccessToken: cookie.Value})
			req, err := client.Get(ar.options.OAuth2UserInfoUrl)
			if err != nil {
				ar.options.Logger.PrintAndLog("OAuth2", "Failed to get user info", err)
				unauthorized = true
			}
			defer req.Body.Close()
			if req.StatusCode != http.StatusOK {
				ar.options.Logger.PrintAndLog("OAuth2", "Failed to get user info", err)
				unauthorized = true
			}
		}
	} else {
		unauthorized = true
	}
	if unauthorized {
		state := url.QueryEscape(reqUrl)
		url := oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
		http.Redirect(w, r, url, http.StatusFound)

		return errors.New("unauthorized")
	}
	return nil
}
