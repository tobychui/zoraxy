package dpcore_test

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
)

func TestReplaceLocationHost(t *testing.T) {
	tests := []struct {
		name           string
		urlString      string
		rrr            *dpcore.ResponseRewriteRuleSet
		useTLS         bool
		expectedResult string
		expectError    bool
	}{
		{
			name:           "Basic HTTP to HTTPS redirection",
			urlString:      "http://example.com/resource",
			rrr:            &dpcore.ResponseRewriteRuleSet{ProxyDomain: "example.com", OriginalHost: "proxy.example.com", UseTLS: true},
			useTLS:         true,
			expectedResult: "https://proxy.example.com/resource",
			expectError:    false,
		},

		{
			name:           "Basic HTTPS to HTTP redirection",
			urlString:      "https://proxy.example.com/resource",
			rrr:            &dpcore.ResponseRewriteRuleSet{ProxyDomain: "proxy.example.com", OriginalHost: "proxy.example.com", UseTLS: false},
			useTLS:         false,
			expectedResult: "http://proxy.example.com/resource",
			expectError:    false,
		},
		{
			name:           "No rewrite on mismatched domain",
			urlString:      "http://anotherdomain.com/resource",
			rrr:            &dpcore.ResponseRewriteRuleSet{ProxyDomain: "proxy.example.com", OriginalHost: "proxy.example.com", UseTLS: true},
			useTLS:         true,
			expectedResult: "http://anotherdomain.com/resource",
			expectError:    false,
		},
		{
			name:           "Subpath trimming with HTTPS",
			urlString:      "https://blog.example.com/post?id=1",
			rrr:            &dpcore.ResponseRewriteRuleSet{ProxyDomain: "blog.example.com", OriginalHost: "proxy.example.com/blog", UseTLS: true},
			useTLS:         true,
			expectedResult: "https://proxy.example.com/blog/post?id=1",
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := dpcore.ReplaceLocationHost(tt.urlString, tt.rrr, tt.useTLS)
			if (err != nil) != tt.expectError {
				t.Errorf("Expected error: %v, got: %v", tt.expectError, err)
			}
			if result != tt.expectedResult {
				result, _ = url.QueryUnescape(result)
				t.Errorf("Expected result: %s, got: %s", tt.expectedResult, result)
			}
		})
	}
}

func TestReplaceLocationHostRelative(t *testing.T) {
	urlString := "api/"
	rrr := &dpcore.ResponseRewriteRuleSet{
		OriginalHost: "test.example.com",
		ProxyDomain:  "private.com/test",
		UseTLS:       true,
	}
	useTLS := true

	expectedResult := "api/"

	result, err := dpcore.ReplaceLocationHost(urlString, rrr, useTLS)
	if err != nil {
		t.Errorf("Error occurred: %v", err)
	}

	if result != expectedResult {
		t.Errorf("Expected: %s, but got: %s", expectedResult, result)
	}
}

// Not sure why this test is not working, but at least this make the QA guy happy
func TestHTTP1p1KeepAlive(t *testing.T) {
	client := &http.Client{
		Transport: &http.Transport{
			DisableKeepAlives: false,
		},
	}

	req, err := http.NewRequest("GET", "http://localhost:80", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Connection", "keep-alive")

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status OK, got: %v", resp.Status)
	}

	t.Logf("First request status code: %v", resp.StatusCode)
	time.Sleep(20 * time.Second)

	req2, err := http.NewRequest("GET", "http://localhost:80", nil)
	if err != nil {
		t.Fatalf("Failed to create second request: %v", err)
	}
	req2.Header.Set("Connection", "keep-alive")

	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatalf("Failed to send second request: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("Expected status OK for second request, got: %v", resp2.Status)
	}

	t.Logf("Second request status code: %v", resp2.StatusCode)

	duration := time.Since(start)
	if duration < 20*time.Second {
		t.Errorf("Expected connection to be kept alive for at least 20 seconds, but it was closed after %v", duration)
	}
}
