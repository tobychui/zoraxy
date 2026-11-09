//go:build !mipsle && !riscv64 && !fsdb

package oauth2

import (
	"errors"
	"path/filepath"
	"testing"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/database/dbinc"
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

type failingPolicyBackend struct {
	dbinc.Backend
}

func (db failingPolicyBackend) Write(table, key string, value interface{}) error {
	if table == "oauth2" && (key == oauth2SettingsKey || key == "oauth2DefaultAllowedGroups") {
		return errors.New("injected policy persistence failure")
	}
	return db.Backend.Write(table, key, value)
}

func failSettingsTestWrites(t *testing.T, router *OAuth2Router) func() {
	t.Helper()
	backend := router.options.Database.Backend
	router.options.Database.Backend = failingPolicyBackend{backend}
	restore := func() { router.options.Database.Backend = backend }
	t.Cleanup(restore)
	return restore
}
