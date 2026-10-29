package plugins

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/database/dbinc"
)

func TestInspectPolicySurvivesColdStart(t *testing.T) {
	m, fake := policyManager(t, `{"mode":"block","invert_failure_hosts":["closed.test"]}`, `{"action":"allow"}`)
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plugins")
	root := filepath.Join(pluginDir, "waf")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	// The entry point no longer exists after restart: even introspection cannot succeed.
	m.LoadedPlugins["waf"].RootDir = root
	dbPath := filepath.Join(dir, "test.db")
	db, err := database.NewDatabase(dbPath, dbinc.BackendBoltDB)
	if err != nil {
		t.Fatal(err)
	}
	m.Options.Database = db
	db.NewTable("plugins")
	db.Write("plugins", "waf", true)
	m.inspectPolicy = nil
	m.UpdateTagsToPluginMaps()
	m.InspectRoute(newDeadlineRecorder(), httptest.NewRequest("GET", "http://closed.test/", nil), inspectTarget("closed.test"))
	if policies, _ := fake.counts(); policies != 1 {
		t.Fatal("policy not fetched")
	}
	db.Close()
	db, err = database.NewDatabase(dbPath, dbinc.BackendBoltDB)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	logManager, _ := lifecycleManager(t)
	options := &ManagerOptions{Database: db, Logger: logManager.Options.Logger, APIKeyManager: logManager.Options.APIKeyManager, PluginDir: pluginDir, PluginGroups: map[string][]string{"waf": {"waf"}}, PluginGroupsConfig: filepath.Join(dir, "groups.json")}
	// NewPluginManager creates an empty groups file; provide the configured binding.
	if err := os.WriteFile(options.PluginGroupsConfig, []byte(`{"waf":["waf"]}`), 0644); err != nil {
		t.Fatal(err)
	}
	cold := NewPluginManager(options)
	if err := cold.LoadPluginsFromDisk(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		host string
		want int
	}{{"closed.test", 503}, {"open.test", 0}} {
		_, status := cold.InspectRoute(newDeadlineRecorder(), httptest.NewRequest("GET", "http://"+tc.host+"/", nil), inspectTarget(tc.host))
		if status != tc.want {
			t.Fatalf("cold start: host=%s status=%d want=%d", tc.host, status, tc.want)
		}
	}
	// An explicit admin disable survives the next manager reconstruction too.
	if err := cold.DisablePlugin("waf"); err != nil {
		t.Fatal(err)
	}
	disabled := NewPluginManager(options)
	if err := disabled.LoadPluginsFromDisk(); err != nil {
		t.Fatal(err)
	}
	if handled, _ := disabled.InspectRoute(newDeadlineRecorder(), httptest.NewRequest("GET", "http://closed.test/", nil), inspectTarget("closed.test")); handled {
		t.Fatal("disabled persisted inspector selected")
	}
}
