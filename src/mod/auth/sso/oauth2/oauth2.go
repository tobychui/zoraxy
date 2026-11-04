package oauth2

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
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
	OAuth2DefaultAllowedGroups   string //Comma-separated groups required by default for OAuth2 routes
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
	options.Database.Read("oauth2", "oauth2DefaultAllowedGroups", &options.OAuth2DefaultAllowedGroups)
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
		"oauth2DefaultAllowedGroups":   ar.options.OAuth2DefaultAllowedGroups,
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

	// Allowed groups are configured globally but routes can override them.
	defaultAllowedGroups, _ := utils.PostPara(r, "oauth2DefaultAllowedGroups")
	defaultAllowedGroups = strings.Join(SplitAllowedGroups(defaultAllowedGroups), ",")

	//Write changes to runtime
	ar.options.OAuth2WellKnownUrl = oauth2WellKnownUrl
	ar.options.OAuth2ServerURL = oauth2ServerUrl
	ar.options.OAuth2TokenURL = oauth2TokenURL
	ar.options.OAuth2UserInfoUrl = oauth2UserInfoUrl
	ar.options.OAuth2ClientId = oauth2ClientId
	ar.options.OAuth2ClientSecret = oauth2ClientSecret
	ar.options.OAuth2Scopes = oauth2Scopes
	ar.options.OAuth2DefaultAllowedGroups = defaultAllowedGroups
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
	ar.options.Database.Write("oauth2", "oauth2DefaultAllowedGroups", defaultAllowedGroups)
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
	ar.options.OAuth2DefaultAllowedGroups = ""
	ar.options.OAuth2CodeChallengeMethod = ""

	ar.options.Database.Delete("oauth2", "oauth2WellKnownUrl")
	ar.options.Database.Delete("oauth2", "oauth2ServerUrl")
	ar.options.Database.Delete("oauth2", "oauth2TokenUrl")
	ar.options.Database.Delete("oauth2", "oauth2UserInfoUrl")
	ar.options.Database.Delete("oauth2", "oauth2ClientId")
	ar.options.Database.Delete("oauth2", "oauth2ClientSecret")
	ar.options.Database.Delete("oauth2", "oauth2Scopes")
	ar.options.Database.Delete("oauth2", "oauth2DefaultAllowedGroups")
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
			ar.options.Logger.PrintAndLog("OAuth2Router", fmt.Sprintf("Failed to decode ([%d] %s)", resp.StatusCode, resp.Status), err)
			return nil, err
		}
		// Discovery describes supported scopes, not a scope request. Never request all of them implicitly.

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
		config.Scopes = strings.FieldsFunc(ar.options.OAuth2Scopes, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' })
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
func (ar *OAuth2Router) HandleOAuth2Auth(w http.ResponseWriter, r *http.Request, policy *AccessPolicy) error {
	if policy == nil {
		policy = &AccessPolicy{ForwardUserInfo: true, RemoveTokenCookie: true}
	}
	const callbackPrefix = "/internal/oauth2"
	const tokenCookie = "z-token"
	const verifierCookie = "z-verifier"
	const stateCookie = "z-oauth-state"
	const returnCookie = "z-oauth-return"
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

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
	if r.URL.Path == callbackPrefix {
		validState, stateErr := r.Cookie(stateCookie)
		originalReturn, returnErr := r.Cookie(returnCookie)
		if r.Method != http.MethodGet || code == "" || state == "" || stateErr != nil ||
			returnErr != nil || subtle.ConstantTimeCompare([]byte(state), []byte(validState.Value)) != 1 {
			http.Error(w, "Invalid OAuth2 callback state", http.StatusUnauthorized)
			return errors.New("invalid callback state")
		}
		originalPath, decodeErr := url.QueryUnescape(originalReturn.Value)
		if decodeErr != nil || !strings.HasPrefix(originalPath, "/") || strings.HasPrefix(originalPath, "//") ||
			strings.ContainsAny(originalPath, "\r\n") || strings.HasPrefix(originalPath, callbackPrefix) {
			http.Error(w, "Invalid OAuth2 return path", http.StatusUnauthorized)
			return errors.New("invalid return path")
		}
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
		cookie := http.Cookie{Name: tokenCookie, Value: token.AccessToken, Path: "/", Expires: cookieExpiry, HttpOnly: true}
		if scheme == "https" {
			cookie.Secure = true
			cookie.SameSite = http.SameSiteLaxMode
		}
		w.Header().Add("Set-Cookie", cookie.String())

		if ar.options.OAuth2CodeChallengeMethod == "PKCE" || ar.options.OAuth2CodeChallengeMethod == "PKCE_S256" {
			cookie := http.Cookie{Name: verifierCookie, Value: "", Path: "/", Expires: time.Now().Add(-time.Hour * 1), HttpOnly: true}
			if scheme == "https" {
				cookie.Secure = true
				cookie.SameSite = http.SameSiteLaxMode
			}
			w.Header().Add("Set-Cookie", cookie.String())
		}

		// State and return destination are short-lived and bound to this host.
		for _, name := range []string{stateCookie, returnCookie} {
			http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: scheme == "https", SameSite: http.SameSiteLaxMode})
		}
		http.Redirect(w, r, originalPath, http.StatusSeeOther)

		return errors.New("authorized")
	}
	unauthorized := false
	cookie, err := r.Cookie(tokenCookie)
	if err == nil {
		if cookie.Value == "" {
			unauthorized = true
		} else {
			ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
			defer cancel()
			client := oauthConfig.Client(ctx, &oauth2.Token{AccessToken: cookie.Value})
			identity, fetchErr := FetchIdentity(ctx, client, ar.options.OAuth2UserInfoUrl)
			if fetchErr != nil {
				ar.options.Logger.PrintAndLog("OAuth2", "Failed to validate user info", fetchErr)
				unauthorized = true
			} else {
				allowedGroups := policy.AllowedGroups
				if len(allowedGroups) == 0 {
					allowedGroups = SplitAllowedGroups(ar.options.OAuth2DefaultAllowedGroups)
				}
				if !GroupsAllowed(identity.Groups, allowedGroups) {
					http.Error(w, "403 - Access denied: insufficient group permissions", http.StatusForbidden)
					return errors.New("forbidden")
				}
				isWebSocket := strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
				if policy.ForwardUserInfo && (!isWebSocket || policy.ForwardWebSocketIdentity) {
					SetIdentityHeaders(r.Header, identity, policy.ForwardedClaims)
				}
				StripOAuth2Cookies(r, policy.RemoveTokenCookie)
			}
		}
	} else {
		unauthorized = true
	}
	if unauthorized {
		state := oauth2.GenerateVerifier()
		originalPath := r.URL.RequestURI()
		if originalPath == "" || len(originalPath) > 4096 || !strings.HasPrefix(originalPath, "/") || strings.HasPrefix(originalPath, "//") {
			originalPath = "/"
		}
		http.SetCookie(w, &http.Cookie{Name: stateCookie, Value: state, Path: "/", Expires: time.Now().Add(10*time.Minute), HttpOnly: true, Secure: scheme == "https", SameSite: http.SameSiteLaxMode})
		http.SetCookie(w, &http.Cookie{Name: returnCookie, Value: url.QueryEscape(originalPath), Path: "/", Expires: time.Now().Add(10*time.Minute), HttpOnly: true, Secure: scheme == "https", SameSite: http.SameSiteLaxMode})
		var authURL string
		if ar.options.OAuth2CodeChallengeMethod == "PKCE" || ar.options.OAuth2CodeChallengeMethod == "PKCE_S256" {
			cookie := http.Cookie{Name: verifierCookie, Value: oauth2.GenerateVerifier(), Path: "/", Expires: time.Now().Add(time.Hour * 1), HttpOnly: true}
			if scheme == "https" {
				cookie.Secure = true
				cookie.SameSite = http.SameSiteLaxMode
			}

			w.Header().Add("Set-Cookie", cookie.String())

			if ar.options.OAuth2CodeChallengeMethod == "PKCE" {
				authURL = oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("code_challenge", cookie.Value))
			} else {
				authURL = oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.S256ChallengeOption(cookie.Value))
			}
		} else {
			authURL = oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
		}

		http.Redirect(w, r, authURL, http.StatusFound)

		return errors.New("unauthorized")
	}
	return nil
}
