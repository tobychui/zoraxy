package oauth2

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func settingsRequest(groups string) *http.Request {
	form := url.Values{
		"oauth2ClientId": {"new-client"}, "oauth2ClientSecret": {"new-secret"},
		"oauth2CodeChallengeMethod": {"S256"}, "oauth2ConfigurationCacheTime": {"90s"},
		"oauth2ServerUrl": {"https://new.test/authorize"}, "oauth2TokenUrl": {"https://new.test/token"},
		"oauth2UserInfoUrl": {"https://new.test/userinfo"}, "oauth2Scopes": {"profile email"},
		"oauth2DefaultAllowedGroups": {groups},
	}
	r := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return r
}

func restartSettingsTestRouter(t *testing.T, router *OAuth2Router) *OAuth2Router {
	t.Helper()
	restarted := NewOAuth2Router(&OAuth2RouterOptions{Database: router.options.Database, Logger: router.options.Logger})
	t.Cleanup(restarted.options.OAuth2ConfigCache.Stop)
	return restarted
}

func TestGlobalSettingsPersistenceFailurePreservesPolicy(t *testing.T) {
	for _, initialGroups := range []string{"", "admins"} {
		for _, method := range []string{http.MethodPost, http.MethodDelete} {
			t.Run(initialGroups+"/"+method, func(t *testing.T) {
				router := newSettingsTestRouter(t)
				saveTestSettings(t, router, "https://old.test/userinfo", initialGroups)
				before, err := router.options.Database.ListTable("oauth2")
				if err != nil {
					t.Fatal(err)
				}
				config, options := router.requestConfiguration("app.test", "https://app.test/internal/oauth2")
				cookie, err := router.createSession(httptest.NewRequest(http.MethodGet, "https://app.test/", nil),
					&oauth2.Token{AccessToken: "valid"}, "alice", options.settingsGeneration)
				if err != nil {
					t.Fatal(err)
				}
				restoreWrites := failSettingsTestWrites(t, router)
				groups := "admins"
				if initialGroups != "" {
					groups = ""
				}
				r := settingsRequest(groups)
				if method == http.MethodDelete {
					r = httptest.NewRequest(method, "/settings", nil)
				}
				w := httptest.NewRecorder()
				router.HandleSetOAuth2Settings(w, r)
				restoreWrites()
				if !strings.Contains(w.Body.String(), "error") {
					t.Fatalf("failed write reported success: %s", w.Body.String())
				}
				currentConfig, currentOptions := router.requestConfiguration("app.test", "https://app.test/internal/oauth2")
				if currentConfig != config || !reflect.DeepEqual(currentOptions, options) {
					t.Fatal("failed save changed runtime settings, generation, or cached configuration")
				}
				if router.sessions.Get(cookie.Value) == nil {
					t.Fatal("failed save invalidated existing sessions")
				}
				after, err := router.options.Database.ListTable("oauth2")
				if err != nil || !reflect.DeepEqual(before, after) {
					t.Fatalf("failed save changed persisted settings: %v", err)
				}
				restarted := restartSettingsTestRouter(t, router)
				if restarted.options.OAuth2DefaultAllowedGroups != initialGroups || restarted.options.OAuth2ClientId != options.OAuth2ClientId {
					t.Fatal("restart did not retain the previously saved policy and provider")
				}
			})
		}
	}
}

func TestGlobalSettingsMigrateLegacyAndReset(t *testing.T) {
	router := newSettingsTestRouter(t)
	for key, value := range map[string]string{
		"oauth2ClientId": "legacy-client", "oauth2ClientSecret": "legacy-secret",
		"oauth2ServerUrl": "https://legacy.test/authorize", "oauth2TokenUrl": "https://legacy.test/token",
		"oauth2UserInfoUrl": "https://legacy.test/userinfo", "oauth2DefaultAllowedGroups": "legacy-admins",
	} {
		if err := router.options.Database.Write("oauth2", key, value); err != nil {
			t.Fatal(err)
		}
	}
	router = restartSettingsTestRouter(t, router)
	if router.options.OAuth2ClientId != "legacy-client" || router.options.OAuth2DefaultAllowedGroups != "legacy-admins" {
		t.Fatal("legacy settings did not load")
	}
	restoreWrites := failSettingsTestWrites(t, router)
	w := httptest.NewRecorder()
	router.HandleSetOAuth2Settings(w, settingsRequest("admins"))
	restoreWrites()
	if !strings.Contains(w.Body.String(), "error") || router.options.Database.KeyExists("oauth2", oauth2SettingsKey) {
		t.Fatal("failed migration must report an error and leave legacy storage intact")
	}
	router = restartSettingsTestRouter(t, router)
	if router.options.OAuth2DefaultAllowedGroups != "legacy-admins" || router.options.OAuth2ClientId != "legacy-client" {
		t.Fatal("failed migration changed legacy settings")
	}

	_, options := router.requestConfiguration("app.test", "https://app.test/internal/oauth2")
	cookie, err := router.createSession(httptest.NewRequest(http.MethodGet, "https://app.test/", nil),
		&oauth2.Token{AccessToken: "valid"}, "alice", options.settingsGeneration)
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	router.HandleSetOAuth2Settings(w, settingsRequest(" admins, developers,admins "))
	if strings.Contains(w.Body.String(), "error") {
		t.Fatal(w.Body.String())
	}
	if router.settingsGeneration != options.settingsGeneration+1 || router.sessions.Get(cookie.Value) != nil || router.options.OAuth2ConfigCache.Len() != 0 {
		t.Fatal("successful save did not invalidate sessions and cached configuration")
	}
	router = restartSettingsTestRouter(t, router)
	if router.options.OAuth2DefaultAllowedGroups != "admins,developers" || router.options.OAuth2ClientId != "new-client" || *router.options.OAuth2ConfigurationCacheTime != 90*time.Second {
		t.Fatal("restart did not load the complete saved settings")
	}
	w = httptest.NewRecorder()
	router.HandleSetOAuth2Settings(w, httptest.NewRequest(http.MethodDelete, "/settings", nil))
	if strings.Contains(w.Body.String(), "error") {
		t.Fatal(w.Body.String())
	}
	router = restartSettingsTestRouter(t, router)
	if router.options.OAuth2ClientId != "" || router.options.OAuth2DefaultAllowedGroups != "" || router.options.OAuth2UserInfoUrl != "" {
		t.Fatal("reset resurrected legacy settings on restart")
	}
}
