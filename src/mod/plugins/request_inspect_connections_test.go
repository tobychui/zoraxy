package plugins

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Bursts isolate idle-pool churn instead of conflating it with active capacity.
func BenchmarkInspectConnectionBursts(b *testing.B) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		time.Sleep(time.Millisecond)
		io.WriteString(w, `{"action":"allow"}`)
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	port, _ := strconv.Atoi(u.Port())
	ref := &inspectorRef{port: port, inspectPath: "/inspect"}
	var fresh atomic.Int64
	ctx := httptrace.WithClientTrace(context.Background(), &httptrace.ClientTrace{GotConn: func(g httptrace.GotConnInfo) {
		if !g.Reused {
			fresh.Add(1)
		}
	}})
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r := httptest.NewRequest("GET", "http://poc.test/", nil).WithContext(ctx)
				if _, err := ref.inspectRequest(httptest.NewRecorder(), r, RequestInspectTarget{}, inspectSettings{maxBody: 65536}); err != nil {
					b.Error(err)
				}
			}()
		}
		wg.Wait()
	}
	b.ReportMetric(float64(fresh.Load())/float64(b.N*8), "newconn/req")
}
