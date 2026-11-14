package statistic

import (
	"mime"
	"path"
	"strings"
	"sync/atomic"
	"time"
)

/*
	Extended.go

	Additional per-day statistics used by the statistic dashboard:
	request status classes, upstream latency (TTFB) histograms,
	hourly buckets and the request classifiers that feed them
*/

// StatusClass is the high level outcome of a request, see ClassifyRequest
type StatusClass int

const (
	StatusClassSuccess  StatusClass = iota //1xx and 2xx responses
	StatusClassRedirect                    //3xx responses
	StatusClassBlocked                     //Rejected by Zoraxy rules (access list, rate limit, exploit / captcha, mTLS)
	StatusClassError                       //Upstream 4xx / 5xx responses and gateway failures
)

// ClassifyRequest returns the status class of a recorded request
func ClassifyRequest(ri RequestInfo) StatusClass {
	if ri.Blocked {
		return StatusClassBlocked
	}
	switch {
	case ri.StatusCode >= 100 && ri.StatusCode < 300:
		return StatusClassSuccess
	case ri.StatusCode >= 300 && ri.StatusCode < 400:
		return StatusClassRedirect
	case ri.StatusCode == 0 && ri.Succ:
		return StatusClassSuccess
	}
	return StatusClassError
}

/*
	Status class counters
*/

type statusClassCounters struct {
	success  atomic.Int64
	redirect atomic.Int64
	blocked  atomic.Int64
	errors   atomic.Int64
}

// RequestStatusExport is the serializable form of the status class counters
type RequestStatusExport struct {
	Success  int64
	Redirect int64
	Blocked  int64
	Error    int64
}

func (s *statusClassCounters) add(class StatusClass) {
	switch class {
	case StatusClassSuccess:
		s.success.Add(1)
	case StatusClassRedirect:
		s.redirect.Add(1)
	case StatusClassBlocked:
		s.blocked.Add(1)
	default:
		s.errors.Add(1)
	}
}

func (s *statusClassCounters) export() RequestStatusExport {
	return RequestStatusExport{
		Success:  s.success.Load(),
		Redirect: s.redirect.Load(),
		Blocked:  s.blocked.Load(),
		Error:    s.errors.Load(),
	}
}

func newStatusClassCountersFromExport(e RequestStatusExport) *statusClassCounters {
	s := &statusClassCounters{}
	s.success.Store(e.Success)
	s.redirect.Store(e.Redirect)
	s.blocked.Store(e.Blocked)
	s.errors.Store(e.Error)
	return s
}

// Total number of classified requests
func (e RequestStatusExport) Total() int64 {
	return e.Success + e.Redirect + e.Blocked + e.Error
}

// Merge adds the counters of another export into this one
func (e *RequestStatusExport) Merge(o RequestStatusExport) {
	e.Success += o.Success
	e.Redirect += o.Redirect
	e.Blocked += o.Blocked
	e.Error += o.Error
}

/*
	Upstream latency histogram

	Latency is the time to first byte (TTFB) of the upstream: from sending the
	request until the upstream response headers are received. Values are kept in
	a fixed size histogram so percentiles can be estimated over any time range
	while the storage size stays constant.
*/

// LatencyBucketBoundsMs is the inclusive upper bound (in milliseconds) of each
// latency histogram bucket. One extra overflow bucket holds everything above
// the last bound. This list is part of the storage format, never change or
// reorder the existing values.
var LatencyBucketBoundsMs = []float64{1, 2, 5, 10, 25, 50, 75, 100, 150, 200, 300, 500, 750, 1000, 1500, 2000, 3000, 5000, 10000, 30000}

const latencyBucketCount = 21 //len(LatencyBucketBoundsMs) + 1 overflow bucket

type latencyHistogram struct {
	buckets [latencyBucketCount]atomic.Int64
	sumUs   atomic.Int64
	count   atomic.Int64
}

// LatencyHistogramExport is the serializable form of a latency histogram
type LatencyHistogramExport struct {
	Buckets []int64 `json:",omitempty"` //Request count of each bucket, see LatencyBucketBoundsMs
	SumUs   int64   //Sum of all latencies in microseconds
	Count   int64   //Number of observed requests
}

func latencyBucketIndex(d time.Duration) int {
	ms := float64(d) / float64(time.Millisecond)
	for i, bound := range LatencyBucketBoundsMs {
		if ms <= bound {
			return i
		}
	}
	return latencyBucketCount - 1
}

func (h *latencyHistogram) observe(d time.Duration) {
	h.buckets[latencyBucketIndex(d)].Add(1)
	h.sumUs.Add(d.Microseconds())
	h.count.Add(1)
}

func (h *latencyHistogram) export() LatencyHistogramExport {
	e := LatencyHistogramExport{
		SumUs: h.sumUs.Load(),
		Count: h.count.Load(),
	}
	if e.Count == 0 {
		return e
	}
	e.Buckets = make([]int64, latencyBucketCount)
	for i := range h.buckets {
		e.Buckets[i] = h.buckets[i].Load()
	}
	return e
}

func newLatencyHistogramFromExport(e LatencyHistogramExport) *latencyHistogram {
	h := &latencyHistogram{}
	for i := 0; i < len(e.Buckets) && i < latencyBucketCount; i++ {
		h.buckets[i].Store(e.Buckets[i])
	}
	h.sumUs.Store(e.SumUs)
	h.count.Store(e.Count)
	return h
}

// Merge adds the observations of another histogram into this one
func (e *LatencyHistogramExport) Merge(o LatencyHistogramExport) {
	if o.Count == 0 {
		return
	}
	if len(e.Buckets) < latencyBucketCount {
		merged := make([]int64, latencyBucketCount)
		copy(merged, e.Buckets)
		e.Buckets = merged
	}
	for i := 0; i < len(o.Buckets) && i < latencyBucketCount; i++ {
		e.Buckets[i] += o.Buckets[i]
	}
	e.SumUs += o.SumUs
	e.Count += o.Count
}

// AverageMs returns the mean latency in milliseconds, 0 if nothing was observed
func (e LatencyHistogramExport) AverageMs() float64 {
	if e.Count == 0 {
		return 0
	}
	return float64(e.SumUs) / float64(e.Count) / 1000
}

// PercentileMs estimates the p-th percentile (0 < p <= 1) in milliseconds by
// linear interpolation inside the bucket that holds the target rank
func (e LatencyHistogramExport) PercentileMs(p float64) float64 {
	var total int64
	for _, c := range e.Buckets {
		total += c
	}
	if total == 0 {
		return 0
	}

	rank := p * float64(total)
	var cumulative float64
	for i, c := range e.Buckets {
		if c == 0 {
			continue
		}
		if cumulative+float64(c) >= rank {
			lower := 0.0
			if i > 0 {
				lower = LatencyBucketBoundsMs[i-1]
			}
			upper := lower * 2 //Overflow bucket has no upper bound
			if i < len(LatencyBucketBoundsMs) {
				upper = LatencyBucketBoundsMs[i]
			}
			return lower + (upper-lower)*(rank-cumulative)/float64(c)
		}
		cumulative += float64(c)
	}
	return LatencyBucketBoundsMs[len(LatencyBucketBoundsMs)-1]
}

/*
	Hourly buckets

	Per hour request counters of the day, used to render the trend graphs
	when a single day is selected
*/

type hourlyCounter struct {
	total   atomic.Int64
	blocked atomic.Int64
	errors  atomic.Int64
	latency latencyHistogram
}

type hourlyStats [24]hourlyCounter

// HourlyStatExport is the serializable form of one hour of the day
type HourlyStatExport struct {
	Total   int64
	Blocked int64
	Error   int64
	Latency LatencyHistogramExport
}

func (h *hourlyStats) record(hour int, class StatusClass, latency time.Duration) {
	if hour < 0 || hour >= len(h) {
		return
	}
	slot := &h[hour]
	slot.total.Add(1)
	switch class {
	case StatusClassBlocked:
		slot.blocked.Add(1)
	case StatusClassError:
		slot.errors.Add(1)
	}
	if latency > 0 {
		slot.latency.observe(latency)
	}
}

func (h *hourlyStats) export() []HourlyStatExport {
	result := make([]HourlyStatExport, len(h))
	for i := range h {
		result[i] = HourlyStatExport{
			Total:   h[i].total.Load(),
			Blocked: h[i].blocked.Load(),
			Error:   h[i].errors.Load(),
			Latency: h[i].latency.export(),
		}
	}
	return result
}

func newHourlyStatsFromExport(e []HourlyStatExport) *hourlyStats {
	h := &hourlyStats{}
	for i := 0; i < len(e) && i < len(h); i++ {
		h[i].total.Store(e[i].Total)
		h[i].blocked.Store(e[i].Blocked)
		h[i].errors.Store(e[i].Error)
		for j := 0; j < len(e[i].Latency.Buckets) && j < latencyBucketCount; j++ {
			h[i].latency.buckets[j].Store(e[i].Latency.Buckets[j])
		}
		h[i].latency.sumUs.Store(e[i].Latency.SumUs)
		h[i].latency.count.Store(e[i].Latency.Count)
	}
	return h
}

/*
	Request classifiers
*/

// requestPathOf extracts the path from a RequestInfo.RequestURL (host + escaped path)
func requestPathOf(requestURL string) string {
	idx := strings.Index(requestURL, "/")
	if idx < 0 {
		return "/"
	}
	return requestURL[idx:]
}

// requestPathPattern groups a request path by its first segment so that the
// endpoint table stays readable, e.g. /api/v1/users -> /api/*, /login -> /login
func requestPathPattern(p string) string {
	trimmed := strings.TrimPrefix(strings.TrimSpace(p), "/")
	if trimmed == "" {
		return "/"
	}
	first, _, hasMore := strings.Cut(trimmed, "/")
	if hasMore {
		return "/" + first + "/*"
	}
	return "/" + first
}

// hostPathKey joins a requested hostname and a path pattern into a HostPaths key,
// e.g. example.com/api/*. Slashes are removed from the hostname so the key can
// always be split at its first slash, see SplitHostPathKey.
func hostPathKey(host string, pathPattern string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	return strings.ReplaceAll(host, "/", "") + pathPattern
}

// SplitHostPathKey splits a HostPaths key into the requested hostname and the path pattern
func SplitHostPathKey(key string) (string, string) {
	idx := strings.Index(key, "/")
	if idx < 0 {
		return key, "/"
	}
	return key[:idx], key[idx:]
}

var fileTypeByExtension = map[string]string{
	".html": "html", ".htm": "html", ".php": "html", ".aspx": "html", ".jsp": "html",
	".js": "js", ".mjs": "js", ".jsx": "js",
	".css":  "css",
	".json": "json",
	".png":  "image", ".jpg": "image", ".jpeg": "image", ".gif": "image", ".svg": "image", ".webp": "image", ".ico": "image", ".avif": "image", ".bmp": "image",
	".woff": "font", ".woff2": "font", ".ttf": "font", ".otf": "font", ".eot": "font",
	".mp4": "video", ".webm": "video", ".mkv": "video", ".m3u8": "video", ".ts": "video", ".mov": "video",
	".mp3": "audio", ".flac": "audio", ".ogg": "audio", ".wav": "audio", ".m4a": "audio",
	".xml": "xml",
	".txt": "text", ".csv": "text", ".md": "text",
	".wasm": "wasm",
}

// classifyFileType returns a short file type label of an upstream response,
// using its Content-Type header and falling back to the request path extension
func classifyFileType(contentType string, requestPath string) string {
	mediaType := ""
	if contentType != "" {
		if parsed, _, err := mime.ParseMediaType(contentType); err == nil {
			mediaType = parsed
		} else {
			mediaType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
		}
	}

	switch {
	case mediaType == "":
		//No content type (e.g. 304 Not Modified), guess from the extension
		if ft, ok := fileTypeByExtension[strings.ToLower(path.Ext(requestPath))]; ok {
			return ft
		}
		return "unknown"
	case mediaType == "text/html" || mediaType == "application/xhtml+xml":
		return "html"
	case strings.Contains(mediaType, "javascript") || strings.Contains(mediaType, "ecmascript"):
		return "js"
	case mediaType == "text/css":
		return "css"
	case mediaType == "application/json" || strings.HasSuffix(mediaType, "+json"):
		return "json"
	case strings.HasPrefix(mediaType, "image/"):
		return "image"
	case strings.HasPrefix(mediaType, "font/") || strings.Contains(mediaType, "font-"):
		return "font"
	case strings.HasPrefix(mediaType, "video/") || mediaType == "application/vnd.apple.mpegurl" || mediaType == "application/x-mpegurl":
		return "video"
	case strings.HasPrefix(mediaType, "audio/"):
		return "audio"
	case mediaType == "text/event-stream":
		return "stream"
	case mediaType == "application/xml" || mediaType == "text/xml" || strings.HasSuffix(mediaType, "+xml"):
		return "xml"
	case strings.HasPrefix(mediaType, "text/"):
		return "text"
	case mediaType == "application/wasm":
		return "wasm"
	}
	return "other"
}

// Keywords (lower case) in the User-Agent header of crawlers, scanners and scripts
var botUserAgentKeywords = []string{
	"bot", "crawl", "spider", "slurp", "scrapy", "headless", "archiver",
	"curl/", "wget/", "python-", "python/", "go-http-client", "libwww", "httpclient", "java/", "node-fetch", "axios/",
	"zgrab", "masscan", "nmap", "nikto", "sqlmap", "censysinspect", "internetmeasurement", "nuhk", "expanse",
	"facebookexternalhit", "uptime", "monitor",
}

// isBotUserAgent returns true if the User-Agent belongs to a crawler, scanner
// or script instead of a browser or app. An empty User-Agent counts as a bot.
func isBotUserAgent(userAgent string) bool {
	ua := strings.ToLower(strings.TrimSpace(userAgent))
	if ua == "" {
		return true
	}
	for _, keyword := range botUserAgentKeywords {
		if strings.Contains(ua, keyword) {
			return true
		}
	}
	return false
}
