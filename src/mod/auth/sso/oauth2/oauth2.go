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
	"sync"
	"time"

	"github.com/jellydator/ttlcache/v3"
	"golang.org/x/oauth2"
	"golang.org/x/sync/singleflight"
	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/utils"
)

const (
	// DefaultOAuth2ConfigCacheTime defines the default cache duration for OAuth2 configuration
	DefaultOAuth2ConfigCacheTime = 60 * time.Second
	maxOAuth2CookieSize          = 4096
)

var defaultOIDCScopes = []string{"openid", "profile", "email"}

type OAuth2RouterOptions struct {
	settingsGeneration           uint64 // Internal snapshot version; never supplied by callers.
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
	// Protect settings, discovery updates, and configuration cache invalidation.
	settingsMu         sync.RWMutex
	settingsGeneration uint64
	configLoads        singleflight.Group
	options            *OAuth2RouterOptions
	sessions           *ttlcache.Cache[string, oauth2Session]
}

// NewOAuth2Router creates a new OAuth2Router object
func NewOAuth2Router(options *OAuth2RouterOptions) *OAuth2Router {
	options.Database.NewTable("oauth2")

	loadOAuth2Settings(options)

	ar := &OAuth2Router{
		options: options,
		sessions: ttlcache.New[string, oauth2Session](
			ttlcache.WithCapacity[string, oauth2Session](10000),
			ttlcache.WithDisableTouchOnHit[string, oauth2Session](),
		),
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
	ar.settingsMu.RLock()
	defer ar.settingsMu.RUnlock()
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

	ar.settingsMu.Lock()
	defer ar.settingsMu.Unlock()
	settings := persistedOAuth2Settings{
		OAuth2WellKnownUrl:           oauth2WellKnownUrl,
		OAuth2ServerURL:              oauth2ServerUrl,
		OAuth2TokenURL:               oauth2TokenURL,
		OAuth2UserInfoUrl:            oauth2UserInfoUrl,
		OAuth2ClientId:               oauth2ClientId,
		OAuth2ClientSecret:           oauth2ClientSecret,
		OAuth2Scopes:                 oauth2Scopes,
		OAuth2DefaultAllowedGroups:   defaultAllowedGroups,
		OAuth2CodeChallengeMethod:    oauth2CodeChallengeMethod,
		OAuth2ConfigurationCacheTime: oauth2ConfigurationCacheTime,
	}
	if err := ar.saveOAuth2Settings(settings); err != nil {
		ar.options.Logger.PrintAndLog("OAuth2Router", "Failed to save OAuth2 settings", err)
		utils.SendErrorResponse(w, "Failed to save OAuth2 settings")
		return
	}

	utils.SendOK(w)
}

func (ar *OAuth2Router) handleSetOAuthSettingsDELETE(w http.ResponseWriter, r *http.Request) {
	ar.settingsMu.Lock()
	defer ar.settingsMu.Unlock()
	cacheTime := DefaultOAuth2ConfigCacheTime
	// Persist an empty record rather than deleting it, so legacy keys cannot
	// resurrect the previous provider or group policy after a restart.
	if err := ar.saveOAuth2Settings(persistedOAuth2Settings{OAuth2ConfigurationCacheTime: &cacheTime}); err != nil {
		ar.options.Logger.PrintAndLog("OAuth2Router", "Failed to reset OAuth2 settings", err)
		utils.SendErrorResponse(w, "Failed to reset OAuth2 settings")
		return
	}

	utils.SendOK(w)
}

// options is a request-owned snapshot; discovery must never mutate live settings.
func fetchOAuth2Configuration(config *oauth2.Config, options *OAuth2RouterOptions) (*oauth2.Config, error) {
	req, err := http.NewRequest("GET", options.OAuth2WellKnownUrl, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 15 * time.Second}
	if resp, err := client.Do(req); err != nil {
		return nil, err
	} else {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("OIDC discovery returned HTTP %d", resp.StatusCode)
		}
		oidcDiscoveryDocument := OIDCDiscoveryDocument{}
		if err := json.NewDecoder(resp.Body).Decode(&oidcDiscoveryDocument); err != nil {
			options.Logger.PrintAndLog("OAuth2Router", fmt.Sprintf("Failed to decode ([%d] %s)", resp.StatusCode, resp.Status), err)
			return nil, err
		}
		// Discovery describes supported scopes, not a scope request. Never request all of them implicitly.

		if config.Endpoint.AuthURL == "" {
			config.Endpoint.AuthURL = oidcDiscoveryDocument.AuthorizationEndpoint
		}

		if config.Endpoint.TokenURL == "" {
			config.Endpoint.TokenURL = oidcDiscoveryDocument.TokenEndpoint
		}

		if options.OAuth2UserInfoUrl == "" {
			options.OAuth2UserInfoUrl = oidcDiscoveryDocument.UserinfoEndpoint
		}

	}
	// Validate after merging discovery with explicit overrides. A valid JSON
	// document can still be incomplete and must remain eligible for retry.
	if strings.TrimSpace(config.Endpoint.AuthURL) == "" || strings.TrimSpace(config.Endpoint.TokenURL) == "" ||
		strings.TrimSpace(options.OAuth2UserInfoUrl) == "" {
		return nil, errors.New("OIDC discovery did not resolve all required endpoints")
	}
	return config, nil
}

// Build from a private settings snapshot, without holding settings or cache locks.
func newOAuth2Conf(redirectUrl string, options *OAuth2RouterOptions) (*oauth2.Config, error) {
	config := &oauth2.Config{
		ClientID:     options.OAuth2ClientId,
		ClientSecret: options.OAuth2ClientSecret,
		RedirectURL:  redirectUrl,
		Endpoint: oauth2.Endpoint{
			AuthURL:  options.OAuth2ServerURL,
			TokenURL: options.OAuth2TokenURL,
		},
	}
	if options.OAuth2Scopes != "" {
		config.Scopes = strings.FieldsFunc(options.OAuth2Scopes, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' })
	} else if strings.TrimSpace(options.OAuth2WellKnownUrl) != "" {
		// Discovery lists what a provider supports; it does not define what this
		// client should request. Use the conservative conventional OIDC set.
		config.Scopes = append([]string(nil), defaultOIDCScopes...)
	}
	if options.OAuth2WellKnownUrl != "" && (config.Endpoint.AuthURL == "" || config.Endpoint.TokenURL == "" ||
		options.OAuth2UserInfoUrl == "") {
		return fetchOAuth2Configuration(config, options)
	}
	return config, nil
}

func newOAuth2ReturnCookie(originalPath string, secure bool, expires time.Time) *http.Cookie {
	if originalPath == "" || !strings.HasPrefix(originalPath, "/") || strings.HasPrefix(originalPath, "//") ||
		strings.ContainsAny(originalPath, "\r\n") {
		originalPath = "/"
	}
	build := func(value string) *http.Cookie {
		return &http.Cookie{
			Name: returnCookieName, Value: url.QueryEscape(value), Path: "/", Expires: expires,
			HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
		}
	}
	cookie := build(originalPath)
	if len(cookie.String()) > maxOAuth2CookieSize {
		cookie = build("/")
	}
	return cookie
}

const returnCookieName = "z-oauth-return"

type oauth2RequestConfiguration struct {
	config  *oauth2.Config
	options OAuth2RouterOptions
}

// Called with settingsMu held so a request carries its configuration version
// through provider calls and session creation/validation.
func (ar *OAuth2Router) snapshotOptions() OAuth2RouterOptions {
	options := *ar.options
	options.settingsGeneration = ar.settingsGeneration
	return options
}

// requestConfiguration reads and publishes matching settings/config snapshots
// under short locks. Slow discovery only holds up misses for the same host and
// settings generation; it never holds either the settings or cache lock.
func (ar *OAuth2Router) requestConfiguration(host, redirectURL string) (*oauth2.Config, OAuth2RouterOptions) {
	for {
		ar.settingsMu.RLock()
		options := ar.snapshotOptions()
		generation := ar.settingsGeneration
		item := options.OAuth2ConfigCache.Get(host)
		ar.settingsMu.RUnlock()
		if item != nil {
			return item.Value(), options
		}

		// New settings must not wait for discovery started by an older generation.
		key := fmt.Sprintf("%d:%s", generation, host)
		result, _, _ := ar.configLoads.Do(key, func() (interface{}, error) {
			// Another load or settings update may have completed before this flight.
			ar.settingsMu.RLock()
			if generation != ar.settingsGeneration {
				ar.settingsMu.RUnlock()
				return nil, nil
			}
			if item := ar.options.OAuth2ConfigCache.Get(host); item != nil {
				cached := oauth2RequestConfiguration{item.Value(), ar.snapshotOptions()}
				ar.settingsMu.RUnlock()
				return cached, nil
			}
			ar.settingsMu.RUnlock()

			config, err := newOAuth2Conf(redirectURL, &options)
			if err != nil {
				options.Logger.PrintAndLog("OAuth2Router", "Failed to fetch OIDC configuration:", err)
			}

			ar.settingsMu.Lock()
			defer ar.settingsMu.Unlock()
			if generation == ar.settingsGeneration {
				if err != nil {
					// Share this failure with in-flight callers, but let the next
					// request retry. Cache hits renew TTL, so caching nil could
					// otherwise keep a recovered provider unavailable indefinitely.
					return oauth2RequestConfiguration{nil, ar.snapshotOptions()}, nil
				}
				if ar.options.OAuth2UserInfoUrl == "" {
					ar.options.OAuth2UserInfoUrl = options.OAuth2UserInfoUrl
				}
				ar.options.OAuth2ConfigCache.Set(host, config, ttlcache.DefaultTTL)
				return oauth2RequestConfiguration{config, ar.snapshotOptions()}, nil
			}
			return nil, nil
		})
		if result != nil {
			loaded := result.(oauth2RequestConfiguration)
			return loaded.config, loaded.options
		}
		// Settings changed during discovery. Retry using the current generation.
	}
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
	const returnCookie = returnCookieName
	const nonceCookie = "z-oidc-nonce"
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	oauthConfig, options := ar.requestConfiguration(r.Host, scheme+"://"+r.Host+callbackPrefix)
	if oauthConfig == nil {
		w.WriteHeader(500)
		return errors.New("failed to fetch OIDC configuration")
	}

	if oauthConfig.Endpoint.AuthURL == "" || oauthConfig.Endpoint.TokenURL == "" || options.OAuth2UserInfoUrl == "" {
		options.Logger.PrintAndLog("OAuth2Router", "Invalid OAuth2 configuration", nil)
		w.WriteHeader(500)
		return errors.New("invalid OAuth2 configuration")
	}
	oidcEnabled := isOIDCConfiguration(oauthConfig.Scopes, options.OAuth2WellKnownUrl)

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
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		var authCodeOptions []oauth2.AuthCodeOption
		if options.OAuth2CodeChallengeMethod == "PKCE" || options.OAuth2CodeChallengeMethod == "PKCE_S256" {
			verifierCookie, err := r.Cookie(verifierCookie)
			if err != nil || verifierCookie.Value == "" {
				options.Logger.PrintAndLog("OAuth2Router", "Read OAuth2 verifier cookie failed", err)
				w.WriteHeader(401)
				return errors.New("unauthorized")
			}
			authCodeOptions = append(authCodeOptions, oauth2.VerifierOption(verifierCookie.Value))
		}
		token, err := oauthConfig.Exchange(ctx, code, authCodeOptions...)
		if err != nil {
			options.Logger.PrintAndLog("OAuth2", "Token exchange failed", err)
			w.WriteHeader(401)
			return errors.New("unauthorized")
		}
		if !token.Valid() {
			options.Logger.PrintAndLog("OAuth2", "Invalid token", err)
			w.WriteHeader(401)
			return errors.New("unauthorized")
		}

		// OIDC requires a verified ID token, bound to the login nonce, and
		// a matching UserInfo subject before accepting the access token.
		var identity *Identity
		if oidcEnabled {
			loginNonce, nonceErr := r.Cookie(nonceCookie)
			if nonceErr != nil || loginNonce.Value == "" {
				http.Error(w, "Missing OIDC login nonce", http.StatusUnauthorized)
				return errors.New("missing OIDC login nonce")
			}
			identity, err = verifyOIDCLogin(ctx, oauthConfig, token, options.OAuth2WellKnownUrl, options.OAuth2UserInfoUrl, loginNonce.Value)
			if err != nil {
				options.Logger.PrintAndLog("OAuth2", "OIDC login verification failed", err)
				http.Error(w, "OIDC login verification failed", http.StatusUnauthorized)
				return errors.New("OIDC login verification failed")
			}
		} else {
			identity, err = FetchOAuth2Identity(ctx, oauthConfig.Client(ctx, token), options.OAuth2UserInfoUrl)
			if err != nil {
				http.Error(w, "OAuth2 login verification failed", http.StatusUnauthorized)
				return errors.New("OAuth2 login verification failed")
			}
		}

		cookie, err := ar.createSession(r, token, identity.Subject, options.settingsGeneration)
		if err != nil {
			http.Error(w, "OAuth2 configuration changed; please sign in again", http.StatusUnauthorized)
			return err
		}
		http.SetCookie(w, cookie)
		// Migrate old browser-held bearer tokens to server-side sessions.
		http.SetCookie(w, &http.Cookie{Name: tokenCookie, Path: "/", MaxAge: -1, HttpOnly: true, Secure: scheme == "https", SameSite: http.SameSiteLaxMode})

		if options.OAuth2CodeChallengeMethod == "PKCE" || options.OAuth2CodeChallengeMethod == "PKCE_S256" {
			cookie := http.Cookie{Name: verifierCookie, Value: "", Path: "/", Expires: time.Now().Add(-time.Hour * 1), HttpOnly: true}
			if scheme == "https" {
				cookie.Secure = true
				cookie.SameSite = http.SameSiteLaxMode
			}
			w.Header().Add("Set-Cookie", cookie.String())
		}

		// State and return destination are short-lived and bound to this host.
		for _, name := range []string{stateCookie, returnCookie, nonceCookie} {
			http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: scheme == "https", SameSite: http.SameSiteLaxMode})
		}
		http.Redirect(w, r, originalPath, http.StatusSeeOther)

		return errors.New("authorized")
	}
	unauthorized := false
	session, validSession := ar.sessionForRequest(r, options.settingsGeneration)
	if validSession {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		client := oauthConfig.Client(ctx, &oauth2.Token{AccessToken: session.accessToken, TokenType: session.tokenType})
		var identity *Identity
		var fetchErr error
		if oidcEnabled {
			identity, fetchErr = FetchIdentity(ctx, client, options.OAuth2UserInfoUrl)
		} else {
			identity, fetchErr = FetchOAuth2Identity(ctx, client, options.OAuth2UserInfoUrl)
		}
		_, stillValid := ar.sessionForRequest(r, options.settingsGeneration)
		if fetchErr != nil || !stillValid || identity.Subject != session.subject {
			options.Logger.PrintAndLog("OAuth2", "Failed to validate user info", fetchErr)
			unauthorized = true
		} else {
			allowedGroups := policy.AllowedGroups
			if len(allowedGroups) == 0 {
				allowedGroups = SplitAllowedGroups(options.OAuth2DefaultAllowedGroups)
			}
			if !GroupsAllowed(identity.Groups, allowedGroups) {
				http.Error(w, "403 - Access denied: insufficient group permissions", http.StatusForbidden)
				return errors.New("forbidden")
			}
			isWebSocket := strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
			if policy.ForwardUserInfo && (!isWebSocket || policy.ForwardWebSocketIdentity) {
				SetIdentityHeaders(r.Header, identity)
				*r = *r.WithContext(context.WithValue(r.Context(), verifiedIdentityHeadersKey{}, true))
			}
			// Never forward the session credential or a client-supplied token.
			StripOAuth2Cookies(r, true)
			if !policy.RemoveTokenCookie {
				r.AddCookie(&http.Cookie{Name: tokenCookie, Value: session.accessToken})
			}
		}
	} else {
		unauthorized = true
	}
	if unauthorized {
		state := oauth2.GenerateVerifier()
		originalPath := r.URL.RequestURI()
		http.SetCookie(w, &http.Cookie{Name: stateCookie, Value: state, Path: "/", Expires: time.Now().Add(10 * time.Minute), HttpOnly: true, Secure: scheme == "https", SameSite: http.SameSiteLaxMode})
		http.SetCookie(w, newOAuth2ReturnCookie(originalPath, scheme == "https", time.Now().Add(10*time.Minute)))
		// A unique nonce prevents ID token replay and binds claims to this login.
		authOptions := []oauth2.AuthCodeOption{oauth2.AccessTypeOffline}
		if oidcEnabled {
			nonce := oauth2.GenerateVerifier()
			http.SetCookie(w, &http.Cookie{
				Name: nonceCookie, Value: nonce, Path: "/", MaxAge: 600,
				HttpOnly: true, Secure: scheme == "https", SameSite: http.SameSiteLaxMode,
			})
			authOptions = append(authOptions, oauth2.SetAuthURLParam("nonce", nonce))
		}
		var authURL string
		if options.OAuth2CodeChallengeMethod == "PKCE" || options.OAuth2CodeChallengeMethod == "PKCE_S256" {
			cookie := http.Cookie{Name: verifierCookie, Value: oauth2.GenerateVerifier(), Path: "/", Expires: time.Now().Add(time.Hour * 1), HttpOnly: true}
			if scheme == "https" {
				cookie.Secure = true
				cookie.SameSite = http.SameSiteLaxMode
			}

			w.Header().Add("Set-Cookie", cookie.String())

			if options.OAuth2CodeChallengeMethod == "PKCE" {
				authURL = oauthConfig.AuthCodeURL(state, append(authOptions, oauth2.SetAuthURLParam("code_challenge", cookie.Value))...)
			} else {
				authURL = oauthConfig.AuthCodeURL(state, append(authOptions, oauth2.S256ChallengeOption(cookie.Value))...)
			}
		} else {
			authURL = oauthConfig.AuthCodeURL(state, authOptions...)
		}

		http.Redirect(w, r, authURL, http.StatusFound)

		return errors.New("unauthorized")
	}
	return nil
}
