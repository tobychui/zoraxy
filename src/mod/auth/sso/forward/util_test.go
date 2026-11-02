package forward

import (
	"crypto/tls"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScheme(t *testing.T) {
	testCases := []struct {
		name     string
		have     *http.Request
		expected string
	}{
		{
			"ShouldHandleDefault",
			&http.Request{},
			"http",
		},
		{
			"ShouldHandleExplicit",
			&http.Request{
				TLS: nil,
			},
			"http",
		},
		{
			"ShouldHandleHTTPS",
			&http.Request{
				TLS: &tls.ConnectionState{},
			},
			"https",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, scheme(tc.have))
		})
	}
}

func TestHeaderCookieRedact(t *testing.T) {
	testCases := []struct {
		name            string
		have            string
		names           []string
		expectedInclude string
		expectedExclude string
	}{
		{
			"ShouldHandleIncludeEmptyWithoutSettings",
			"",
			nil,
			"",
			"",
		},
		{
			"ShouldHandleIncludeEmptyWithSettings",
			"",
			[]string{"include"},
			"",
			"",
		},
		{
			"ShouldHandleValueWithoutSettings",
			"include=value; exclude=value",
			nil,
			"include=value; exclude=value",
			"include=value; exclude=value",
		},
		{
			"ShouldHandleValueWithSettings",
			"include=value; exclude=value",
			[]string{"include"},
			"include=value",
			"exclude=value",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var include, exclude *http.Request

			include, exclude = &http.Request{Header: http.Header{}}, &http.Request{Header: http.Header{}}

			if tc.have != "" {
				include.Header.Set(HeaderCookie, tc.have)
				exclude.Header.Set(HeaderCookie, tc.have)
			}

			headerCookieRedact(include, tc.names, false)

			assert.Equal(t, tc.expectedInclude, include.Header.Get(HeaderCookie))

			headerCookieRedact(exclude, tc.names, true)

			assert.Equal(t, tc.expectedExclude, exclude.Header.Get(HeaderCookie))
		})
	}
}

func TestHeaderCopyExcluded(t *testing.T) {
	testCases := []struct {
		name     string
		original http.Header
		excluded []string
		expected http.Header
	}{
		{
			"ShouldHandleNoSettingsNoHeaders",
			http.Header{},
			nil,
			http.Header{},
		},
		{
			"ShouldHandleNoSettingsWithHeaders",
			http.Header{
				"Example":     []string{"value", "other"},
				"Exclude":     []string{"value", "other"},
				HeaderUpgrade: []string{"do", "not", "copy"},
			},
			nil,
			http.Header{
				"Example": []string{"value", "other"},
				"Exclude": []string{"value", "other"},
			},
		},
		{
			"ShouldHandleSettingsWithHeaders",
			http.Header{
				"Example":     []string{"value", "other"},
				"Exclude":     []string{"value", "other"},
				HeaderUpgrade: []string{"do", "not", "copy"},
			},
			[]string{"exclude"},
			http.Header{
				"Example": []string{"value", "other"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			headers := http.Header{}

			headerCopyExcluded(tc.original, headers, tc.excluded)

			assert.Equal(t, tc.expected, headers)
		})
	}
}

func TestHeaderCopyIncluded(t *testing.T) {
	testCases := []struct {
		name        string
		original    http.Header
		included    []string
		expected    http.Header
		expectedAll http.Header
	}{
		{
			"ShouldHandleNoSettingsNoHeaders",
			http.Header{},
			nil,
			http.Header{},
			http.Header{},
		},
		{
			"ShouldHandleNoSettingsWithHeaders",
			http.Header{
				"Example":     []string{"value", "other"},
				"Include":     []string{"value", "other"},
				HeaderUpgrade: []string{"do", "not", "copy"},
			},
			nil,
			http.Header{},
			http.Header{
				"Example": []string{"value", "other"},
				"Include": []string{"value", "other"},
			},
		},
		{
			"ShouldHandleSettingsWithHeaders",
			http.Header{
				"Example":     []string{"value", "other"},
				"Include":     []string{"value", "other"},
				HeaderUpgrade: []string{"do", "not", "copy"},
			},
			[]string{"include"},
			http.Header{
				"Include": []string{"value", "other"},
			},
			http.Header{
				"Include": []string{"value", "other"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			headers := http.Header{}

			headerCopyIncluded(tc.original, headers, tc.included, false)

			assert.Equal(t, tc.expected, headers)

			headers = http.Header{}

			headerCopyIncluded(tc.original, headers, tc.included, true)

			assert.Equal(t, tc.expectedAll, headers)
		})
	}
}

func TestRSetIPHeader(t *testing.T) {
	testCases := []struct {
		name       string
		remoteAddr string
		supplied   string
		expected   string
	}{
		{"ShouldHandleIPv4", "10.0.0.1:54321", "", "10.0.0.1"},
		{"ShouldHandleIPv6", "[2a01:4f8::1]:54321", "", "2a01:4f8::1"},
		{"ShouldHandleIPv6WithoutPort", "2a01:4f8::1", "", "2a01:4f8::1"},
		{"ShouldOverwriteClientSuppliedIPv4", "10.0.0.1:54321", "10.2.45.5", "10.0.0.1"},
		{"ShouldOverwriteClientSuppliedIPv6", "[2a01:4f8::1]:54321", "10.2.45.5", "2a01:4f8::1"},
		{"ShouldDropClientSuppliedOnUnparsableAddress", "not-an-address", "10.2.45.5", ""},
		{"ShouldDropClientSuppliedOnEmptyAddress", "", "10.2.45.5", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := &http.Request{RemoteAddr: tc.remoteAddr}
			req := &http.Request{Header: http.Header{}}

			if tc.supplied != "" {
				req.Header.Set(HeaderXForwardedFor, tc.supplied)
				req.Header.Set(HeaderXOriginalIP, tc.supplied)
			}

			rSetIPHeader(r, req, HeaderXForwardedFor, HeaderXOriginalIP)

			assert.Equal(t, tc.expected, req.Header.Get(HeaderXForwardedFor))
			assert.Equal(t, tc.expected, req.Header.Get(HeaderXOriginalIP))
		})
	}
}
