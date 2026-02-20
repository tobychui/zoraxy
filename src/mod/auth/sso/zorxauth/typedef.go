package zorxauth

import (
	"sync"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/info/logger"
)

const (
	DB_NAME             = "zorxauth"
	DB_USERS_TABLE      = "zorxauth_users"
	DB_USERS_KEY_PREFIX = "user_"
)

type User struct {
	ID           string   `json:"id"` //uuidv4
	Username     string   `json:"username"`
	Email        string   `json:"email"` //optional
	PasswordHash string   `json:"passwordHash"`
	AllowedHosts []string `json:"allowedHosts"` //optional, if empty, allow all hosts
}

type GroupOptions struct {
	AllowedHosts []string
}

type Group struct {
	ID      string        `json:"id"` //uuidv4
	Name    string        `json:"name"`
	Admin   bool          `json:"admin"` //Allow access all proxy rules
	Options *GroupOptions `json:"options"`
}

// AuthRouterOptions contains configuration for the ZorxAuth router
type AuthRouterOptions struct {
	/* Auth Gateway Options */
	EnableAuthGateway   bool   `json:"enable_auth_gateway"`   //Whether to enable the authentication gateway. If disabled, all auth request are treated as rejected. Default: false
	GatewayPort         int    `json:"gateway_port"`          //Port number for the authentication gateway. Default: 5489
	FallbackRedirectURL string `json:"fallback_redirect_url"` //Fallback redirect URL if the original redirect URL is missing or invalid.
	/* Auth Router Options */
	EnableRateLimit          bool   `json:"enable_rate_limit"`           //Whether to enable rate limiting for authentication attempts. Default: true
	RateLimitPerIp           int    `json:"rate_limit_per_ip"`           //Number of allowed authentication attempts per minute per IP. Default: 60
	UseExpotentialBackoff    bool   `json:"use_exponential_backoff"`     //Whether to use exponential backoff for failed authentication attempts. Default: true
	SSORedirectURL           string `json:"sso_redirect_url"`            //URL of the SSO authentication endpoint
	SSOSessionSetURL         string `json:"sso_session_set_url"`         //URL of the SSO session set endpoint
	CookieName               string `json:"cookie_name"`                 //Name of the session cookie
	CookieDuration           int    `json:"cookie_duration"`             //Duration in seconds for the session cookie
	CookieDurationRememberMe int    `json:"cookie_duration_remember_me"` //Duration in seconds for the session cookie when "Remember Me" is selected
	AllowCrossHostSession    bool   `json:"allow_cross_host_session"`    //Whether to allow session cookies to be valid across different hosts
}

// AuthRouter handles ZorxAuth SSO authentication routing
type AuthRouter struct {
	Logger   *logger.Logger
	Database *database.Database
	Options  *AuthRouterOptions

	/* Internal */
	sessionIdStore      sync.Map //sessionId -> userID
	gatewaySessionStore sync.Map //sessionId -> userID
	gatewayServer       *GatewayServer

	/* Login rate limiting */
	loginAttemptCounter sync.Map  // IP -> *int64, total attempts in current minute window
	loginFailureCounter sync.Map  // IP -> *int64, consecutive failures used for exponential backoff
	rateLimitResetStop  chan bool // stop channel for the per-minute counter reset ticker
}

func getDefaultOptions() *AuthRouterOptions {
	return &AuthRouterOptions{
		GatewayPort:              5489,
		EnableAuthGateway:        false,
		EnableRateLimit:          true,
		RateLimitPerIp:           60,
		UseExpotentialBackoff:    true,
		SSORedirectURL:           "",
		SSOSessionSetURL:         "/.wellknown/zorxauth/session/set",
		CookieName:               "zr_xauth_session",
		CookieDuration:           3600,
		CookieDurationRememberMe: 604800, // 7 days
		AllowCrossHostSession:    true,
	}
}
