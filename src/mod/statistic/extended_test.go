package statistic

import (
	"math"
	"testing"
	"time"
)

func TestClassifyRequest(t *testing.T) {
	cases := []struct {
		ri   RequestInfo
		want StatusClass
	}{
		{RequestInfo{StatusCode: 200, Succ: true}, StatusClassSuccess},
		{RequestInfo{StatusCode: 101, Succ: true}, StatusClassSuccess},
		{RequestInfo{StatusCode: 304, Succ: true}, StatusClassRedirect},
		{RequestInfo{StatusCode: 307, Succ: false}, StatusClassRedirect},
		{RequestInfo{StatusCode: 404, Succ: true}, StatusClassError},
		{RequestInfo{StatusCode: 521, Succ: false}, StatusClassError},
		{RequestInfo{StatusCode: 403, Succ: false, Blocked: true}, StatusClassBlocked},
		{RequestInfo{StatusCode: 429, Succ: false, Blocked: true}, StatusClassBlocked},
	}
	for _, c := range cases {
		if got := ClassifyRequest(c.ri); got != c.want {
			t.Errorf("ClassifyRequest(%d, blocked=%v) = %v, want %v", c.ri.StatusCode, c.ri.Blocked, got, c.want)
		}
	}
}

func TestLatencyHistogramPercentiles(t *testing.T) {
	h := &latencyHistogram{}
	//90 fast requests (~3ms) and 10 slow requests (~400ms)
	for i := 0; i < 90; i++ {
		h.observe(3 * time.Millisecond)
	}
	for i := 0; i < 10; i++ {
		h.observe(400 * time.Millisecond)
	}
	e := h.export()

	if e.Count != 100 {
		t.Fatalf("expected 100 observations, got %d", e.Count)
	}
	if avg := e.AverageMs(); math.Abs(avg-42.7) > 0.01 {
		t.Fatalf("expected average 42.7ms, got %f", avg)
	}
	if p50 := e.PercentileMs(0.5); p50 < 2 || p50 > 5 {
		t.Fatalf("expected p50 inside the 2-5ms bucket, got %f", p50)
	}
	if p95 := e.PercentileMs(0.95); p95 < 300 || p95 > 500 {
		t.Fatalf("expected p95 inside the 300-500ms bucket, got %f", p95)
	}

	//Merging two histograms doubles the counts but keeps the percentiles
	merged := LatencyHistogramExport{}
	merged.Merge(e)
	merged.Merge(e)
	if merged.Count != 200 || merged.PercentileMs(0.95) != e.PercentileMs(0.95) {
		t.Fatalf("unexpected merge result: count %d, p95 %f", merged.Count, merged.PercentileMs(0.95))
	}

	//Empty histogram must not divide by zero
	if (LatencyHistogramExport{}).PercentileMs(0.99) != 0 || (LatencyHistogramExport{}).AverageMs() != 0 {
		t.Fatalf("empty histogram should report 0")
	}
}

func TestLatencyOverflowBucket(t *testing.T) {
	h := &latencyHistogram{}
	h.observe(2 * time.Minute)
	e := h.export()
	if e.Buckets[latencyBucketCount-1] != 1 {
		t.Fatalf("expected the observation in the overflow bucket")
	}
	if p := e.PercentileMs(1); p <= 30000 {
		t.Fatalf("expected overflow percentile above 30s, got %f", p)
	}
}

func TestRequestPathPattern(t *testing.T) {
	cases := map[string]string{
		"":                    "/",
		"/":                   "/",
		"/login":              "/login",
		"/favicon.ico":        "/favicon.ico",
		"/api/v1/users":       "/api/*",
		"/static/app.css":     "/static/*",
		"/admin/":             "/admin/*",
		"example.com/foo/bar": "/example.com/*",
	}
	for in, want := range cases {
		if got := requestPathPattern(in); got != want {
			t.Errorf("requestPathPattern(%q) = %q, want %q", in, got, want)
		}
	}
	if got := requestPathOf("example.com/api/v1"); got != "/api/v1" {
		t.Errorf("requestPathOf returned %q", got)
	}
	if got := requestPathOf("example.com"); got != "/" {
		t.Errorf("requestPathOf without path returned %q", got)
	}
}

func TestHostPathKey(t *testing.T) {
	cases := []struct {
		host, pattern, key, wantHost string
	}{
		{"Example.COM", "/api/*", "example.com/api/*", "example.com"},
		{"example.com:8443", "/", "example.com:8443/", "example.com:8443"},
		{"[::1]:8443", "/login", "[::1]:8443/login", "[::1]:8443"},
		{"", "/static/*", "/static/*", ""},
		{"evil.com/fake", "/x", "evil.comfake/x", "evil.comfake"},
	}
	for _, c := range cases {
		key := hostPathKey(c.host, c.pattern)
		if key != c.key {
			t.Errorf("hostPathKey(%q, %q) = %q, want %q", c.host, c.pattern, key, c.key)
		}
		host, pattern := SplitHostPathKey(key)
		if host != c.wantHost || pattern != c.pattern {
			t.Errorf("SplitHostPathKey(%q) = %q, %q, want %q, %q", key, host, pattern, c.wantHost, c.pattern)
		}
	}
}

func TestClassifyFileType(t *testing.T) {
	cases := []struct {
		contentType string
		path        string
		want        string
	}{
		{"text/html; charset=utf-8", "/", "html"},
		{"application/javascript", "/app.js", "js"},
		{"text/css", "/style.css", "css"},
		{"application/json", "/api/state", "json"},
		{"application/problem+json", "/api/state", "json"},
		{"image/png", "/logo.png", "image"},
		{"font/woff2", "/font.woff2", "font"},
		{"video/mp4", "/movie", "video"},
		{"application/octet-stream", "/download", "other"},
		{"", "/cached/app.js", "js"},
		{"", "/", "unknown"},
	}
	for _, c := range cases {
		if got := classifyFileType(c.contentType, c.path); got != c.want {
			t.Errorf("classifyFileType(%q, %q) = %q, want %q", c.contentType, c.path, got, c.want)
		}
	}
}

func TestIsBotUserAgent(t *testing.T) {
	bots := []string{
		"",
		"curl/8.4.0",
		"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)",
		"python-requests/2.31.0",
		"Go-http-client/1.1",
		"Uptime-Kuma/1.23.0",
	}
	for _, ua := range bots {
		if !isBotUserAgent(ua) {
			t.Errorf("expected %q to be a bot", ua)
		}
	}
	humans := []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.1 Mobile/15E148 Safari/604.1",
		"Home Assistant/2023.11.0-1234 (Android 14; Pixel 8)",
	}
	for _, ua := range humans {
		if isBotUserAgent(ua) {
			t.Errorf("expected %q not to be a bot", ua)
		}
	}
}

// Recording a request must fill the dashboard statistics and survive an export / import round trip
func TestRecordRequestDashboardFields(t *testing.T) {
	c := &Collector{
		DailySummary: NewDailySummary(),
		Option:       &CollectorOption{},
		incr:         newIncrFn(0),
	}

	record := func(ri RequestInfo) {
		c.RecordRequest(ri)
	}
	record(RequestInfo{IpAddr: "1.1.1.1", Succ: true, StatusCode: 200, UserAgent: "Mozilla/5.0 Chrome", RequestURL: "a.example.com/api/v1/state", Target: "a.example.com", Upstream: "10.0.0.1:8080", ProxyRule: "a.example.com", UpstreamLatency: 20 * time.Millisecond, ContentType: "application/json"})
	record(RequestInfo{IpAddr: "2.2.2.2", Succ: false, StatusCode: 403, UserAgent: "curl/8.0", RequestURL: "a.example.com/", Target: "a.example.com", Upstream: "", ProxyRule: "a.example.com", Blocked: true})
	record(RequestInfo{IpAddr: "1.1.1.1", Succ: true, StatusCode: 302, UserAgent: "Mozilla/5.0 Chrome", RequestURL: "a.example.com/login", Target: "a.example.com", Upstream: "10.0.0.1:8080", ProxyRule: "a.example.com", UpstreamLatency: 5 * time.Millisecond, ContentType: "text/html"})

	//RecordRequest runs in a goroutine
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		e := DailySummaryToExport(*c.DailySummary)
		if e.RequestStatus.Total() == 3 && len(e.RequestPaths) == 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)

	e := DailySummaryToExport(*c.DailySummary)
	if e.RequestStatus != (RequestStatusExport{Success: 1, Redirect: 1, Blocked: 1}) {
		t.Fatalf("unexpected status classes: %+v", e.RequestStatus)
	}
	if e.StatusCodes["200"] != 1 || e.StatusCodes["403"] != 1 || e.StatusCodes["302"] != 1 {
		t.Fatalf("unexpected status codes: %v", e.StatusCodes)
	}
	if e.ProxyRules["a.example.com"] != 3 {
		t.Fatalf("expected 3 requests on the proxy rule, got %v", e.ProxyRules)
	}
	if _, ok := e.BotClientIp["2.2.2.2"]; !ok || len(e.BotClientIp) != 1 {
		t.Fatalf("expected only 2.2.2.2 as bot, got %v", e.BotClientIp)
	}
	if e.Upstreams["10.0.0.1:8080"] != 2 || len(e.Upstreams) != 1 {
		t.Fatalf("blocked requests must not count as upstream traffic: %v", e.Upstreams)
	}
	if e.Downstreams["a.example.com"] != 3 {
		t.Fatalf("all requests should count towards the downstream hostname: %v", e.Downstreams)
	}
	if e.RequestPaths["/api/*"] != 1 || e.RequestPaths["/"] != 1 || e.RequestPaths["/login"] != 1 {
		t.Fatalf("unexpected request paths: %v", e.RequestPaths)
	}
	if e.HostPaths["a.example.com/api/*"] != 1 || e.HostPaths["a.example.com/"] != 1 || e.HostPaths["a.example.com/login"] != 1 || len(e.HostPaths) != 3 {
		t.Fatalf("unexpected hostname + path counters: %v", e.HostPaths)
	}
	if e.FileTypes["json"] != 1 || e.FileTypes["html"] != 1 || len(e.FileTypes) != 2 {
		t.Fatalf("unexpected file types: %v", e.FileTypes)
	}
	if e.Latency.Count != 2 {
		t.Fatalf("expected 2 latency observations, got %d", e.Latency.Count)
	}
	hour := time.Now().Hour()
	if len(e.Hourly) != 24 || e.Hourly[hour].Total != 3 || e.Hourly[hour].Blocked != 1 || e.Hourly[hour].Latency.Count != 2 {
		t.Fatalf("unexpected hourly data for hour %d: %+v", hour, e.Hourly)
	}

	//Round trip through the storage format
	restored := DailySummaryExportToSummary(e)
	again := DailySummaryToExport(restored)
	if again.RequestStatus != e.RequestStatus || again.Latency.Count != e.Latency.Count || again.Hourly[hour].Total != 3 {
		t.Fatalf("dashboard statistics lost in export round trip")
	}

	//Summaries saved before the dashboard fields existed must load without panics
	legacy := DailySummaryExportToSummary(DailySummaryExport{TotalRequest: 10})
	c.DailySummary = &legacy
	record(RequestInfo{IpAddr: "3.3.3.3", Succ: true, StatusCode: 200, RequestURL: "b.example.com/", UpstreamLatency: time.Millisecond})
	time.Sleep(100 * time.Millisecond)
	if got := DailySummaryToExport(*c.DailySummary).RequestStatus.Success; got != 1 {
		t.Fatalf("expected a legacy summary to keep recording, got %d successes", got)
	}
}
