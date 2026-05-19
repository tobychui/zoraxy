package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	plugin "example.com/zoraxy/prometheus-exporter/mod/zoraxy_plugin"
)

const (
	PLUGIN_ID    = "com.example.zoraxy.prometheus-exporter"
	UI_PATH      = "/ui"
	METRICS_PATH = "/metrics"
)

// DailySummaryExport mirrors the structure returned by /api/stats/summary.
// High-cardinality fields (RequestClientIp, Referer, UserAgent, RequestURL)
// are intentionally omitted to avoid label cardinality explosion in Prometheus.
type DailySummaryExport struct {
	TotalRequest  int64          `json:"TotalRequest"`
	ErrorRequest  int64          `json:"ErrorRequest"`
	ValidRequest  int64          `json:"ValidRequest"`
	ForwardTypes  map[string]int `json:"ForwardTypes"`
	RequestOrigin map[string]int `json:"RequestOrigin"`
	Downstreams   map[string]int `json:"Downstreams"`
	Upstreams     map[string]int `json:"Upstreams"`
}

type NetStat struct {
	RX int64 `json:"RX"`
	TX int64 `json:"TX"`
}

type metricsState struct {
	mu         sync.RWMutex
	summary    *DailySummaryExport
	netstat    *NetStat
	lastUpdate time.Time
	lastError  string
}

var (
	state      metricsState
	runtimeCfg *plugin.ConfigureSpec
)

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func escapeLabel(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}

func fetchStats() {
	client := &http.Client{Timeout: 10 * time.Second}

	doGet := func(apiPath string) ([]byte, error) {
		url := fmt.Sprintf("http://127.0.0.1:%d/plugin%s", runtimeCfg.ZoraxyPort, apiPath)
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+runtimeCfg.APIKey)
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		return io.ReadAll(resp.Body)
	}

	summaryBody, err := doGet("/api/stats/summary")
	if err != nil {
		state.mu.Lock()
		state.lastError = "summary fetch: " + err.Error()
		state.mu.Unlock()
		return
	}

	var summary DailySummaryExport
	if err := json.Unmarshal(summaryBody, &summary); err != nil {
		state.mu.Lock()
		state.lastError = "summary parse: " + err.Error()
		state.mu.Unlock()
		return
	}

	var ns *NetStat
	if netBody, err := doGet("/api/stats/netstat"); err == nil {
		var tmp NetStat
		if json.Unmarshal(netBody, &tmp) == nil {
			ns = &tmp
		}
	}

	state.mu.Lock()
	state.summary = &summary
	state.netstat = ns
	state.lastUpdate = time.Now()
	state.lastError = ""
	state.mu.Unlock()
}

func startPoller() {
	go func() {
		fetchStats()
		ticker := time.NewTicker(30 * time.Second)
		for range ticker.C {
			fetchStats()
		}
	}()
}

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	state.mu.RLock()
	defer state.mu.RUnlock()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	if state.summary == nil {
		if state.lastError != "" {
			fmt.Fprintf(w, "# ERROR %s\n", state.lastError)
		} else {
			fmt.Fprintln(w, "# Waiting for first data fetch...")
		}
		return
	}

	s := state.summary

	writeGauge := func(name, help string, value int64) {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n%s %d\n", name, help, name, name, value)
	}

	writeLabeledGauge := func(name, help, labelKey string, m map[string]int) {
		if len(m) == 0 {
			return
		}
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n", name, help, name)
		for _, k := range sortedKeys(m) {
			fmt.Fprintf(w, "%s{%s=\"%s\"} %d\n", name, labelKey, escapeLabel(k), m[k])
		}
	}

	writeGauge("zoraxy_requests_today_total", "Total requests proxied today", s.TotalRequest)
	writeGauge("zoraxy_requests_today_valid", "Valid requests today", s.ValidRequest)
	writeGauge("zoraxy_requests_today_error", "Error requests today", s.ErrorRequest)

	writeLabeledGauge("zoraxy_requests_by_forward_type", "Requests by forward type today", "type", s.ForwardTypes)
	writeLabeledGauge("zoraxy_requests_by_country", "Requests by origin country ISO code today", "country", s.RequestOrigin)
	writeLabeledGauge("zoraxy_requests_by_downstream", "Requests by downstream hostname today", "hostname", s.Downstreams)
	writeLabeledGauge("zoraxy_requests_by_upstream", "Requests by upstream hostname today", "hostname", s.Upstreams)

	if state.netstat != nil {
		fmt.Fprintln(w, "# HELP zoraxy_network_rx_bits_total Accumulated received bits across all network interfaces")
		fmt.Fprintln(w, "# TYPE zoraxy_network_rx_bits_total counter")
		fmt.Fprintf(w, "zoraxy_network_rx_bits_total %d\n", state.netstat.RX)
		fmt.Fprintln(w, "# HELP zoraxy_network_tx_bits_total Accumulated transmitted bits across all network interfaces")
		fmt.Fprintln(w, "# TYPE zoraxy_network_tx_bits_total counter")
		fmt.Fprintf(w, "zoraxy_network_tx_bits_total %d\n", state.netstat.TX)
	}

	fmt.Fprintln(w, "# HELP zoraxy_stats_last_update_unix Unix timestamp of last successful stats fetch")
	fmt.Fprintln(w, "# TYPE zoraxy_stats_last_update_unix gauge")
	fmt.Fprintf(w, "zoraxy_stats_last_update_unix %d\n", state.lastUpdate.Unix())
}

func handleUI(metricsPort int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		state.mu.RLock()
		lastUpdate := state.lastUpdate
		lastError := state.lastError
		var total, valid, errs int64
		if state.summary != nil {
			total = state.summary.TotalRequest
			valid = state.summary.ValidRequest
			errs = state.summary.ErrorRequest
		}
		state.mu.RUnlock()

		var statusDiv string
		switch {
		case lastError != "":
			statusDiv = `<div class="ui red message"><i class="warning icon"></i>Fetch error: ` + lastError + `</div>`
		case !lastUpdate.IsZero():
			statusDiv = `<div class="ui green message"><i class="check icon"></i>Last updated: ` + lastUpdate.Format("2006-01-02 15:04:05") + `</div>`
		default:
			statusDiv = `<div class="ui yellow message"><i class="clock icon"></i>Waiting for first fetch...</div>`
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
<meta charset="UTF-8">
<title>Prometheus Exporter</title>
<link rel="stylesheet" href="/script/semantic/semantic.min.css">
<link rel="stylesheet" href="/main.css">
<style>body{background:none}</style>
</head>
<body>
<link rel="stylesheet" href="/darktheme.css">
<script src="/script/darktheme.js"></script>
<div class="ui container" style="padding-top:1.5em">
<h2 class="ui header">Prometheus Exporter</h2>
<p>Exposes Zoraxy statistical analysis data as Prometheus metrics.</p>
%s
<div class="ui segment">
<h4>Metrics Endpoint</h4>
<code>http://&lt;host&gt;:%d/metrics</code>
<p style="margin-top:.5em;color:gray;font-size:.9em">Configure Prometheus to scrape this endpoint. Stats are fetched from Zoraxy every 30 seconds.</p>
</div>
<div class="ui segment">
<h4>Today's Requests</h4>
<div class="ui three statistics">
<div class="statistic"><div class="value">%d</div><div class="label">Total</div></div>
<div class="statistic"><div class="value">%d</div><div class="label">Valid</div></div>
<div class="statistic"><div class="value">%d</div><div class="label">Errors</div></div>
</div>
</div>
</div>
</body>
</html>`, statusDiv, metricsPort, total, valid, errs)
	}
}

// parseMetricsPort reads -metrics-port=N from os.Args without using the flag
// package, which would conflict with Zoraxy's own -configure and -introspect flags.
func parseMetricsPort() int {
	for _, arg := range os.Args[1:] {
		for _, prefix := range []string{"-metrics-port=", "--metrics-port="} {
			if strings.HasPrefix(arg, prefix) {
				if p, err := strconv.Atoi(arg[len(prefix):]); err == nil && p > 0 && p < 65536 {
					return p
				}
			}
		}
	}
	return 9100
}

func main() {
	metricsPort := parseMetricsPort()

	cfg, err := plugin.ServeAndRecvSpec(&plugin.IntroSpect{
		ID:            PLUGIN_ID,
		Name:          "Prometheus Exporter",
		Author:        "Zoraxy",
		AuthorContact: "",
		Description:   "Exports Zoraxy statistical analysis data as Prometheus metrics",
		Type:          plugin.PluginType_Utilities,
		VersionMajor:  1,
		VersionMinor:  0,
		VersionPatch:  0,
		UIPath:        UI_PATH,
		PermittedAPIEndpoints: []plugin.PermittedAPIEndpoint{
			{
				Method:   http.MethodGet,
				Endpoint: "/plugin/api/stats/summary",
				Reason:   "Fetch daily request statistics for Prometheus export",
			},
			{
				Method:   http.MethodGet,
				Endpoint: "/plugin/api/stats/netstat",
				Reason:   "Fetch network interface statistics for Prometheus export",
			},
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "startup error:", err)
		os.Exit(1)
	}
	runtimeCfg = cfg

	startPoller()

	// Dedicated metrics server on all interfaces for Prometheus scraping.
	metricsMux := http.NewServeMux()
	metricsMux.HandleFunc(METRICS_PATH, handleMetrics)
	go func() {
		addr := ":" + strconv.Itoa(metricsPort)
		fmt.Println("Metrics server listening on", addr)
		if err := http.ListenAndServe(addr, metricsMux); err != nil {
			fmt.Fprintln(os.Stderr, "metrics server:", err)
		}
	}()

	// Plugin server on localhost only for Zoraxy UI integration.
	mux := http.NewServeMux()
	mux.HandleFunc(UI_PATH+"/", handleUI(metricsPort))
	mux.HandleFunc(METRICS_PATH, handleMetrics)
	mux.HandleFunc(UI_PATH+"/term", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		go func() {
			time.Sleep(100 * time.Millisecond)
			os.Exit(0)
		}()
	})

	pluginAddr := "127.0.0.1:" + strconv.Itoa(cfg.Port)
	fmt.Println("Plugin server listening on", pluginAddr)
	if err := http.ListenAndServe(pluginAddr, mux); err != nil {
		fmt.Fprintln(os.Stderr, "plugin server:", err)
		os.Exit(1)
	}
}
