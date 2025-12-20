package oauth2

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"golang.org/x/oauth2"
	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/utils"
)

const (
	// DefaultOAuth2ConfigCacheTime defines the default cache duration for OAuth2 configuration
	DefaultOAuth2ConfigCacheTime = 60 * time.Second
)

type OAuth2RouterOptions struct {
	OAuth2ServerURL              string //The URL of the OAuth 2.0 server server
	OAuth2TokenURL               string //The URL of the OAuth 2.0 token server
	OAuth2ClientId               string //The client id for OAuth 2.0 Application
	OAuth2ClientSecret           string //The client secret for OAuth 2.0 Application
	OAuth2WellKnownUrl           string //The well-known url for OAuth 2.0 server
	OAuth2UserInfoUrl            string //The URL of the OAuth 2.0 user info endpoint
	OAuth2Scopes                 string //The scopes for OAuth 2.0 Application
	OAuth2CodeChallengeMethod    string //The authorization code challenge method
	OAuth2ConfigurationCacheTime *time.Duration
	Logger                       *logger.Logger
	Database                     *database.Database
	OAuth2ConfigCache            *ttlcache.Cache[string, *oauth2.Config]
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
	options.Database.Read("oauth2", "oauth2UserInfoUrl", &options.OAuth2UserInfoUrl)
	options.Database.Read("oauth2", "oauth2CodeChallengeMethod", &options.OAuth2CodeChallengeMethod)
	options.Database.Read("oauth2", "oauth2Scopes", &options.OAuth2Scopes)
	options.Database.Read("oauth2", "oauth2ConfigurationCacheTime", &options.OAuth2ConfigurationCacheTime)

	ar := &OAuth2Router{
		options: options,
	}

	if options.OAuth2ConfigurationCacheTime == nil ||
		options.OAuth2ConfigurationCacheTime.Seconds() == 0 {
		cacheTime := DefaultOAuth2ConfigCacheTime
		options.OAuth2ConfigurationCacheTime = &cacheTime
	}

	options.OAuth2ConfigCache = ttlcache.New[string, *oauth2.Config](
		ttlcache.WithTTL[string, *oauth2.Config](*options.OAuth2ConfigurationCacheTime),
	)
	go options.OAuth2ConfigCache.Start()

	return ar
}

// HandleSetOAuth2Settings is the internal handler for setting the OAuth URL and HTTPS
func (ar *OAuth2Router) HandleSetOAuth2Settings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ar.handleSetOAuthSettingsGET(w, r)
	case http.MethodPost:
		ar.handleSetOAuthSettingsPOST(w, r)
	case http.MethodDelete:
		ar.handleSetOAuthSettingsDELETE(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (ar *OAuth2Router) handleSetOAuthSettingsGET(w http.ResponseWriter, r *http.Request) {
	//Return the current settings
	js, _ := json.Marshal(map[string]interface{}{
		"oauth2WellKnownUrl":           ar.options.OAuth2WellKnownUrl,
		"oauth2ServerUrl":              ar.options.OAuth2ServerURL,
		"oauth2TokenUrl":               ar.options.OAuth2TokenURL,
		"oauth2UserInfoUrl":            ar.options.OAuth2UserInfoUrl,
		"oauth2Scopes":                 ar.options.OAuth2Scopes,
		"oauth2ClientSecret":           ar.options.OAuth2ClientSecret,
		"oauth2ClientId":               ar.options.OAuth2ClientId,
		"oauth2CodeChallengeMethod":    ar.options.OAuth2CodeChallengeMethod,
		"oauth2ConfigurationCacheTime": ar.options.OAuth2ConfigurationCacheTime.String(),
	})

	utils.SendJSONResponse(w, string(js))
}

func (ar *OAuth2Router) handleSetOAuthSettingsPOST(w http.ResponseWriter, r *http.Request) {
	//Update the settings
	var oauth2ServerUrl, oauth2TokenURL, oauth2Scopes, oauth2UserInfoUrl, oauth2CodeChallengeMethod string
	var oauth2ConfigurationCacheTime *time.Duration

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

	oauth2CodeChallengeMethod, err = utils.PostPara(r, "oauth2CodeChallengeMethod")
	if err != nil {
		utils.SendErrorResponse(w, "oauth2CodeChallengeMethod not found")
		return
	}

	oauth2ConfigurationCacheTime, err = utils.PostDuration(r, "oauth2ConfigurationCacheTime")
	if err != nil {
		utils.SendErrorResponse(w, "oauth2ConfigurationCacheTime not found")
		return
	}

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

		oauth2UserInfoUrl, err = utils.PostPara(r, "oauth2UserInfoUrl")
		if err != nil {
			utils.SendErrorResponse(w, "oauth2UserInfoUrl not found")
			return
		}

		oauth2Scopes, err = utils.PostPara(r, "oauth2Scopes")
		if err != nil {
			utils.SendErrorResponse(w, "oauth2Scopes not found")
			return
		}
	} else {
		oauth2Scopes, _ = utils.PostPara(r, "oauth2Scopes")
	}

	//Write changes to runtime
	ar.options.OAuth2WellKnownUrl = oauth2WellKnownUrl
	ar.options.OAuth2ServerURL = oauth2ServerUrl
	ar.options.OAuth2TokenURL = oauth2TokenURL
	ar.options.OAuth2UserInfoUrl = oauth2UserInfoUrl
	ar.options.OAuth2ClientId = oauth2ClientId
	ar.options.OAuth2ClientSecret = oauth2ClientSecret
	ar.options.OAuth2Scopes = oauth2Scopes
	ar.options.OAuth2CodeChallengeMethod = oauth2CodeChallengeMethod
	ar.options.OAuth2ConfigurationCacheTime = oauth2ConfigurationCacheTime

	//Write changes to database
	ar.options.Database.Write("oauth2", "oauth2WellKnownUrl", oauth2WellKnownUrl)
	ar.options.Database.Write("oauth2", "oauth2ServerUrl", oauth2ServerUrl)
	ar.options.Database.Write("oauth2", "oauth2TokenUrl", oauth2TokenURL)
	ar.options.Database.Write("oauth2", "oauth2UserInfoUrl", oauth2UserInfoUrl)
	ar.options.Database.Write("oauth2", "oauth2ClientId", oauth2ClientId)
	ar.options.Database.Write("oauth2", "oauth2ClientSecret", oauth2ClientSecret)
	ar.options.Database.Write("oauth2", "oauth2Scopes", oauth2Scopes)
	ar.options.Database.Write("oauth2", "oauth2CodeChallengeMethod", oauth2CodeChallengeMethod)
	ar.options.Database.Write("oauth2", "oauth2ConfigurationCacheTime", oauth2ConfigurationCacheTime)

	// Flush caches
	ar.options.OAuth2ConfigCache.DeleteAll()

	utils.SendOK(w)
}

func (ar *OAuth2Router) handleSetOAuthSettingsDELETE(w http.ResponseWriter, r *http.Request) {
	ar.options.OAuth2WellKnownUrl = ""
	ar.options.OAuth2ServerURL = ""
	ar.options.OAuth2TokenURL = ""
	ar.options.OAuth2UserInfoUrl = ""
	ar.options.OAuth2ClientId = ""
	ar.options.OAuth2ClientSecret = ""
	ar.options.OAuth2Scopes = ""
	ar.options.OAuth2CodeChallengeMethod = ""

	ar.options.Database.Delete("oauth2", "oauth2WellKnownUrl")
	ar.options.Database.Delete("oauth2", "oauth2ServerUrl")
	ar.options.Database.Delete("oauth2", "oauth2TokenUrl")
	ar.options.Database.Delete("oauth2", "oauth2UserInfoUrl")
	ar.options.Database.Delete("oauth2", "oauth2ClientId")
	ar.options.Database.Delete("oauth2", "oauth2ClientSecret")
	ar.options.Database.Delete("oauth2", "oauth2Scopes")
	ar.options.Database.Delete("oauth2", "oauth2CodeChallengeMethod")
	ar.options.Database.Delete("oauth2", "oauth2ConfigurationCacheTime")

	utils.SendOK(w)
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
	const verifierCookie = "z-verifier"
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	reqUrl := scheme + "://" + r.Host + r.RequestURI
	oauthConfigCache, _ := ar.options.OAuth2ConfigCache.GetOrSetFunc(r.Host, func() *oauth2.Config {
		oauthConfig, err := ar.newOAuth2Conf(scheme + "://" + r.Host + callbackPrefix)
		if err != nil {
			ar.options.Logger.PrintAndLog("OAuth2Router", "Failed to fetch OIDC configuration:", err)
			return nil
		}
		return oauthConfig
	})

	oauthConfig := oauthConfigCache.Value()
	if oauthConfig == nil {
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
		var authCodeOptions []oauth2.AuthCodeOption
		if ar.options.OAuth2CodeChallengeMethod == "PKCE" || ar.options.OAuth2CodeChallengeMethod == "PKCE_S256" {
			verifierCookie, err := r.Cookie(verifierCookie)
			if err != nil || verifierCookie.Value == "" {
				ar.options.Logger.PrintAndLog("OAuth2Router", "Read OAuth2 verifier cookie failed", err)
				w.WriteHeader(401)
				return errors.New("unauthorized")
			}
			authCodeOptions = append(authCodeOptions, oauth2.VerifierOption(verifierCookie.Value))
		}
		token, err := oauthConfig.Exchange(ctx, code, authCodeOptions...)
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

		cookieExpiry := token.Expiry
		if cookieExpiry.IsZero() || cookieExpiry.Before(time.Now()) {
			cookieExpiry = time.Now().Add(time.Hour)
		}
		cookie := http.Cookie{Name: tokenCookie, Value: token.AccessToken, Path: "/", Expires: cookieExpiry}
		if scheme == "https" {
			cookie.Secure = true
			cookie.SameSite = http.SameSiteLaxMode
		}
		w.Header().Add("Set-Cookie", cookie.String())

		if ar.options.OAuth2CodeChallengeMethod == "PKCE" || ar.options.OAuth2CodeChallengeMethod == "PKCE_S256" {
			cookie := http.Cookie{Name: verifierCookie, Value: "", Path: "/", Expires: time.Now().Add(-time.Hour * 1)}
			if scheme == "https" {
				cookie.Secure = true
				cookie.SameSite = http.SameSiteLaxMode
			}
			w.Header().Add("Set-Cookie", cookie.String())
		}

		//Fix for #695
		location := strings.TrimPrefix(state, "/internal/")
		//Check if the location starts with http:// or https://. if yes, this is full URL
		decodedLocation, err := url.PathUnescape(location)
		if err == nil && (strings.HasPrefix(decodedLocation, "http://") || strings.HasPrefix(decodedLocation, "https://")) {
			//Redirect to the full URL
			http.Redirect(w, r, decodedLocation, http.StatusTemporaryRedirect)
		} else {
			//Redirect to a relative path
			http.Redirect(w, r, state, http.StatusTemporaryRedirect)
		}

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
		var url string
		if ar.options.OAuth2CodeChallengeMethod == "PKCE" || ar.options.OAuth2CodeChallengeMethod == "PKCE_S256" {
			cookie := http.Cookie{Name: verifierCookie, Value: oauth2.GenerateVerifier(), Path: "/", Expires: time.Now().Add(time.Hour * 1)}
			if scheme == "https" {
				cookie.Secure = true
				cookie.SameSite = http.SameSiteLaxMode
			}

			w.Header().Add("Set-Cookie", cookie.String())

			if ar.options.OAuth2CodeChallengeMethod == "PKCE" {
				url = oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("code_challenge", cookie.Value))
			} else {
				url = oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.S256ChallengeOption(cookie.Value))
			}
		} else {
			url = oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
		}

		http.Redirect(w, r, url, http.StatusFound)

		return errors.New("unauthorized")
	}
	return nil
}
