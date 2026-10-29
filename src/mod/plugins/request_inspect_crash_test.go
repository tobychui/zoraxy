package plugins

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestInspectKilledPluginKeepsFailurePolicy(t *testing.T) {
	m, p := lifecycleManager(t)
	p.Spec.RequestInspectPolicy = "/policy"
	if err := m.EnablePlugin("inspector"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, status := m.InspectRoute(newDeadlineRecorder(), httptest.NewRequest("GET", "http://closed.test/", nil), inspectTarget("closed.test"))
		if status == 403 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper never answered")
		}
		time.Sleep(20 * time.Millisecond)
	}
	m.lifecycleMutex.Lock()
	process, exited := p.process.Process, p.processExited
	m.lifecycleMutex.Unlock()
	if err := process.Kill(); err != nil {
		t.Fatal(err)
	}
	<-exited
	for {
		m.lifecycleMutex.Lock()
		enabled := p.Enabled
		m.lifecycleMutex.Unlock()
		if !enabled {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("exit was not observed")
		}
		time.Sleep(time.Millisecond)
	}
	// Rebuilding tags must not drop a crashed inspector either.
	m.UpdateTagsToPluginMaps()
	for _, tc := range []struct {
		host string
		want int
	}{{"closed.test", 503}, {"open.test", 0}} {
		_, status := m.InspectRoute(newDeadlineRecorder(), httptest.NewRequest("GET", "http://"+tc.host+"/", nil), inspectTarget(tc.host))
		if status != tc.want {
			t.Fatalf("crashed inspector: host=%s status=%d want=%d", tc.host, status, tc.want)
		}
	}
	if err := m.DisablePlugin("inspector"); err != nil {
		t.Fatal(err)
	}
	if handled, _ := m.InspectRoute(newDeadlineRecorder(), httptest.NewRequest("GET", "http://closed.test/", nil), inspectTarget("closed.test")); handled {
		t.Fatal("admin disable retained inspector")
	}
}
