package oauth2

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"golang.org/x/oauth2"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/info/logger"
)

func newSettingsTestRouter(t *testing.T) *OAuth2Router {
	t.Helper()
	db, err := database.NewDatabase(filepath.Join(t.TempDir(), "settings.db"), database.GetRecommendedBackendType())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	log, _ := logger.NewFmtLogger()
	router := NewOAuth2Router(&OAuth2RouterOptions{Database: db, Logger: log})
	t.Cleanup(router.options.OAuth2ConfigCache.Stop)
	return router
}

func saveTestSettings(t *testing.T, router *OAuth2Router, userInfoURL, groups string) {
	t.Helper()
	form := url.Values{
		"oauth2ClientId": {"client"}, "oauth2ClientSecret": {"secret"},
		"oauth2CodeChallengeMethod": {"none"}, "oauth2ConfigurationCacheTime": {"60s"},
		"oauth2ServerUrl": {"https://issuer.test/authorize"}, "oauth2TokenUrl": {"https://issuer.test/token"},
		"oauth2UserInfoUrl": {userInfoURL}, "oauth2Scopes": {"profile"},
		"oauth2DefaultAllowedGroups": {groups},
	}
	r := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.HandleSetOAuth2Settings(w, r)
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "error") {
		t.Fatalf("saving settings: %d %s", w.Code, w.Body.String())
	}
}

func TestGlobalPolicyConcurrentSettingsAndAuthorization(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sub":"alice","preferred_username":"alice","groups":["admins"]}`))
	}))
	defer upstream.Close()
	router := newSettingsTestRouter(t)
	saveTestSettings(t, router, upstream.URL, "admins")
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() {
			for range 60 {
				r := httptest.NewRequest(http.MethodGet, "https://app.test/private", nil)
				_, options := router.requestConfiguration(r.Host, "https://app.test/internal/oauth2")
				cookie, sessionErr := router.createSession(r, &oauth2.Token{AccessToken: "valid"}, "alice", options.settingsGeneration)
				if sessionErr == nil {
					r.AddCookie(cookie)
				}
				w := httptest.NewRecorder()
				err := router.HandleOAuth2Auth(w, r, &AccessPolicy{ForwardUserInfo: true})
				// A settings change also invalidates sessions and requires login.
				if w.Code != http.StatusOK && w.Code != http.StatusForbidden && w.Code != http.StatusFound {
					t.Errorf("unexpected authorization result: %d, %v", w.Code, err)
				}
				if HasVerifiedIdentityHeaders(r) != (err == nil) {
					t.Error("identity evidence must only be set after successful authorization")
				}
			}
		})
	}
	wg.Go(func() {
		for range 60 {
			saveTestSettings(t, router, upstream.URL, "guests")
			saveTestSettings(t, router, upstream.URL, "admins")
		}
	})
	wg.Wait()

	// An explicit route policy still overrides the global default.
	r := httptest.NewRequest(http.MethodGet, "https://app.test/private", nil)
	_, options := router.requestConfiguration(r.Host, "https://app.test/internal/oauth2")
	cookie, err := router.createSession(r, &oauth2.Token{AccessToken: "valid"}, "alice", options.settingsGeneration)
	if err != nil {
		t.Fatal(err)
	}
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	if err := router.HandleOAuth2Auth(w, r, &AccessPolicy{AllowedGroups: []string{"guests"}}); err == nil || w.Code != http.StatusForbidden {
		t.Fatal("route group restrictions were not enforced")
	}
}

func TestGlobalPolicyConcurrentPostDeleteGetAndSnapshots(t *testing.T) {
	router := newSettingsTestRouter(t)
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() {
			for range 100 {
				config, options := router.requestConfiguration("app.test", "https://app.test/internal/oauth2")
				if (options.OAuth2DefaultAllowedGroups == "admins") != (config.ClientID == "client") {
					t.Error("authorization policy and cached config came from different settings")
				}
				w := httptest.NewRecorder()
				router.HandleSetOAuth2Settings(w, httptest.NewRequest(http.MethodGet, "/settings", nil))
				var settings map[string]string
				if err := json.Unmarshal(w.Body.Bytes(), &settings); err != nil {
					t.Error(err)
				}
				if (settings["oauth2DefaultAllowedGroups"] == "admins") != (settings["oauth2ClientId"] == "client") {
					t.Error("settings GET returned a partially updated policy")
				}
			}
		})
	}
	wg.Go(func() {
		for range 60 {
			saveTestSettings(t, router, "https://issuer.test/userinfo", "admins")
			router.HandleSetOAuth2Settings(httptest.NewRecorder(), httptest.NewRequest(http.MethodDelete, "/settings", nil))
		}
	})
	wg.Wait()
	config, options := router.requestConfiguration("app.test", "https://app.test/internal/oauth2")
	if options.OAuth2DefaultAllowedGroups != "" || config.ClientID != "" {
		t.Fatal("deleted policy or cached configuration survived the final DELETE")
	}
}
