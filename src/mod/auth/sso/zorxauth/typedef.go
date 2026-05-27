package zorxauth

import (
	"sync"
	"time"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/info/logger"
)

const (
	DB_NAME                       = "zorxauth"
	DB_USERS_TABLE                = "zorxauth_users"
	DB_USERS_KEY_PREFIX           = "user_"
	DB_BROWSER_SESSIONS_TABLE     = "zorxauth_browser_sessions"
	DB_BROWSER_SESSION_KEY_PREFIX = "session_"
	DB_GATEWAY_SESSIONS_TABLE     = "zorxauth_gateway_sessions"
	DB_GATEWAY_SESSION_KEY_PREFIX = "gateway_"
)

type User struct {
	ID             string   `json:"id"` //uuidv4
	Username       string   `json:"username"`
	Email          string   `json:"email"` //optional, alternative login identifier if provided
	PasswordHash   string   `json:"passwordHash"`
	UseGroupPolicy bool     `json:"useGroupPolicy"` //Use group poliy instead of AllowedHosts for checking access
	GroupID        string   `json:"groupId"`        //GroupID is the group policy ID that the user belongs to
	AllowedHosts   []string `json:"allowedHosts"`   //optional, if empty, allow all hosts

	/* User Configs */
	Enable2FA  bool   `json:"enable2FA"`  //Whether the user has enabled 2FA. If true, the user must complete 2FA verification after password verification to complete login.
	TOTPSecret string `json:"totpSecret"` //The TOTP secret for 2FA. Required if Enable2FA is true.

	/* Passkeys (WebAuthn) */
	PasskeyCredentials []PasskeyCredential `json:"passkeyCredentials"` //Registered WebAuthn/passkey credentials

}

// PasskeyCredential stores a single WebAuthn credential for a user.
type PasskeyCredential struct {
	ID             []byte   `json:"id"`             // Raw credential ID bytes
	PublicKey      []byte   `json:"publicKey"`      // COSE-encoded public key
	AAGUID         []byte   `json:"aaguid"`         // Authenticator AAGUID
	SignCount      uint32   `json:"signCount"`      // Signature counter (clone detection)
	CloneWarning   bool     `json:"cloneWarning"`   // Possible clone detected
	Transports     []string `json:"transports"`     // e.g. ["internal", "hybrid", "usb"]
	BackupEligible bool     `json:"backupEligible"` // Can be cloud-synced
	BackupState    bool     `json:"backupState"`    // Currently synced to cloud
	Name           string   `json:"name"`           // User-assigned display name
	CreatedAt      int64    `json:"createdAt"`      // Unix timestamp
	LastUsedAt     int64    `json:"lastUsedAt"`     // Unix timestamp
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
	/* Storage Options */
	ConfigFolderPath string `json:"config_folder_path"` //Path to the config folder for storing group policy files. Default: ./conf/sso/zorxauth
}

type BrowserSession struct {
	Username string
	Expiry   time.Time
}

type GatewaySession struct {
	Username string
	Expiry   time.Time
}

// PendingTOTPSession holds login context while waiting for 2FA verification
type PendingTOTPSession struct {
	Username       string
	Host           string
	Port           string
	Protocol       string
	RedirectTarget string
	RememberMe     bool
	Expiry         time.Time
}

// PendingTOTPSetup holds a newly generated TOTP secret before the user confirms it
type PendingTOTPSetup struct {
	Secret string
	Expiry time.Time
}

// AuthRouter handles ZorxAuth SSO authentication routing
type AuthRouter struct {
	Logger   *logger.Logger
	Database *database.Database
	Options  *AuthRouterOptions

	/* Internal */
	sessionIdStore      sync.Map //sessionId -> userID
	gatewaySessionStore sync.Map //sessionId -> *GatewaySession
	cookieIdStore       sync.Map //browserSessionID (cookie value) -> *BrowserSession
	gatewayServer       *GatewayServer

	/* Group Policies */
	groupPolicies sync.Map // id (string) -> *GroupPolicy

	/* Login rate limiting */
	loginAttemptCounter sync.Map // IP -> *int64, total attempts in current minute window
	loginFailureCounter sync.Map // IP -> *int64, consecutive failures used for exponential backoff
	rateLimitResetStop  chan bool // stop channel for the per-minute counter reset ticker

	/* 2FA */
	pendingTOTPSessions sync.Map // totp_token (string) -> *PendingTOTPSession (pending login 2FA)
	pendingTOTPSetup    sync.Map // username (string) -> *PendingTOTPSetup (pending 2FA enrollment)

	/* Passkeys (WebAuthn) */
	pendingPasskeyReg  sync.Map // token (string) -> *PendingPasskeyRegistration
	pendingPasskeyAuth sync.Map // token (string) -> *PendingPasskeyAuth
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
		ConfigFolderPath:         "./conf/sso/zorxauth",
	}
}
