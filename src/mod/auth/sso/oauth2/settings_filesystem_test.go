//go:build mipsle || riscv64 || fsdb

package oauth2

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/database/dbinc"
	"imuslab.com/zoraxy/mod/info/logger"
)

func newFilesystemSettingsRouter(t *testing.T) *OAuth2Router {
	t.Helper()
	db := &database.Database{Db: t.TempDir(), BackendType: dbinc.BackendFSOnly}
	log, _ := logger.NewFmtLogger()
	router := NewOAuth2Router(&OAuth2RouterOptions{Database: db, Logger: log})
	t.Cleanup(router.options.OAuth2ConfigCache.Stop)
	return router
}

func TestFilesystemSettingsStartupFailsClosed(t *testing.T) {
	for _, failure := range []string{"unreadable", "corrupt"} {
		t.Run(failure, func(t *testing.T) {
			router := newFilesystemSettingsRouter(t)
			for key, value := range map[string]string{
				"oauth2ClientId": "legacy", "oauth2ClientSecret": "secret", "oauth2DefaultAllowedGroups": "",
				"oauth2ServerUrl": "https://legacy.test/authorize", "oauth2TokenUrl": "https://legacy.test/token", "oauth2UserInfoUrl": "https://legacy.test/userinfo",
			} {
				if err := router.options.Database.Write("oauth2", key, value); err != nil {
					t.Fatal(err)
				}
			}
			// Confirm actual absence still allows loading the legacy installation.
			legacy := restartSettingsTestRouter(t, router)
			if legacy.options.OAuth2ClientId != "legacy" {
				t.Fatal("legacy settings did not load")
			}
			saveTestSettings(t, router, "https://new.test/userinfo", "admins")
			path := filepath.Join(router.options.Database.Db.(string), "oauth2", "settings.entry")
			if failure == "unreadable" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.WriteFile(path, []byte(`{"oauth2ClientId":`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			restarted := restartSettingsTestRouter(t, router)
			w := httptest.NewRecorder()
			_ = restarted.HandleOAuth2Auth(w, httptest.NewRequest(http.MethodGet, "https://app.test/private", nil), nil)
			if w.Code != http.StatusInternalServerError || w.Header().Get("Location") != "" {
				t.Fatalf("unreadable record restored login: %d %s", w.Code, w.Header().Get("Location"))
			}
		})
	}
}
