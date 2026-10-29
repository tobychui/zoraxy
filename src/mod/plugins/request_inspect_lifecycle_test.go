package plugins

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"imuslab.com/zoraxy/mod/auth"
	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/database/dbinc"
	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
	"imuslab.com/zoraxy/mod/eventsystem"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/plugins/zoraxy_plugin"
)

const helperPluginEnv = "ZORAXY_INSPECT_HELPER_PLUGIN"

// TestInspectHelperPluginProcess is not a test: it is the plugin process started
// by lifecycle tests through a start.sh entry point. It denies every inspection.
func TestInspectHelperPluginProcess(t *testing.T) {
	if os.Getenv(helperPluginEnv) != "1" {
		return
	}
	var spec zoraxy_plugin.ConfigureSpec
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "-configure=") {
			json.Unmarshal([]byte(strings.TrimPrefix(arg, "-configure=")), &spec)
		}
	}
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM)
	go func() { <-sigs; os.Exit(0) }()
	mux := http.NewServeMux()
	mux.HandleFunc("/policy", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"mode":"block","fail_closed":true,"invert_failure_hosts":["open.test"]}`)
	})
	mux.HandleFunc("/inspect", func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		io.WriteString(w, `{"action":"deny","reason":"helper"}`)
	})
	mux.HandleFunc("/ui/term", func(w http.ResponseWriter, r *http.Request) {
		os.WriteFile("term-received", nil, 0644)
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		go func() { time.Sleep(50 * time.Millisecond); os.Exit(0) }()
	})
	http.ListenAndServe("127.0.0.1:"+strconv.Itoa(spec.Port), mux)
	os.Exit(1)
}

// lifecycleManager returns a manager with one helper inspector plugin assigned to tag "waf".
func lifecycleManager(t *testing.T) (*Manager, *Plugin) {
	t.Helper()
	tmp := t.TempDir()
	log, err := logger.NewLogger("test", filepath.Join(tmp, "log"))
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.NewDatabase(filepath.Join(tmp, "test.db"), dbinc.BackendBoltDB)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	eventsystem.InitEventSystem(log)
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(tmp, "inspector")
	os.MkdirAll(dir, 0755)
	script := "#!/bin/sh\n" + helperPluginEnv + "=1 exec '" + self + "' -test.run='^TestInspectHelperPluginProcess$' -- \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "start.sh"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	m := &Manager{
		LoadedPlugins: map[string]*Plugin{},
		tagPluginList: map[string][]*Plugin{},
		PluginHash:    map[string]string{},
		Options: &ManagerOptions{
			PluginGroups:  map[string][]string{"waf": {"inspector"}},
			SystemConst:   &zoraxy_plugin.RuntimeConstantValue{},
			Database:      db,
			Logger:        log,
			APIKeyManager: auth.NewAPIKeyManager(),
		},
	}
	db.NewTable("plugins")
	p := &Plugin{
		RootDir:          dir,
		Spec:             &zoraxy_plugin.IntroSpect{ID: "inspector", Name: "inspector", UIPath: "/ui", RequestInspect: "/inspect"},
		staticRouteProxy: map[string]*dpcore.ReverseProxy{},
	}
	m.LoadedPlugins["inspector"] = p
	m.UpdateTagsToPluginMaps()
	t.Cleanup(func() {
		if p.IsRunning() {
			p.process.Process.Kill()
		}
	})
	return m, p
}

// Selection must follow enable/disable/restart (new assigned port) while
// requests, tag rebuilds and lifecycle changes run concurrently.
func TestInspectSnapshotConcurrentLifecycle(t *testing.T) {
	m, _ := lifecycleManager(t)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	var inspected atomic.Int64
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				r := httptest.NewRequest("POST", "http://example.test/", strings.NewReader("body"))
				if handled, _ := m.InspectRoute(newDeadlineRecorder(), r, RequestInspectTarget{RouteID: "example.test", Tags: []string{"waf"}}); handled {
					inspected.Add(1)
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				m.UpdateTagsToPluginMaps()
				time.Sleep(time.Millisecond)
			}
		}
	}()
	ports := map[int]bool{}
	for i := 0; i < 3; i++ {
		if err := m.EnablePlugin("inspector"); err != nil {
			t.Fatal(err)
		}
		ports[m.inspectors.Load().tags["waf"][0].port] = true
		before := inspected.Load()
		deadline := time.Now().Add(5 * time.Second)
		for inspected.Load() == before && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if inspected.Load() == before {
			t.Fatalf("cycle %d: enabled inspector was never selected", i)
		}
		if err := m.DisablePlugin("inspector"); err != nil {
			t.Fatal(err)
		}
		if n := len(m.inspectors.Load().tags["waf"]); n != 0 {
			t.Fatalf("cycle %d: disabled inspector still selected (%d)", i, n)
		}
		after := inspected.Load()
		r := httptest.NewRequest("GET", "http://example.test/", nil)
		if handled, _ := m.InspectRoute(newDeadlineRecorder(), r, RequestInspectTarget{Tags: []string{"waf"}}); handled || inspected.Load() > after+4 {
			t.Fatalf("cycle %d: disabled inspector still answered", i)
		}
	}
	close(stop)
	wg.Wait()
	if len(ports) < 2 {
		t.Logf("restarts reused the same random port: %v", ports)
	}
}

// A disabling plugin is withdrawn from selection before its shutdown request.
func TestInspectDisableWithdrawsBeforeShutdown(t *testing.T) {
	m, p := lifecycleManager(t)
	if err := m.EnablePlugin("inspector"); err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "http://example.test/", nil)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if handled, _ := m.InspectRoute(newDeadlineRecorder(), r, RequestInspectTarget{Tags: []string{"waf"}}); handled {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper inspector never answered")
		}
		time.Sleep(20 * time.Millisecond)
	}
	done := make(chan error, 1)
	go func() { done <- m.DisablePlugin("inspector") }()
	marker := filepath.Join(p.RootDir, "term-received")
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("termination request not received")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if handled, _ := m.InspectRoute(newDeadlineRecorder(), r, RequestInspectTarget{Tags: []string{"waf"}}); handled {
		t.Fatal("stopping plugin still selected for inspection")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// An exit of a replaced process must not disable the instance that superseded it.
func TestInspectStaleExitKeepsRestartedPlugin(t *testing.T) {
	m, p := lifecycleManager(t)
	old, current := exec.Command("true"), exec.Command("true")
	m.lifecycleMutex.Lock()
	p.process, p.Enabled, p.AssignedPort = current, true, 1
	m.lifecycleMutex.Unlock()
	m.UpdateTagsToPluginMaps()
	m.handlePluginExit(p, old, io.ErrUnexpectedEOF)
	if !p.Enabled || len(m.inspectors.Load().tags["waf"]) != 1 {
		t.Fatal("stale process exit disabled the restarted plugin")
	}
	m.handlePluginExit(p, current, io.ErrUnexpectedEOF)
	if p.Enabled || len(m.inspectors.Load().tags["waf"]) != 1 {
		t.Fatal("crash of the current process did not retain its inspection binding")
	}
}
