package dynamicproxy

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/database/dbinc"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/plugins"
	"imuslab.com/zoraxy/mod/statistic"
)

// A request a plugin denied shows up in the access log with status and
// reason, and the route's logging switch still applies.
func TestLogPluginDenialRecordsReasonAndHonoursLoggingSwitch(t *testing.T) {
	l, err := logger.NewLogger("router", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	router := &Router{Option: &RouterOption{Logger: l}}

	denied := httptest.NewRequest("POST", "/login", nil)
	denied.Host = "chat.example.test"
	quiet := httptest.NewRequest("GET", "/secret", nil)
	quiet.Host = "quiet.example.test"
	router.logPluginDenial(denied, &plugins.Denial{Status: 403, Reason: "waf-942100"}, "chat.example.test", &ProxyEndpoint{})
	router.logPluginDenial(quiet, &plugins.Denial{Status: 429}, "quiet.example.test", &ProxyEndpoint{DisableLogging: true})

	// LogHTTPRequest writes asynchronously.
	var got string
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		b, err := os.ReadFile(l.CurrentLogFile)
		if err != nil {
			t.Fatal(err)
		}
		if got = string(b); strings.Contains(got, "POST /login 403") {
			break
		}
	}
	if !strings.Contains(got, "[router:plugin-deny:waf-942100]") || !strings.Contains(got, "[origin:chat.example.test]") || !strings.Contains(got, "POST /login 403") {
		t.Fatalf("denial not logged as expected:\n%s", got)
	}
	time.Sleep(100 * time.Millisecond)
	if b, _ := os.ReadFile(l.CurrentLogFile); strings.Contains(string(b), "/secret") {
		t.Fatalf("route with logging disabled was logged:\n%s", b)
	}
}

// Statistics count every denial under one forward type, whatever reason the
// plugin sends, so plugin-chosen reasons cannot grow the statistic maps.
func TestLogPluginDenialStatisticsUseOneForwardType(t *testing.T) {
	db, err := database.NewDatabase(filepath.Join(t.TempDir(), "sysdb"), dbinc.BackendLevelDB)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	collector, err := statistic.NewStatisticCollector(statistic.CollectorOption{Database: db})
	if err != nil {
		t.Fatal(err)
	}
	defer collector.Close() // runs before db.Close
	l, err := logger.NewFmtLogger()
	if err != nil {
		t.Fatal(err)
	}
	router := &Router{Option: &RouterOption{Logger: l, StatisticCollector: collector}}
	types := func() map[string]int {
		out := map[string]int{}
		collector.DailySummary.ForwardTypes.Range(func(k, v any) bool {
			out[k.(string)] = v.(int)
			return true
		})
		return out
	}
	// One denial at a time: the collector counts asynchronously and its
	// counters are not safe for concurrent increments of the same key.
	for i, reason := range []string{"waf-942100", "waf-941100", ""} {
		req := httptest.NewRequest("POST", "/login", nil)
		req.Host = "chat.example.test"
		router.logPluginDenial(req, &plugins.Denial{Status: 403, Reason: reason}, "chat.example.test", &ProxyEndpoint{})
		for deadline := time.Now().Add(3 * time.Second); types()["plugin-deny"] < i+1 && time.Now().Before(deadline); {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if got := types(); len(got) != 1 || got["plugin-deny"] != 3 {
		t.Fatalf("expected 3 denials under the single type plugin-deny, got %v", got)
	}
}
