package oauth2

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	xoauth2 "golang.org/x/oauth2"
)

type testConfigurationResult struct {
	config  *xoauth2.Config
	options OAuth2RouterOptions
}

func awaitDiscoveryTestResult[T any](t *testing.T, result <-chan T) T {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(3 * time.Second):
		t.Fatal("operation blocked while discovery was stalled")
		var zero T
		return zero
	}
}

func startTestConfiguration(router *OAuth2Router, host string) <-chan testConfigurationResult {
	result := make(chan testConfigurationResult, 1)
	go func() {
		defer close(result)
		config, options := router.requestConfiguration(host, "https://"+host+"/internal/oauth2")
		result <- testConfigurationResult{config, options}
	}()
	return result
}

func TestStalledDiscoveryDoesNotBlockRequestsOrSettings(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodDelete} {
		for _, invalidResponse := range []bool{false, true} {
			name := method + "/success"
			if invalidResponse {
				name = method + "/discovery-error"
			}
			t.Run(name, func(t *testing.T) {
				router := newSettingsTestRouter(t)
				entered, release := make(chan struct{}), make(chan struct{})
				var releaseOnce, enteredOnce sync.Once
				unblock := func() { releaseOnce.Do(func() { close(release) }) }
				var calls atomic.Int32
				discovery := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if calls.Add(1) > 1 {
						enteredOnce.Do(func() { close(entered) })
						<-release
						if invalidResponse {
							_, _ = w.Write([]byte("invalid discovery JSON"))
							return
						}
					}
					_ = json.NewEncoder(w).Encode(OIDCDiscoveryDocument{
						AuthorizationEndpoint: "https://old.test/authorize",
						TokenEndpoint:         "https://old.test/token", UserinfoEndpoint: "https://old.test/userinfo",
					})
				}))
				defer discovery.Close()
				defer unblock()
				form := url.Values{
					"oauth2ClientId": {"old-client"}, "oauth2ClientSecret": {"secret"},
					"oauth2CodeChallengeMethod": {"none"}, "oauth2ConfigurationCacheTime": {"60s"},
					"oauth2WellKnownUrl": {discovery.URL}, "oauth2DefaultAllowedGroups": {"old-group"},
				}
				request := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
				request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				w := httptest.NewRecorder()
				router.HandleSetOAuth2Settings(w, request)
				if strings.Contains(w.Body.String(), "error") {
					t.Fatal(w.Body.String())
				}
				warm := awaitDiscoveryTestResult(t, startTestConfiguration(router, "warm.test"))
				if warm.config == nil || warm.options.OAuth2UserInfoUrl != "https://old.test/userinfo" {
					t.Fatal("failed to warm configuration cache")
				}
				cold := startTestConfiguration(router, "cold.test")
				// Always drain the in-flight request before closing the router's cache.
				defer func() {
					unblock()
					for range cold {
					}
				}()
				awaitDiscoveryTestResult(t, entered)

				warmStatus := make(chan int, 1)
				go func() {
					w := httptest.NewRecorder()
					_ = router.HandleOAuth2Auth(w, httptest.NewRequest(http.MethodGet, "https://warm.test/private", nil), nil)
					warmStatus <- w.Code
				}()
				if status := awaitDiscoveryTestResult(t, warmStatus); status != http.StatusFound {
					t.Fatalf("warm-cache login returned %d, want redirect", status)
				}
				getDone := make(chan int, 1)
				go func() {
					w := httptest.NewRecorder()
					router.HandleSetOAuth2Settings(w, httptest.NewRequest(http.MethodGet, "/settings", nil))
					getDone <- w.Code
				}()
				if status := awaitDiscoveryTestResult(t, getDone); status != http.StatusOK {
					t.Fatalf("settings GET returned %d", status)
				}
				updated := make(chan struct{})
				go func() {
					defer close(updated)
					if method == http.MethodPost {
						saveTestSettings(t, router, "https://new.test/userinfo", "new-group")
					} else {
						router.HandleSetOAuth2Settings(httptest.NewRecorder(), httptest.NewRequest(http.MethodDelete, "/settings", nil))
					}
				}()
				awaitDiscoveryTestResult(t, updated)

				// The same host in the new generation must not join the old flight.
				current := awaitDiscoveryTestResult(t, startTestConfiguration(router, "cold.test"))
				checkCurrent := func(result testConfigurationResult) {
					t.Helper()
					wantClient, wantUserInfo, wantGroups := "", "", ""
					if method == http.MethodPost {
						wantClient, wantUserInfo, wantGroups = "client", "https://new.test/userinfo", "new-group"
					}
					if result.config == nil || result.config.ClientID != wantClient || result.options.OAuth2UserInfoUrl != wantUserInfo || result.options.OAuth2DefaultAllowedGroups != wantGroups {
						t.Fatalf("stale discovery escaped the generation check: %+v", result)
					}
				}
				checkCurrent(current)
				unblock()
				checkCurrent(awaitDiscoveryTestResult(t, cold))
				checkCurrent(awaitDiscoveryTestResult(t, startTestConfiguration(router, "cold.test")))
				if calls.Load() != 2 {
					t.Fatalf("unexpected discovery retries: %d", calls.Load())
				}
			})
		}
	}
}

func TestConcurrentDiscoveryMissesShareOneLoad(t *testing.T) {
	router := newSettingsTestRouter(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	var enteredOnce, releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	discovery := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		enteredOnce.Do(func() { close(entered) })
		<-release
		_ = json.NewEncoder(w).Encode(OIDCDiscoveryDocument{
			AuthorizationEndpoint: "https://issuer.test/authorize",
			TokenEndpoint:         "https://issuer.test/token", UserinfoEndpoint: "https://issuer.test/userinfo",
		})
	}))
	defer discovery.Close()
	defer unblock()
	// Initialize before starting requests; no live settings are changed here.
	router.options.OAuth2WellKnownUrl = discovery.URL
	var pending []<-chan testConfigurationResult
	defer func() {
		unblock()
		for _, result := range pending {
			for range result {
			}
		}
	}()
	for range 12 {
		pending = append(pending, startTestConfiguration(router, "app.test"))
	}
	awaitDiscoveryTestResult(t, entered)
	unblock()
	for _, result := range pending {
		loaded := awaitDiscoveryTestResult(t, result)
		if loaded.config == nil || loaded.config.Endpoint.AuthURL != "https://issuer.test/authorize" || loaded.options.OAuth2UserInfoUrl != "https://issuer.test/userinfo" {
			t.Fatal("concurrent request did not receive the discovered configuration")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("concurrent cache misses made %d discovery calls, want 1", calls.Load())
	}
}

func TestDiscoveryRecoversWithoutAnIdlePeriod(t *testing.T) {
	for _, failure := range []struct {
		name   string
		status int
		body   string
	}{
		{"invalid-json", http.StatusOK, "invalid discovery JSON"},
		{"provider-unavailable", http.StatusServiceUnavailable, `{"error":"temporarily_unavailable"}`},
		{"empty-document", http.StatusOK, `{}`},
		{"missing-authorization", http.StatusOK, `{"token_endpoint":"https://issuer.test/token","userinfo_endpoint":"https://issuer.test/userinfo"}`},
		{"missing-token", http.StatusOK, `{"authorization_endpoint":"https://issuer.test/authorize","userinfo_endpoint":"https://issuer.test/userinfo"}`},
		{"missing-userinfo", http.StatusOK, `{"authorization_endpoint":"https://issuer.test/authorize","token_endpoint":"https://issuer.test/token"}`},
	} {
		t.Run(failure.name, func(t *testing.T) {
			router := newSettingsTestRouter(t)
			var recovered atomic.Bool
			var calls atomic.Int32
			discovery := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if !recovered.Load() {
					w.WriteHeader(failure.status)
					_, _ = w.Write([]byte(failure.body))
					return
				}
				_ = json.NewEncoder(w).Encode(OIDCDiscoveryDocument{
					AuthorizationEndpoint: "https://issuer.test/authorize",
					TokenEndpoint:         "https://issuer.test/token", UserinfoEndpoint: "https://issuer.test/userinfo",
				})
			}))
			defer discovery.Close()
			router.options.OAuth2WellKnownUrl = discovery.URL

			// These requests run back to back, well inside the cache lifetime. Failed
			// discovery must return an error without creating a renewable negative entry.
			for range 3 {
				w := httptest.NewRecorder()
				err := router.HandleOAuth2Auth(w, httptest.NewRequest(http.MethodGet, "https://app.test/private", nil), nil)
				if err == nil || w.Code != http.StatusInternalServerError {
					t.Fatalf("discovery failure: status %d, error %v", w.Code, err)
				}
			}

			recovered.Store(true)
			for range 10 {
				w := httptest.NewRecorder()
				_ = router.HandleOAuth2Auth(w, httptest.NewRequest(http.MethodGet, "https://app.test/private", nil), nil)
				if w.Code != http.StatusFound || !strings.HasPrefix(w.Header().Get("Location"), "https://issuer.test/authorize?") {
					t.Fatalf("recovered provider did not resume login: %d %s", w.Code, w.Header().Get("Location"))
				}
			}
			if calls.Load() != 4 {
				t.Fatalf("successful discovery was not cached: %d calls, want 4", calls.Load())
			}
		})
	}
}

func TestDiscoveryPreservesExplicitEndpointOverrides(t *testing.T) {
	for _, endpoint := range []string{"authorization", "token", "userinfo"} {
		for _, missing := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/missing=%t", endpoint, missing), func(t *testing.T) {
				router := newSettingsTestRouter(t)
				document := OIDCDiscoveryDocument{
					AuthorizationEndpoint: "https://issuer.test/authorize",
					TokenEndpoint:         "https://issuer.test/token", UserinfoEndpoint: "https://issuer.test/userinfo",
				}
				override := "https://override.test/" + endpoint
				switch endpoint {
				case "authorization":
					router.options.OAuth2ServerURL = override
					if missing {
						document.AuthorizationEndpoint = ""
					}
				case "token":
					router.options.OAuth2TokenURL = override
					if missing {
						document.TokenEndpoint = ""
					}
				case "userinfo":
					router.options.OAuth2UserInfoUrl = override
					if missing {
						document.UserinfoEndpoint = ""
					}
				}
				discovery := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _ = json.NewEncoder(w).Encode(document) }))
				defer discovery.Close()
				router.options.OAuth2WellKnownUrl = discovery.URL
				config, options := router.requestConfiguration("app.test", "https://app.test/internal/oauth2")
				if config == nil {
					t.Fatal("explicit override did not complete discovery")
				}
				got := map[string]string{"authorization": config.Endpoint.AuthURL, "token": config.Endpoint.TokenURL, "userinfo": options.OAuth2UserInfoUrl}
				if got[endpoint] != override {
					t.Fatalf("explicit override changed: %v", got)
				}
				w := httptest.NewRecorder()
				_ = router.HandleOAuth2Auth(w, httptest.NewRequest(http.MethodGet, "https://app.test/private", nil), nil)
				if w.Code != http.StatusFound || !strings.HasPrefix(w.Header().Get("Location"), config.Endpoint.AuthURL+"?") {
					t.Fatalf("login failed: %d %s", w.Code, w.Header().Get("Location"))
				}
			})
		}
	}
}
