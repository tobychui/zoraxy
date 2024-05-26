package permissionpolicy_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"imuslab.com/zoraxy/mod/dynamicproxy/permissionpolicy"
)

func TestInjectPermissionPolicyHeader(t *testing.T) {
	//Prepare the data for permission policy
	testPermissionPolicy := permissionpolicy.GetDefaultPermissionPolicy()
	testPermissionPolicy.Geolocation = []string{"self"}
	testPermissionPolicy.Microphone = []string{"self", "https://example.com"}
	testPermissionPolicy.Camera = []string{"*"}

	tests := []struct {
		name           string
		existingHeader string
		policy         *permissionpolicy.PermissionsPolicy
		expectedHeader string
	}{
		{
			name:           "Default policy with a few limitations",
			existingHeader: "",
			policy:         testPermissionPolicy,
			expectedHeader: `accelerometer=*, ambient-light-sensor=*, autoplay=*, battery=*, camera=*, cross-origin-isolated=*, display-capture=*, document-domain=*, encrypted-media=*, execution-while-not-rendered=*, execution-while-out-of-viewport=*, fullscreen=*, geolocation=(self), gyroscope=*, keyboard-map=*, magnetometer=*, microphone=(self "https://example.com"), midi=*, navigation-override=*, payment=*, picture-in-picture=*, publickey-credentials-get=*, screen-wake-lock=*, sync-xhr=*, usb=*, web-share=*, xr-spatial-tracking=*, clipboard-read=*, clipboard-write=*, gamepad=*, speaker-selection=*, conversion-measurement=*, focus-without-user-activation=*, hid=*, idle-detection=*, interest-cohort=*, serial=*, sync-script=*, trust-token-redemption=*, unload=*, window-placement=*, vertical-scroll=*`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			if tt.existingHeader != "" {
				rr.Header().Set("Permissions-Policy", tt.existingHeader)
			}

			permissionpolicy.InjectPermissionPolicyHeader(rr, tt.policy)

			gotHeader := rr.Header().Get("Permissions-Policy")
			if !strings.Contains(gotHeader, tt.expectedHeader) {
				t.Errorf("got header %s, want %s", gotHeader, tt.expectedHeader)
			}
		})
	}
}
