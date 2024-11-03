package rewrite

import (
	"net/http/httptest"
	"testing"
)

func TestGetHeaderVariableValuesFromRequest(t *testing.T) {
	// Create a sample request
	req := httptest.NewRequest("GET", "https://example.com/test?foo=bar", nil)
	req.Host = "example.com"
	req.RemoteAddr = "192.168.1.1:12345"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "TestAgent")
	req.Header.Set("Referer", "https://referer.com")

	// Call the function
	vars := GetHeaderVariableValuesFromRequest(req)

	// Expected results
	expected := map[string]string{
		"$host":            "example.com",
		"$remote_addr":     "192.168.1.1:12345",
		"$request_uri":     "https://example.com/test?foo=bar",
		"$request_method":  "GET",
		"$content_length":  "0", // ContentLength is 0 because there's no body in the request
		"$content_type":    "application/json",
		"$uri":             "/test",
		"$args":            "foo=bar",
		"$scheme":          "https",
		"$query_string":    "foo=bar",
		"$http_user_agent": "TestAgent",
		"$http_referer":    "https://referer.com",
	}

	// Check each expected variable
	for key, expectedValue := range expected {
		if vars[key] != expectedValue {
			t.Errorf("Expected %s to be %s, but got %s", key, expectedValue, vars[key])
		}
	}
}

func TestCustomHeadersIncludeDynamicVariables(t *testing.T) {
	tests := []struct {
		name           string
		headers        []*UserDefinedHeader
		expectedHasVar bool
	}{
		{
			name:           "No headers",
			headers:        []*UserDefinedHeader{},
			expectedHasVar: false,
		},
		{
			name: "Headers without dynamic variables",
			headers: []*UserDefinedHeader{
				{
					Direction: HeaderDirection_ZoraxyToUpstream,
					Key:       "X-Custom-Header",
					Value:     "staticValue",
					IsRemove:  false,
				},
				{
					Direction: HeaderDirection_ZoraxyToDownstream,
					Key:       "X-Another-Header",
					Value:     "staticValue",
					IsRemove:  false,
				},
			},
			expectedHasVar: false,
		},
		{
			name: "Headers with one dynamic variable",
			headers: []*UserDefinedHeader{
				{
					Direction: HeaderDirection_ZoraxyToUpstream,
					Key:       "X-Custom-Header",
					Value:     "$dynamicValue",
					IsRemove:  false,
				},
			},
			expectedHasVar: true,
		},
		{
			name: "Headers with multiple dynamic variables",
			headers: []*UserDefinedHeader{
				{
					Direction: HeaderDirection_ZoraxyToUpstream,
					Key:       "X-Custom-Header",
					Value:     "$dynamicValue1",
					IsRemove:  false,
				},
				{
					Direction: HeaderDirection_ZoraxyToDownstream,
					Key:       "X-Another-Header",
					Value:     "$dynamicValue2",
					IsRemove:  false,
				},
			},
			expectedHasVar: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasVar := CustomHeadersIncludeDynamicVariables(tt.headers)
			if hasVar != tt.expectedHasVar {
				t.Errorf("Expected %v, but got %v", tt.expectedHasVar, hasVar)
			}
		})
	}
}

func TestPopulateRequestHeaderVariables(t *testing.T) {
	// Create a sample request with specific values
	req := httptest.NewRequest("GET", "https://example.com/test?foo=bar", nil)
	req.Host = "example.com"
	req.RemoteAddr = "192.168.1.1:12345"
	req.Header.Set("User-Agent", "TestAgent")
	req.Header.Set("Referer", "https://referer.com")

	// Define user-defined headers with dynamic variables
	userDefinedHeaders := []*UserDefinedHeader{
		{
			Direction: HeaderDirection_ZoraxyToUpstream,
			Key:       "X-Forwarded-Host",
			Value:     "$host",
		},
		{
			Direction: HeaderDirection_ZoraxyToDownstream,
			Key:       "X-Client-IP",
			Value:     "$remote_addr",
		},
		{
			Direction: HeaderDirection_ZoraxyToDownstream,
			Key:       "X-Custom-Header",
			Value:     "$request_uri",
		},
	}

	// Call the function with the test data
	resultHeaders := PopulateRequestHeaderVariables(req, userDefinedHeaders)

	// Expected results after variable substitution
	expectedHeaders := []*UserDefinedHeader{
		{
			Direction: HeaderDirection_ZoraxyToUpstream,
			Key:       "X-Forwarded-Host",
			Value:     "example.com",
		},
		{
			Direction: HeaderDirection_ZoraxyToDownstream,
			Key:       "X-Client-IP",
			Value:     "192.168.1.1:12345",
		},
		{
			Direction: HeaderDirection_ZoraxyToDownstream,
			Key:       "X-Custom-Header",
			Value:     "https://example.com/test?foo=bar",
		},
	}

	// Validate results
	for i, expected := range expectedHeaders {
		if resultHeaders[i].Direction != expected.Direction ||
			resultHeaders[i].Key != expected.Key ||
			resultHeaders[i].Value != expected.Value {
			t.Errorf("Expected header %v, but got %v", expected, resultHeaders[i])
		}
	}
}
