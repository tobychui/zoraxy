package permissionpolicy

import (
	"fmt"
	"net/http"
	"strings"
)

/*
	Permisson Policy

	This is a permission policy header modifier that changes
	the request permission related policy fields

	author: tobychui
*/

type PermissionsPolicy struct {
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

// GetDefaultPermissionPolicy returns a PermissionsPolicy struct with all policies set to *
func GetDefaultPermissionPolicy() *PermissionsPolicy {
	return &PermissionsPolicy{
		Accelerometer:              []string{"*"},
		AmbientLightSensor:         []string{"*"},
		Autoplay:                   []string{"*"},
		Battery:                    []string{"*"},
		Camera:                     []string{"*"},
		CrossOriginIsolated:        []string{"*"},
		DisplayCapture:             []string{"*"},
		DocumentDomain:             []string{"*"},
		EncryptedMedia:             []string{"*"},
		ExecutionWhileNotRendered:  []string{"*"},
		ExecutionWhileOutOfView:    []string{"*"},
		Fullscreen:                 []string{"*"},
		Geolocation:                []string{"*"},
		Gyroscope:                  []string{"*"},
		KeyboardMap:                []string{"*"},
		Magnetometer:               []string{"*"},
		Microphone:                 []string{"*"},
		Midi:                       []string{"*"},
		NavigationOverride:         []string{"*"},
		Payment:                    []string{"*"},
		PictureInPicture:           []string{"*"},
		PublicKeyCredentialsGet:    []string{"*"},
		ScreenWakeLock:             []string{"*"},
		SyncXHR:                    []string{"*"},
		USB:                        []string{"*"},
		WebShare:                   []string{"*"},
		XRSpatialTracking:          []string{"*"},
		ClipboardRead:              []string{"*"},
		ClipboardWrite:             []string{"*"},
		Gamepad:                    []string{"*"},
		SpeakerSelection:           []string{"*"},
		ConversionMeasurement:      []string{"*"},
		FocusWithoutUserActivation: []string{"*"},
		HID:                        []string{"*"},
		IdleDetection:              []string{"*"},
		InterestCohort:             []string{"*"},
		Serial:                     []string{"*"},
		SyncScript:                 []string{"*"},
		TrustTokenRedemption:       []string{"*"},
		Unload:                     []string{"*"},
		WindowPlacement:            []string{"*"},
		VerticalScroll:             []string{"*"},
	}
}

// ToKeyValueHeader convert a permission policy struct into a key value string header
func (policy *PermissionsPolicy) ToKeyValueHeader() []string {
	policyHeader := []string{}

	// Helper function to add policy directives
	addDirective := func(name string, sources []string) {
		if len(sources) > 0 {
			if sources[0] == "*" {
				//Allow all
				policyHeader = append(policyHeader, fmt.Sprintf("%s=%s", name, "*"))
			} else {
				//Other than "self" which do not need double quote, others domain need double quote in place
				formatedSources := []string{}
				for _, source := range sources {
					if source == "self" {
						formatedSources = append(formatedSources, "self")
					} else {
						formatedSources = append(formatedSources, "\""+source+"\"")
					}
				}
				policyHeader = append(policyHeader, fmt.Sprintf("%s=(%s)", name, strings.Join(formatedSources, " ")))
			}
		} else {
			//There are no setting for this field. Assume no permission
			policyHeader = append(policyHeader, fmt.Sprintf("%s=()", name))
		}
	}

	// Add each policy directive to the header
	addDirective("accelerometer", policy.Accelerometer)
	addDirective("ambient-light-sensor", policy.AmbientLightSensor)
	addDirective("autoplay", policy.Autoplay)
	addDirective("battery", policy.Battery)
	addDirective("camera", policy.Camera)
	addDirective("cross-origin-isolated", policy.CrossOriginIsolated)
	addDirective("display-capture", policy.DisplayCapture)
	addDirective("document-domain", policy.DocumentDomain)
	addDirective("encrypted-media", policy.EncryptedMedia)
	addDirective("execution-while-not-rendered", policy.ExecutionWhileNotRendered)
	addDirective("execution-while-out-of-viewport", policy.ExecutionWhileOutOfView)
	addDirective("fullscreen", policy.Fullscreen)
	addDirective("geolocation", policy.Geolocation)
	addDirective("gyroscope", policy.Gyroscope)
	addDirective("keyboard-map", policy.KeyboardMap)
	addDirective("magnetometer", policy.Magnetometer)
	addDirective("microphone", policy.Microphone)
	addDirective("midi", policy.Midi)
	addDirective("navigation-override", policy.NavigationOverride)
	addDirective("payment", policy.Payment)
	addDirective("picture-in-picture", policy.PictureInPicture)
	addDirective("publickey-credentials-get", policy.PublicKeyCredentialsGet)
	addDirective("screen-wake-lock", policy.ScreenWakeLock)
	addDirective("sync-xhr", policy.SyncXHR)
	addDirective("usb", policy.USB)
	addDirective("web-share", policy.WebShare)
	addDirective("xr-spatial-tracking", policy.XRSpatialTracking)
	addDirective("clipboard-read", policy.ClipboardRead)
	addDirective("clipboard-write", policy.ClipboardWrite)
	addDirective("gamepad", policy.Gamepad)
	addDirective("speaker-selection", policy.SpeakerSelection)
	addDirective("conversion-measurement", policy.ConversionMeasurement)
	addDirective("focus-without-user-activation", policy.FocusWithoutUserActivation)
	addDirective("hid", policy.HID)
	addDirective("idle-detection", policy.IdleDetection)
	addDirective("interest-cohort", policy.InterestCohort)
	addDirective("serial", policy.Serial)
	addDirective("sync-script", policy.SyncScript)
	addDirective("trust-token-redemption", policy.TrustTokenRedemption)
	addDirective("unload", policy.Unload)
	addDirective("window-placement", policy.WindowPlacement)
	addDirective("vertical-scroll", policy.VerticalScroll)

	// Join the directives and set the header
	policyHeaderValue := strings.Join(policyHeader, ", ")
	return []string{"Permissions-Policy", policyHeaderValue}
}

// InjectPermissionPolicyHeader inject the permission policy into headers
func InjectPermissionPolicyHeader(w http.ResponseWriter, policy *PermissionsPolicy) {
	//Keep the original Permission Policy if exists, or there are no policy given
	if policy == nil || w.Header().Get("Permissions-Policy") != "" {
		return
	}
	headerKV := policy.ToKeyValueHeader()
	//Inject the new policy into the header
	w.Header().Set(headerKV[0], headerKV[1])
}
