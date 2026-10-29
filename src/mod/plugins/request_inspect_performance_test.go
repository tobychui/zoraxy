package plugins

import (
	"imuslab.com/zoraxy/mod/plugins/zoraxy_plugin"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func BenchmarkInspectFake(b *testing.B) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		io.Copy(io.Discard, r.Body)
		io.WriteString(w, `{"action":"allow"}`)
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	p := &Plugin{Enabled: true, AssignedPort: port, Spec: &zoraxy_plugin.IntroSpect{ID: "bench", RequestInspect: "/inspect", RequestInspectFailClosed: true}}
	m := &Manager{LoadedPlugins: map[string]*Plugin{"bench": p}, Options: &ManagerOptions{PluginGroups: map[string][]string{"waf": {"bench"}}}}
	m.UpdateTagsToPluginMaps()
	for _, size := range []int{33, 4096, 65536} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			body := strings.Repeat("a", size)
			calls.Store(0)
			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					r := httptest.NewRequest("POST", "http://poc.test/submit", strings.NewReader(body))
					w := newDeadlineRecorder()
					if m.InspectRequest(w, r, []string{"waf"}, "127.0.0.1") {
						b.Error("not allowed")
					}
					r.Body.Close()
				}
			})
			if got := calls.Load(); got != int64(b.N) {
				b.Fatalf("inspector calls=%d want=%d", got, b.N)
			}
		})
	}
}
