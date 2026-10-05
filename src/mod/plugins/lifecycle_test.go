package plugins

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"imuslab.com/zoraxy/mod/info/logger"
	zoraxyPlugin "imuslab.com/zoraxy/mod/plugins/zoraxy_plugin"
)

// TestStartPluginOutputReadersDoNotRaceProcessField starts a plugin that writes
// to stdout and stderr immediately. The output readers must not read
// Plugin.process while StartPlugin assigns it (run with -race), see #1326.
func TestStartPluginOutputReadersDoNotRaceProcessField(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a start.sh entry point")
	}
	dir := filepath.Join(t.TempDir(), "racer")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho started\necho started >&2\nexec sleep 30\n"
	if err := os.WriteFile(filepath.Join(dir, "start.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	lg, err := logger.NewFmtLogger()
	if err != nil {
		t.Fatal(err)
	}
	p := &Plugin{RootDir: dir, Spec: &zoraxyPlugin.IntroSpect{ID: "racer", Name: "racer"}}
	m := &Manager{
		LoadedPlugins: map[string]*Plugin{"racer": p},
		Options: &ManagerOptions{
			SystemConst: &zoraxyPlugin.RuntimeConstantValue{},
			Logger:      lg,
		},
	}
	if err := m.StartPlugin("racer"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond) // let both readers log their first line
	_ = p.process.Process.Kill()
	_, _ = p.process.Process.Wait()
}

// TestPluginOutputForUnknownPluginDoesNotPanic covers a line that arrives after
// the plugin entry is gone: it must reach the [unknown:<pid>] branch.
func TestPluginOutputForUnknownPluginDoesNotPanic(t *testing.T) {
	lg, err := logger.NewFmtLogger()
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{LoadedPlugins: map[string]*Plugin{}, Options: &ManagerOptions{Logger: lg}}
	m.handlePluginSTDOUT("gone", 1234, "late line")
	m.handlePluginSTDERR("gone", 1234, "late line")
}
