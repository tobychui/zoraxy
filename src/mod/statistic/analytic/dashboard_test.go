package analytic

import (
	"testing"
	"time"

	"imuslab.com/zoraxy/mod/statistic"
)

func TestClassifyReferer(t *testing.T) {
	own := newHostMatcher(map[string]int{"jellyfin.example.com:443": 1}, map[string]int{"*.home.lan": 1, "/": 1})
	cases := map[string]string{
		"":                                    "direct",
		"https://www.google.com/":             "search",
		"https://www.google.com.tw/search?q=": "search",
		"https://duckduckgo.com/":             "search",
		"https://search.brave.com/search":     "search",
		"https://jellyfin.example.com/web/":   "internal",
		"https://ha.home.lan/lovelace":        "internal",
		"https://github.com/tobychui/zoraxy":  "referral",
		"https://google.example.org/":         "referral",
		"not a url":                           "referral",
	}
	for referer, want := range cases {
		if got := classifyReferer(referer, own); got != want {
			t.Errorf("classifyReferer(%q) = %q, want %q", referer, got, want)
		}
	}

	if got := refererOrigin("https://www.Google.com/search?q=zoraxy"); got != "https://www.google.com" {
		t.Errorf("unexpected referer origin %q", got)
	}
}

func TestStatusOfLegacySummary(t *testing.T) {
	//A day recorded before status classes existed
	legacy := &statistic.DailySummaryExport{TotalRequest: 100, ValidRequest: 90, ErrorRequest: 10}
	status := statusOf(legacy)
	if status.Success != 90 || status.Error != 10 || status.Total() != 100 {
		t.Fatalf("unexpected legacy status split: %+v", status)
	}

	//The day of the upgrade: 100 legacy requests, then 20 classified ones
	mixed := &statistic.DailySummaryExport{
		TotalRequest:  120,
		ValidRequest:  105,
		ErrorRequest:  15,
		RequestStatus: statistic.RequestStatusExport{Success: 15, Blocked: 3, Error: 2},
	}
	status = statusOf(mixed)
	if status.Total() != 120 || status.Blocked != 3 || status.Error != 12 || status.Success != 105 {
		t.Fatalf("unexpected mixed status split: %+v", status)
	}
}

func TestBuildDashboardData(t *testing.T) {
	day := func(offset int) time.Time {
		return time.Date(2026, 11, 10+offset, 0, 0, 0, 0, time.Local)
	}
	hourly := make([]statistic.HourlyStatExport, 24)
	hourly[9] = statistic.HourlyStatExport{Total: 5, Blocked: 1}

	current := []dayRecord{
		{Date: day(0), Summary: &statistic.DailySummaryExport{
			TotalRequest:    10,
			RequestStatus:   statistic.RequestStatusExport{Success: 7, Redirect: 1, Blocked: 2},
			RequestClientIp: map[string]int{"1.1.1.1": 4, "2.2.2.2": 3, "2001:db8::1": 3},
			BotClientIp:     map[string]int{"2.2.2.2": 3, "1.1.1.1": 1}, //1.1.1.1 is mostly a browser
			Referer:         map[string]int{"": 6, "https://www.bing.com/": 2, "https://a.example.com/": 2},
			Downstreams:     map[string]int{"a.example.com": 10},
			ProxyRules:      map[string]int{"a.example.com": 10},
			Hourly:          hourly,
		}},
		{Date: day(1), Summary: nil}, //No data recorded on this day
	}
	previous := []dayRecord{
		{Date: day(-2), Summary: &statistic.DailySummaryExport{TotalRequest: 4, ValidRequest: 4, RequestClientIp: map[string]int{"1.1.1.1": 4}}},
		{Date: day(-1), Summary: nil},
	}

	data := buildDashboardData(current, previous, day(0), day(1), day(-2), day(-1), day(5))

	if data.Days != 2 || data.Granularity != "day" || len(data.Trend) != 2 {
		t.Fatalf("unexpected range info: days %d, granularity %s, trend %d", data.Days, data.Granularity, len(data.Trend))
	}
	if data.Trend[1].Total != 0 || data.Trend[1].AvgMs != nil {
		t.Fatalf("a day without data should be an empty trend point")
	}
	if data.Current.TotalRequest != 10 || data.Current.Blocked != 2 || data.Current.UniqueVisitors != 3 {
		t.Fatalf("unexpected current totals: %+v", data.Current)
	}
	if data.Previous.TotalRequest != 4 || data.Previous.Success != 4 || data.Previous.UniqueVisitors != 1 {
		t.Fatalf("unexpected previous totals: %+v", data.Previous)
	}
	if data.Users != (UserBreakdown{Returning: 1, New: 1, Bots: 1}) {
		t.Fatalf("unexpected user breakdown: %+v", data.Users)
	}
	if data.IPVersions != (IPVersions{IPv4: 2, IPv6: 1}) {
		t.Fatalf("unexpected ip versions: %+v", data.IPVersions)
	}
	if data.TrafficSources != (TrafficSources{Direct: 6, Search: 2, Internal: 2}) {
		t.Fatalf("unexpected traffic sources: %+v", data.TrafficSources)
	}
	if len(data.Applications.Items) != 1 || data.Applications.Items[0].Key != "a.example.com" {
		t.Fatalf("unexpected applications: %+v", data.Applications)
	}

	//A single day uses hourly granularity, today only up to the current hour
	single := buildDashboardData(current[:1], previous[:1], day(0), day(0), day(-1), day(-1), day(0).Add(10*time.Hour))
	if single.Granularity != "hour" || len(single.Trend) != 11 || single.Trend[9].Total != 5 || single.Trend[9].Blocked != 1 {
		t.Fatalf("unexpected hourly trend: %+v", single.Trend)
	}
}

func TestBuildEndpointDomainList(t *testing.T) {
	day := func(offset int) time.Time {
		return time.Date(2026, 11, 10+offset, 0, 0, 0, 0, time.Local)
	}
	records := []dayRecord{
		//Recorded before the hostname breakdown existed
		{Date: day(0), Summary: &statistic.DailySummaryExport{TotalRequest: 50, RequestPaths: map[string]int{"/api/*": 50}}},
		{Date: day(1), Summary: nil},
		{Date: day(2), Summary: &statistic.DailySummaryExport{TotalRequest: 9, HostPaths: map[string]int{"a.example.com/api/*": 5, "b.example.com/api/*": 3, "/": 1}}},
		{Date: day(3), Summary: &statistic.DailySummaryExport{TotalRequest: 2, HostPaths: map[string]int{"a.example.com/api/*": 2}}},
	}

	list := buildEndpointDomainList(records, 2)
	if list.MissingDays != 1 || list.Total != 11 || list.Distinct != 3 || len(list.Items) != 2 {
		t.Fatalf("unexpected endpoint domain list: %+v", list)
	}
	if list.Items[0] != (EndpointDomainItem{Host: "a.example.com", Path: "/api/*", Count: 7}) {
		t.Fatalf("unexpected top item: %+v", list.Items[0])
	}
	if list.Items[1] != (EndpointDomainItem{Host: "b.example.com", Path: "/api/*", Count: 3}) {
		t.Fatalf("unexpected second item: %+v", list.Items[1])
	}

	//No data at all still returns an empty list for the UI
	empty := buildEndpointDomainList(nil, 10)
	if empty.Items == nil || len(empty.Items) != 0 || empty.Total != 0 {
		t.Fatalf("unexpected empty list: %+v", empty)
	}
}

func TestRankMap(t *testing.T) {
	list := rankMap(map[string]int{"a": 1, "b": 5, "c": 3, "d": 3}, 3)
	if list.Total != 12 || list.Distinct != 4 || len(list.Items) != 3 {
		t.Fatalf("unexpected ranked list: %+v", list)
	}
	if list.Items[0].Key != "b" || list.Items[1].Key != "c" || list.Items[2].Key != "d" {
		t.Fatalf("unexpected ranking order: %+v", list.Items)
	}
}
