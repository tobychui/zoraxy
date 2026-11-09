//go:build !mipsle && !riscv64 && !fsdb

package oauth2

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/database/dbinc"
	"imuslab.com/zoraxy/mod/info/logger"
)

type unreadableSettingsBackend struct{ dbinc.Backend }

func (db unreadableSettingsBackend) KeyExists(table, key string) bool {
	if key == oauth2SettingsKey {
		return false
	}
	return db.Backend.KeyExists(table, key)
}
func (db unreadableSettingsBackend) Read(table, key string, value interface{}) error {
	if key == oauth2SettingsKey {
		return errors.New("injected lookup failure")
	}
	return db.Backend.Read(table, key, value)
}

func TestSettingsStartupFailsClosed(t *testing.T) {
	for _, backendType := range []dbinc.BackendType{dbinc.BackendBoltDB, dbinc.BackendLevelDB} {
		for _, failure := range []string{"lookup", "decode", "legacy-decode"} {
			t.Run(backendType.String()+"/"+failure, func(t *testing.T) {
				db, err := database.NewDatabase(filepath.Join(t.TempDir(), "db"), backendType)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(db.Close)
				log, _ := logger.NewFmtLogger()
				router := NewOAuth2Router(&OAuth2RouterOptions{Database: db, Logger: log})
				t.Cleanup(router.options.OAuth2ConfigCache.Stop)
				for key, value := range map[string]interface{}{
					"oauth2ClientId": "legacy", "oauth2ClientSecret": "secret",
					"oauth2ServerUrl": "https://legacy.test/authorize", "oauth2TokenUrl": "https://legacy.test/token",
					"oauth2UserInfoUrl": "https://legacy.test/userinfo", "oauth2DefaultAllowedGroups": "",
				} {
					if err := db.Write("oauth2", key, value); err != nil {
						t.Fatal(err)
					}
				}
				if failure == "legacy-decode" {
					if err := db.Write("oauth2", "oauth2DefaultAllowedGroups", 123); err != nil {
						t.Fatal(err)
					}
				} else {
					saveTestSettings(t, router, "https://new.test/userinfo", "admins")
					if failure == "lookup" {
						db.Backend = unreadableSettingsBackend{db.Backend}
					} else {
						if err := db.Write("oauth2", oauth2SettingsKey, map[string]interface{}{
							"oauth2ClientId": "new", "oauth2ClientSecret": "secret",
							"oauth2ServerUrl": "https://new.test/authorize", "oauth2TokenUrl": "https://new.test/token",
							"oauth2UserInfoUrl": "https://new.test/userinfo", "oauth2DefaultAllowedGroups": 123,
						}); err != nil {
							t.Fatal(err)
						}
					}
				}
				restarted := restartSettingsTestRouter(t, router)
				w := httptest.NewRecorder()
				err = restarted.HandleOAuth2Auth(w, httptest.NewRequest(http.MethodGet, "https://app.test/private", nil), nil)
				if err == nil || w.Code != http.StatusInternalServerError || w.Header().Get("Location") != "" {
					t.Fatalf("unreadable settings enabled login: status %d, location %s, error %v", w.Code, w.Header().Get("Location"), err)
				}
			})
		}
	}
}
