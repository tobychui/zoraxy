package logviewer

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- Test data: representative log lines ---

const logLineOK = "[2026-07-28 14:30:00.123456] [router:proxy] [origin:example.com] [client: 192.168.1.100] [useragent: Mozilla/5.0] GET /api/data?q=1 200"
const logLineError = "[2026-07-28 14:30:01.654321] [router:proxy] [origin:example.com] [client: 10.0.0.1] [useragent: curl/7.88] POST /login 403"
const logLineEmpty = ""
const logLineNoRouter = "[2026-07-28 12:00:00.000000] [system:startup] [origin:localhost] Service started"

// parseLogLine is a method on *Viewer.
// The tests below validate both filterAndSortEntries and parseLogLine behavior.
// --- filterAndSortEntries tests ---

func makeEntries() []*LogEntry {
	return []*LogEntry{
		{Timestamp: "2026-07-28 14:30:00.123456", RouterType: "proxy", Origin: "example.com", ClientIP: "192.168.1.100", UserAgent: "Mozilla/5.0", Method: "GET", Path: "/api/data", StatusCode: 200},
		{Timestamp: "2026-07-28 14:30:01.654321", RouterType: "proxy", Origin: "example.com", ClientIP: "10.0.0.1", UserAgent: "curl/7.88", Method: "POST", Path: "/login", StatusCode: 403},
		{Timestamp: "2026-07-28 14:30:02.000000", RouterType: "proxy", Origin: "other.org", ClientIP: "192.168.1.100", UserAgent: "Mozilla/5.0", Method: "GET", Path: "/home", StatusCode: 200},
		{Timestamp: "2026-07-28 14:30:03.111111", RouterType: "redirect", Origin: "other.org", ClientIP: "172.16.0.1", UserAgent: "python/3.11", Method: "DELETE", Path: "/admin", StatusCode: 500},
		{Timestamp: "2026-07-28 14:30:04.222222", RouterType: "proxy", Origin: "example.com", ClientIP: "10.0.0.2", UserAgent: "wget/1.21", Method: "HEAD", Path: "/health", StatusCode: 301},
		{Timestamp: "2026-07-28 14:30:05.333333", RouterType: "vdir", Origin: "app.example.com", ClientIP: "192.168.2.50", UserAgent: "Mozilla/5.0", Method: "GET", Path: "/api/users", StatusCode: 200},
	}
}

func defaultParams() FilterParams {
	return FilterParams{Page: 1, PageSize: 50, SortField: "timestamp", SortOrder: "asc"}
}

func TestFilterAndSortEntries_NoFilterReturnsAll(t *testing.T) {
	entries := makeEntries()
	params := defaultParams()
	result, total := filterAndSortEntries(entries, params)
	if total != 6 {
		t.Fatalf("Expected total 6, got %d", total)
	}
	if len(result) != 6 {
		t.Fatalf("Expected 6 entries on page, got %d", len(result))
	}
}

func TestFilterAndSortEntries_FilterByIP(t *testing.T) {
	entries := makeEntries()
	params := defaultParams()
	params.FilterIP = "192.168"
	result, total := filterAndSortEntries(entries, params)
	if total != 3 {
		t.Fatalf("Expected 3 entries matching IP 192.168, got %d", total)
	}
	for _, e := range result {
		if e.ClientIP != "192.168.1.100" && e.ClientIP != "192.168.2.50" {
			t.Fatalf("Unexpected IP in result: %s", e.ClientIP)
		}
	}
}

func TestFilterAndSortEntries_FilterByStatus(t *testing.T) {
	entries := makeEntries()
	params := defaultParams()
	params.FilterStatus = "200"
	result, total := filterAndSortEntries(entries, params)
	if total != 3 {
		t.Fatalf("Expected 3 entries with status 200, got %d", total)
	}
	for _, e := range result {
		if e.StatusCode != 200 {
			t.Fatalf("Expected status 200, got %d", e.StatusCode)
		}
	}
}

func TestFilterAndSortEntries_FilterByMethod(t *testing.T) {
	entries := makeEntries()
	params := defaultParams()
	params.FilterMethod = "POST"
	result, total := filterAndSortEntries(entries, params)
	if total != 1 {
		t.Fatalf("Expected 1 POST entry, got %d", total)
	}
	if result[0].Method != "POST" {
		t.Fatalf("Expected method POST, got %s", result[0].Method)
	}
}

func TestFilterAndSortEntries_FilterByPath(t *testing.T) {
	entries := makeEntries()
	params := defaultParams()
	params.FilterPath = "/api"
	_, total := filterAndSortEntries(entries, params)
	if total != 2 {
		t.Fatalf("Expected 2 entries with path containing /api, got %d", total)
	}
}

func TestFilterAndSortEntries_FilterByOrigin(t *testing.T) {
	entries := makeEntries()
	params := defaultParams()
	params.FilterOrigin = "other"
	_, total := filterAndSortEntries(entries, params)
	if total != 2 {
		t.Fatalf("Expected 2 entries from other.org, got %d", total)
	}
}

func TestFilterAndSortEntries_FilterByTimeRange(t *testing.T) {
	entries := makeEntries()
	params := defaultParams()
	params.TimeStart = "2026-07-28 14:30:02"
	params.TimeEnd = "2026-07-28 14:30:04"
	_, total := filterAndSortEntries(entries, params)
	// The implementation normalizes a second-granularity TimeEnd to "...59.999999"
	// so the interval is closed at the end second: 14:30:02, 14:30:03, 14:30:04.
	if total != 3 {
		t.Fatalf("Expected 3 entries in time range, got %d", total)
	}
}

func TestFilterAndSortEntries_CombinedFilters(t *testing.T) {
	entries := makeEntries()
	params := defaultParams()
	params.FilterStatus = "200"
	params.FilterOrigin = "example.com"
	_, total := filterAndSortEntries(entries, params)
	if total != 2 {
		t.Fatalf("Expected 2 entries (example.com + 200), got %d", total)
	}
}

func TestFilterAndSortEntries_SortByStatusCodeAsc(t *testing.T) {
	entries := makeEntries()
	params := defaultParams()
	params.SortField = "status_code"
	params.SortOrder = "asc"
	result, _ := filterAndSortEntries(entries, params)
	for i := 1; i < len(result); i++ {
		if result[i].StatusCode < result[i-1].StatusCode {
			t.Fatalf("Sort order broken at index %d: %d < %d", i, result[i].StatusCode, result[i-1].StatusCode)
		}
	}
}

func TestFilterAndSortEntries_SortByStatusCodeDesc(t *testing.T) {
	entries := makeEntries()
	params := defaultParams()
	params.SortField = "status_code"
	params.SortOrder = "desc"
	result, _ := filterAndSortEntries(entries, params)
	for i := 1; i < len(result); i++ {
		if result[i].StatusCode > result[i-1].StatusCode {
			t.Fatalf("Sort order broken at index %d: %d > %d", i, result[i].StatusCode, result[i-1].StatusCode)
		}
	}
}

func TestFilterAndSortEntries_SortByClientIP(t *testing.T) {
	entries := makeEntries()
	params := defaultParams()
	params.SortField = "client_ip"
	params.SortOrder = "asc"
	result, _ := filterAndSortEntries(entries, params)
	for i := 1; i < len(result); i++ {
		if result[i].ClientIP < result[i-1].ClientIP {
			t.Fatalf("Sort order broken at index %d: %s < %s", i, result[i].ClientIP, result[i-1].ClientIP)
		}
	}
}

func TestFilterAndSortEntries_SortByPath(t *testing.T) {
	entries := makeEntries()
	params := defaultParams()
	params.SortField = "path"
	params.SortOrder = "asc"
	result, _ := filterAndSortEntries(entries, params)
	for i := 1; i < len(result); i++ {
		if result[i].Path < result[i-1].Path {
			t.Fatalf("Sort order broken at index %d: %s < %s", i, result[i].Path, result[i-1].Path)
		}
	}
}

func TestFilterAndSortEntries_PaginationFirstPage(t *testing.T) {
	entries := makeEntries()
	params := defaultParams()
	params.PageSize = 2
	params.Page = 1
	result, total := filterAndSortEntries(entries, params)
	if total != 6 {
		t.Fatalf("Expected total 6, got %d", total)
	}
	if len(result) != 2 {
		t.Fatalf("Expected 2 entries on page 1, got %d", len(result))
	}
}

func TestFilterAndSortEntries_PaginationLastPage(t *testing.T) {
	entries := makeEntries()
	params := defaultParams()
	params.PageSize = 2
	params.Page = 3
	result, total := filterAndSortEntries(entries, params)
	if total != 6 {
		t.Fatalf("Expected total 6, got %d", total)
	}
	if len(result) != 2 {
		t.Fatalf("Expected 2 entries on page 3, got %d", len(result))
	}
}

func TestFilterAndSortEntries_PaginationBeyondRange(t *testing.T) {
	entries := makeEntries()
	params := defaultParams()
	params.PageSize = 2
	params.Page = 10 // way beyond range
	result, total := filterAndSortEntries(entries, params)
	if total != 6 {
		t.Fatalf("Expected total 6, got %d", total)
	}
	if len(result) != 0 {
		t.Fatalf("Expected 0 entries on out-of-range page, got %d", len(result))
	}
}

func TestFilterAndSortEntries_EmptyInput(t *testing.T) {
	entries := []*LogEntry{}
	params := defaultParams()
	result, total := filterAndSortEntries(entries, params)
	if total != 0 {
		t.Fatalf("Expected total 0, got %d", total)
	}
	if len(result) != 0 {
		t.Fatalf("Expected 0 entries, got %d", len(result))
	}
}

func TestFilterAndSortEntries_NoMatch(t *testing.T) {
	entries := makeEntries()
	params := defaultParams()
	params.FilterIP = "255.255.255.255"
	result, total := filterAndSortEntries(entries, params)
	if total != 0 {
		t.Fatalf("Expected total 0 for non-matching filter, got %d", total)
	}
	if len(result) != 0 {
		t.Fatalf("Expected 0 entries, got %d", len(result))
	}
}

// --- Time range half-open interval tests ---

func TestFilterAndSortEntries_TimeRangeStartOnly(t *testing.T) {
	entries := makeEntries()
	params := defaultParams()
	params.TimeStart = "2026-07-28 14:30:03"
	// No TimeEnd set — should include everything from 14:30:03 onward
	_, total := filterAndSortEntries(entries, params)
	if total != 3 {
		t.Fatalf("Expected 3 entries from 14:30:03 onward, got %d", total)
	}
}

func TestFilterAndSortEntries_TimeRangeEndOnly(t *testing.T) {
	entries := makeEntries()
	params := defaultParams()
	params.TimeEnd = "2026-07-28 14:30:02"
	// No TimeStart set — TimeEnd is normalized to "...02.999999" so everything
	// up to and including 14:30:02 is kept: 14:30:00, 14:30:01, 14:30:02.
	_, total := filterAndSortEntries(entries, params)
	if total != 3 {
		t.Fatalf("Expected 3 entries up to 14:30:02 (inclusive end second), got %d", total)
	}
}

// --- Invalid sort field / sort order tests ---

func TestFilterAndSortEntries_InvalidSortField(t *testing.T) {
	entries := makeEntries()
	params := defaultParams()
	params.SortField = "nonexistent"
	// Should fall back to default timestamp sort without panicking
	result, total := filterAndSortEntries(entries, params)
	if total != 6 {
		t.Fatalf("Expected total 6 with invalid sort field, got %d", total)
	}
	if len(result) != 6 {
		t.Fatalf("Expected 6 entries with invalid sort field, got %d", len(result))
	}
}

func TestFilterAndSortEntries_InvalidSortOrder(t *testing.T) {
	entries := makeEntries()
	params := defaultParams()
	params.SortOrder = "invalid"
	// SortOrder only checked against "desc"; anything else defaults to ascending
	result, _ := filterAndSortEntries(entries, params)
	// Verify ascending order (first entry should have the earliest timestamp)
	if len(result) > 1 && result[0].Timestamp > result[1].Timestamp {
		t.Fatalf("Expected ascending order for invalid sortOrder, got descending")
	}
}

// --- Zero / negative pagination tests ---

func TestFilterAndSortEntries_ZeroPageSize(t *testing.T) {
	entries := makeEntries()
	params := defaultParams()
	params.PageSize = 0
	// PageSize=0 should be handled gracefully — filterAndSortEntries does not
	// validate PageSize; the caller (HandleReadLogEntries) enforces min 1.
	// Here we test the raw function behavior: 0 PageSize means (page-1)*0 = 0 start,
	// end = 0 + 0 = 0, so it returns an empty slice.
	result, total := filterAndSortEntries(entries, params)
	if total != 6 {
		t.Fatalf("Expected total 6, got %d", total)
	}
	if len(result) != 0 {
		t.Fatalf("Expected 0 entries for PageSize=0, got %d", len(result))
	}
}

func TestFilterAndSortEntries_NegativePage(t *testing.T) {
	entries := makeEntries()
	params := defaultParams()
	params.PageSize = 2
	params.Page = -1
	// Page < 1 is clamped to 1 inside filterAndSortEntries
	result, total := filterAndSortEntries(entries, params)
	if total != 6 {
		t.Fatalf("Expected total 6, got %d", total)
	}
	if len(result) != 2 {
		t.Fatalf("Expected 2 entries for page=-1 (clamped to 1), got %d", len(result))
	}
}

// --- parseLogLine unit tests ---

func newTestViewer() *Viewer {
	return &Viewer{option: &ViewerOption{RootFolder: "/tmp"}}
}

func TestParseLogLine_NormalLine(t *testing.T) {
	v := newTestViewer()
	entry, err := v.parseLogLine("[2026-07-28 14:30:00.123456] [router:proxy] [origin:example.com] [client: 192.168.1.100] [useragent: Mozilla/5.0] GET /api/data?q=1 200")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if entry.Timestamp != "2026-07-28 14:30:00.123456" {
		t.Fatalf("Expected timestamp, got '%s'", entry.Timestamp)
	}
	if entry.RouterType != "proxy" {
		t.Fatalf("Expected router_type 'proxy', got '%s'", entry.RouterType)
	}
	if entry.Origin != "example.com" {
		t.Fatalf("Expected origin 'example.com', got '%s'", entry.Origin)
	}
	if entry.ClientIP != "192.168.1.100" {
		t.Fatalf("Expected client_ip '192.168.1.100', got '%s'", entry.ClientIP)
	}
	if entry.UserAgent != "Mozilla/5.0" {
		t.Fatalf("Expected user_agent 'Mozilla/5.0', got '%s'", entry.UserAgent)
	}
	if entry.Method != "GET" {
		t.Fatalf("Expected method 'GET', got '%s'", entry.Method)
	}
	if entry.Path != "/api/data?q=1" {
		t.Fatalf("Expected path '/api/data?q=1', got '%s'", entry.Path)
	}
	if entry.StatusCode != 200 {
		t.Fatalf("Expected status_code 200, got %d", entry.StatusCode)
	}
}

func TestParseLogLine_ErrorStatusCode(t *testing.T) {
	v := newTestViewer()
	entry, err := v.parseLogLine("[2026-07-28 14:30:01.654321] [router:proxy] [origin:example.com] [client: 10.0.0.1] [useragent: curl/7.88] POST /login 403")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if entry.StatusCode != 403 {
		t.Fatalf("Expected status_code 403, got %d", entry.StatusCode)
	}
	if entry.Method != "POST" {
		t.Fatalf("Expected method 'POST', got '%s'", entry.Method)
	}
}

func TestParseLogLine_EmptyLine(t *testing.T) {
	v := newTestViewer()
	_, err := v.parseLogLine("")
	if err == nil {
		t.Fatalf("Expected error for empty line, got nil")
	}
}

func TestParseLogLine_WhitespaceOnly(t *testing.T) {
	v := newTestViewer()
	_, err := v.parseLogLine("   ")
	if err == nil {
		t.Fatalf("Expected error for whitespace-only line, got nil")
	}
}

func TestParseLogLine_UserAgentWithSpaces(t *testing.T) {
	v := newTestViewer()
	entry, err := v.parseLogLine("[2026-07-28 14:30:02.000000] [router:proxy] [origin:example.com] [client: 192.168.1.1] [useragent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36] GET /home 200")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	expectedUA := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
	if entry.UserAgent != expectedUA {
		t.Fatalf("Expected full UA, got '%s'", entry.UserAgent)
	}
}

func TestParseLogLine_MissingStatusCode(t *testing.T) {
	v := newTestViewer()
	entry, err := v.parseLogLine("[2026-07-28 14:30:03.111111] [router:redirect] [origin:other.org] [client: 172.16.0.1] [useragent: python/3.11] DELETE /admin")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if entry.StatusCode != 0 {
		t.Fatalf("Expected status_code 0 for missing status, got %d", entry.StatusCode)
	}
	if entry.Method != "DELETE" {
		t.Fatalf("Expected method 'DELETE', got '%s'", entry.Method)
	}
}

func TestParseLogLine_NonRouterLine(t *testing.T) {
	v := newTestViewer()
	entry, err := v.parseLogLine("[2026-07-28 12:00:00.000000] [system:startup] [origin:localhost] Service started")
	if err != nil {
		t.Fatalf("Unexpected error for non-router line: %v", err)
	}
	// Should still parse timestamp and origin, but no method / status
	if entry.Timestamp != "2026-07-28 12:00:00.000000" {
		t.Fatalf("Expected timestamp, got '%s'", entry.Timestamp)
	}
	if entry.Origin != "localhost" {
		t.Fatalf("Expected origin 'localhost', got '%s'", entry.Origin)
	}
	if entry.Method != "" {
		t.Fatalf("Expected empty method for non-router line, got '%s'", entry.Method)
	}
}

func TestParseLogLine_IPv6Client(t *testing.T) {
	v := newTestViewer()
	entry, err := v.parseLogLine("[2026-07-28 15:00:00.000000] [router:proxy] [origin:example.com] [client: 2001:db8::1] [useragent: curl/7.88] GET / 200")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if entry.ClientIP != "2001:db8::1" {
		t.Fatalf("Expected IPv6 client_ip, got '%s'", entry.ClientIP)
	}
}

func TestParseLogLine_MinimalValidLine(t *testing.T) {
	v := newTestViewer()
	entry, err := v.parseLogLine("[2026-07-28 00:00:00.000000] [router:proxy] [origin:-] [client: 0.0.0.0] [useragent: -] GET / 200")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if entry.Method != "GET" || entry.Path != "/" || entry.StatusCode != 200 {
		t.Fatalf("Minimal line parsed incorrectly: method=%s path=%s status=%d", entry.Method, entry.Path, entry.StatusCode)
	}
}

// --- matchesFilter tests ---

func TestMatchesFilter_NoFilters(t *testing.T) {
	entry := &LogEntry{Timestamp: "2026-07-28 14:30:00.123456", ClientIP: "192.168.1.100", Method: "GET", Path: "/api", Origin: "example.com", StatusCode: 200}
	if !matchesFilter(entry, FilterParams{}) {
		t.Fatalf("Expected no-filter match, got false")
	}
}

func TestMatchesFilter_ByIP(t *testing.T) {
	entry := &LogEntry{ClientIP: "192.168.1.100"}
	if !matchesFilter(entry, FilterParams{FilterIP: "192.168"}) {
		t.Fatalf("Expected IP match, got false")
	}
	if matchesFilter(entry, FilterParams{FilterIP: "10.0.0"}) {
		t.Fatalf("Expected IP mismatch, got true")
	}
}

func TestMatchesFilter_ByStatus(t *testing.T) {
	entry := &LogEntry{StatusCode: 403}
	if !matchesFilter(entry, FilterParams{FilterStatus: "403"}) {
		t.Fatalf("Expected status match, got false")
	}
	if matchesFilter(entry, FilterParams{FilterStatus: "200"}) {
		t.Fatalf("Expected status mismatch, got true")
	}
}

func TestMatchesFilter_ByMethodCaseInsensitive(t *testing.T) {
	entry := &LogEntry{Method: "get"}
	if !matchesFilter(entry, FilterParams{FilterMethod: "GET"}) {
		t.Fatalf("Expected method match (case-insensitive), got false")
	}
}

func TestMatchesFilter_TimeEndInclusive(t *testing.T) {
	entry := &LogEntry{Timestamp: "2026-07-28 14:30:04.000000"}
	if !matchesFilter(entry, FilterParams{TimeEnd: "2026-07-28 14:30:04"}) {
		t.Fatalf("Expected entry at the end second to be included, got false")
	}
	entryAfter := &LogEntry{Timestamp: "2026-07-28 14:30:05.000000"}
	if matchesFilter(entryAfter, FilterParams{TimeEnd: "2026-07-28 14:30:04"}) {
		t.Fatalf("Expected entry after the end second to be excluded, got true")
	}
}

// --- forEachLogLine streaming tests ---

func TestForEachLogLine_ReadsAllLines(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "access.log")
	lines := []string{"line one", "line two", "line three"}
	if err := os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("Failed to write temp log: %v", err)
	}

	v := newTestViewer()
	got := []string{}
	err := v.forEachLogLine(logPath, func(line string) bool {
		got = append(got, line)
		return true
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(got) != len(lines) {
		t.Fatalf("Expected %d lines, got %d", len(lines), len(got))
	}
	for i := range lines {
		if got[i] != lines[i] {
			t.Fatalf("Line %d mismatch: got %q, want %q", i, got[i], lines[i])
		}
	}
}

func TestForEachLogLine_EarlyStop(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "access.log")
	if err := os.WriteFile(logPath, []byte("a\nb\nc\nd\n"), 0644); err != nil {
		t.Fatalf("Failed to write temp log: %v", err)
	}

	v := newTestViewer()
	count := 0
	err := v.forEachLogLine(logPath, func(line string) bool {
		count++
		return count < 2 // stop after the second line
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if count != 2 {
		t.Fatalf("Expected early stop after 2 lines, got %d", count)
	}
}

func TestForEachLogLine_Gzip(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "access.log.gz")
	f, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("Failed to create temp log: %v", err)
	}
	gz := gzip.NewWriter(f)
	if _, err := gz.Write([]byte("compressed line 1\ncompressed line 2\n")); err != nil {
		t.Fatalf("Failed to write gzip content: %v", err)
	}
	gz.Close()
	f.Close()

	v := newTestViewer()
	got := []string{}
	err = v.forEachLogLine(logPath, func(line string) bool {
		got = append(got, line)
		return true
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(got) != 2 || got[0] != "compressed line 1" || got[1] != "compressed line 2" {
		t.Fatalf("Unexpected gzip lines: %v", got)
	}
}

func TestForEachLogLine_MissingFile(t *testing.T) {
	v := newTestViewer()
	err := v.forEachLogLine("/nonexistent/path/access.log", func(line string) bool { return true })
	if err == nil {
		t.Fatalf("Expected error for missing file, got nil")
	}
}
