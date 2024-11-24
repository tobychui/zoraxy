package v308

/*
	v307 type definitions

	This file wrap up the self-contained data structure
	for v3.0.7 structure and allow automatic updates
	for future releases if required
*/

type v307PermissionsPolicy struct {
	Accelerometer              []string `json:"accelerometer"`
	AmbientLightSensor         []string `json:"ambient_light_sensor"`
	Autoplay                   []string `json:"autoplay"`
	Battery                    []string `json:"battery"`
	Camera                     []string `json:"camera"`
	CrossOriginIsolated        []string `json:"cross_origin_isolated"`
	DisplayCapture             []string `json:"display_capture"`
	DocumentDomain             []string `json:"document_domain"`
	EncryptedMedia             []string `json:"encrypted_media"`
	ExecutionWhileNotRendered  []string `json:"execution_while_not_rendered"`
	ExecutionWhileOutOfView    []string `json:"execution_while_out_of_viewport"`
	Fullscreen                 []string `json:"fullscreen"`
	Geolocation                []string `json:"geolocation"`
	Gyroscope                  []string `json:"gyroscope"`
	KeyboardMap                []string `json:"keyboard_map"`
	Magnetometer               []string `json:"magnetometer"`
	Microphone                 []string `json:"microphone"`
	Midi                       []string `json:"midi"`
	NavigationOverride         []string `json:"navigation_override"`
	Payment                    []string `json:"payment"`
	PictureInPicture           []string `json:"picture_in_picture"`
	PublicKeyCredentialsGet    []string `json:"publickey_credentials_get"`
	ScreenWakeLock             []string `json:"screen_wake_lock"`
	SyncXHR                    []string `json:"sync_xhr"`
	USB                        []string `json:"usb"`
	WebShare                   []string `json:"web_share"`
	XRSpatialTracking          []string `json:"xr_spatial_tracking"`
	ClipboardRead              []string `json:"clipboard_read"`
	ClipboardWrite             []string `json:"clipboard_write"`
	Gamepad                    []string `json:"gamepad"`
	SpeakerSelection           []string `json:"speaker_selection"`
	ConversionMeasurement      []string `json:"conversion_measurement"`
	FocusWithoutUserActivation []string `json:"focus_without_user_activation"`
	HID                        []string `json:"hid"`
	IdleDetection              []string `json:"idle_detection"`
	InterestCohort             []string `json:"interest_cohort"`
	Serial                     []string `json:"serial"`
	SyncScript                 []string `json:"sync_script"`
	TrustTokenRedemption       []string `json:"trust_token_redemption"`
	Unload                     []string `json:"unload"`
	WindowPlacement            []string `json:"window_placement"`
	VerticalScroll             []string `json:"vertical_scroll"`
}

// Auth credential for basic auth on certain endpoints
type v307BasicAuthCredentials struct {
	Username     string
	PasswordHash string
}

// Auth credential for basic auth on certain endpoints
type v307BasicAuthUnhashedCredentials struct {
	Username string
	Password string
}

// Paths to exclude in basic auth enabled proxy handler
type v307BasicAuthExceptionRule struct {
	PathPrefix string
}

// Header injection direction type
type v307HeaderDirection int

const (
	HeaderDirection_ZoraxyToUpstream   v307HeaderDirection = 0 //Inject (or remove) header to request out-going from Zoraxy to backend server
	HeaderDirection_ZoraxyToDownstream v307HeaderDirection = 1 //Inject (or remove) header to request out-going from Zoraxy to client (e.g. browser)
)

// User defined headers to add into a proxy endpoint
type v307UserDefinedHeader struct {
	Direction v307HeaderDirection
	Key       string
	Value     string
	IsRemove  bool //Instead of set, remove this key instead
}

// The original proxy endpoint structure from v3.0.7
type v307ProxyEndpoint struct {
	ProxyType            int      //The type of this proxy, see const def
	RootOrMatchingDomain string   //Matching domain for host, also act as key
	MatchingDomainAlias  []string //A list of domains that alias to this rule
	Domain               string   //Domain or IP to proxy to

	//TLS/SSL Related
	RequireTLS               bool //Target domain require TLS
	BypassGlobalTLS          bool //Bypass global TLS setting options if TLS Listener enabled (parent.tlsListener != nil)
	SkipCertValidations      bool //Set to true to accept self signed certs
	SkipWebSocketOriginCheck bool //Skip origin check on websocket upgrade connections

	//Virtual Directories
	VirtualDirectories []*v307VirtualDirectoryEndpoint

	//Custom Headers
	UserDefinedHeaders           []*v307UserDefinedHeader //Custom headers to append when proxying requests from this endpoint
	HSTSMaxAge                   int64                    //HSTS max age, set to 0 for disable HSTS headers
	EnablePermissionPolicyHeader bool                     //Enable injection of permission policy header
	PermissionPolicy             *v307PermissionsPolicy   //Permission policy header

	//Authentication
	RequireBasicAuth        bool                          //Set to true to request basic auth before proxy
	BasicAuthCredentials    []*v307BasicAuthCredentials   //Basic auth credentials
	BasicAuthExceptionRules []*v307BasicAuthExceptionRule //Path to exclude in a basic auth enabled proxy target

	// Rate Limiting
	RequireRateLimit bool
	RateLimit        int64 // Rate limit in requests per second

	//Access Control
	AccessFilterUUID string //Access filter ID

	Disabled bool //If the rule is disabled

	//Fallback routing logic (Special Rule Sets Only)
	DefaultSiteOption int    //Fallback routing logic options
	DefaultSiteValue  string //Fallback routing target, optional
}

// A Virtual Directory endpoint, provide a subset of ProxyEndpoint for better
// program structure than directly using ProxyEndpoint
type v307VirtualDirectoryEndpoint struct {
	MatchingPath        string //Matching prefix of the request path, also act as key
	Domain              string //Domain or IP to proxy to
	RequireTLS          bool   //Target domain require TLS
	SkipCertValidations bool   //Set to true to accept self signed certs
	Disabled            bool   //If the rule is enabled
}
