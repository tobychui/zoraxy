package plugins

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/info/logger"
)

// pluginListHandler serves a plugin store index that contains a single plugin with the given ID
func pluginListHandler(pluginID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"IconPath":"","PluginIntroSpect":{"id":"` + pluginID + `","name":"Test","version_major":1,"version_minor":2,"version_patch":3},"DownloadURLs":{}}]`))
	}
}

// newAutoSyncTestManager creates a manager whose plugin store is a local test server
// using the given handler, so the test needs no network access.
// The returned counter counts the store requests.
func newAutoSyncTestManager(t *testing.T, handler http.HandlerFunc) (*Manager, *atomic.Int32) {
	t.Helper()

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		handler(w, r)
	}))
	t.Cleanup(server.Close)

	testLogger, err := logger.NewFmtLogger()
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}

	return &Manager{
		Options: &ManagerOptions{
			PluginStoreURLs: []string{server.URL},
			Database:        &database.Database{Backend: &fakeBackend{}},
			Logger:          testLogger,
		},
	}, &hits
}

// waitFor polls until the condition is true, and fails the test after a timeout
func waitFor(t *testing.T, condition func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestAutoSyncRefreshesPluginList(t *testing.T) {
	m, hits := newAutoSyncTestManager(t, pluginListHandler("test.plugin"))

	if err := m.StartAutoSync(20 * time.Millisecond); err != nil {
		t.Fatalf("expected auto sync to start, got %v", err)
	}
	defer m.StopAutoSync()

	//Read the cache while the ticker is writing it, `go test -race` fails if the access is not locked
	waitFor(t, func() bool {
		m.getDownloadablePluginByID("test.plugin")
		m.ListDownloadablePlugins()
		return hits.Load() >= 3
	}, "3 automatic syncs")

	if m.getDownloadablePluginByID("test.plugin") == nil {
		t.Fatalf("expected the plugin to be in the cache after auto sync")
	}
}

func TestAutoSyncStartTwiceAndStop(t *testing.T) {
	m, hits := newAutoSyncTestManager(t, pluginListHandler("test.plugin"))

	if err := m.StartAutoSync(20 * time.Millisecond); err != nil {
		t.Fatalf("expected auto sync to start, got %v", err)
	}
	if err := m.StartAutoSync(20 * time.Millisecond); err == nil {
		t.Fatalf("expected an error when starting auto sync twice")
	}

	m.StopAutoSync()
	m.StopAutoSync() //Stopping twice must be safe

	//Let a sync that was already running finish, then make sure no new ones start.
	//With a 20ms interval, a ticker that keeps running would sync several times in 150ms.
	time.Sleep(100 * time.Millisecond)
	after := hits.Load()
	time.Sleep(150 * time.Millisecond)
	if hits.Load() != after {
		t.Fatalf("expected no more syncs after StopAutoSync, got %d more", hits.Load()-after)
	}

	//It must be possible to start it again after it was stopped
	if err := m.StartAutoSync(time.Hour); err != nil {
		t.Fatalf("expected auto sync to restart, got %v", err)
	}
	m.StopAutoSync()
}

func TestAutoSyncSurvivesFailedSync(t *testing.T) {
	var failing atomic.Bool
	servePlugin := pluginListHandler("test.plugin2")
	m, hits := newAutoSyncTestManager(t, func(w http.ResponseWriter, r *http.Request) {
		if failing.Load() {
			http.Error(w, "store unavailable", http.StatusInternalServerError)
			return
		}
		servePlugin(w, r)
	})

	//Initial sync works and fills the cache
	if err := m.UpdateDownloadablePluginList(); err != nil {
		t.Fatalf("expected the initial sync to work, got %v", err)
	}

	//The store goes down, the auto sync must keep trying and keep the old plugin list
	failing.Store(true)
	if err := m.StartAutoSync(20 * time.Millisecond); err != nil {
		t.Fatalf("expected auto sync to start, got %v", err)
	}
	defer m.StopAutoSync()

	failedFrom := hits.Load()
	waitFor(t, func() bool { return hits.Load() >= failedFrom+3 }, "3 failed auto syncs")
	if m.getDownloadablePluginByID("test.plugin2") == nil {
		t.Fatalf("expected the old plugin list to be kept after failed syncs")
	}

	//The store comes back, the auto sync must pick up again
	failing.Store(false)
	recoveredFrom := hits.Load()
	waitFor(t, func() bool { return hits.Load() >= recoveredFrom+2 }, "auto syncs after the store recovered")
	if m.getDownloadablePluginByID("test.plugin2") == nil {
		t.Fatalf("expected the plugin list to be available after the store recovered")
	}
}
