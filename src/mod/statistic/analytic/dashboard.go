package analytic

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"imuslab.com/zoraxy/mod/statistic"
	"imuslab.com/zoraxy/mod/utils"
)

/*
	Dashboard.go

	Aggregates the daily summaries of a date range into the data
	rendered by the statistic dashboard, so the browser does not
	need to download and merge every raw daily summary
*/

const (
	statKeyLayout       = "2006_01_02"
	dashboardMaxDays    = 400  //Maximum number of days that can be selected at once
	dashboardListLimit  = 100  //Number of entries returned for ranked tables
	dashboardAgentLimit = 3000 //Number of user agents returned for client side parsing
	endpointDomainLimit = 500  //Number of hostname + endpoint rows returned by the endpoint details list
)

// EndpointDomainItem is a path pattern requested on a hostname
type EndpointDomainItem struct {
	Host  string //Hostname the client requested, empty if the request had no Host header
	Path  string //Path pattern, e.g. /api/*
	Count int
}

// EndpointDomainList is the hostname breakdown of the top endpoints
type EndpointDomainList struct {
	Items       []EndpointDomainItem //Top hostname + path pattern pairs sorted by count (descending)
	Total       int64                //Requests with hostname information in the range
	Distinct    int                  //Number of distinct hostname + path pattern pairs
	MissingDays int                  //Days with requests but no hostname breakdown (recorded before it was collected)
}

// RankedItem is a single row of a ranked table
type RankedItem struct {
	Key   string
	Count int
}

// RankedList holds the top entries of a statistic map
type RankedList struct {
	Items    []RankedItem //Top entries sorted by count (descending)
	Total    int64        //Sum of all entries, including the ones not listed
	Distinct int          //Number of distinct keys
}

// PeriodTotals is the summary of a date range shown in the KPI cards
type PeriodTotals struct {
	TotalRequest   int64
	UniqueVisitors int
	Success        int64
	Redirect       int64
	Blocked        int64
	Error          int64
}

// TrendPoint is a single point of the traffic and response time trend graphs
type TrendPoint struct {
	Label   string
	Total   int64
	Blocked int64
	Error   int64
	AvgMs   *float64 //Average upstream response time, null if no data
	P95Ms   *float64 //95th percentile upstream response time, null if no data
}

// LatencySummary is the upstream response time (TTFB) of a date range
type LatencySummary struct {
	Count int64
	AvgMs float64
	P50Ms float64
	P95Ms float64
	P99Ms float64
}

// UserBreakdown splits the unique visitors (client IPs) of a date range
type UserBreakdown struct {
	Returning int //Seen on two or more days, including the previous period
	New       int //Seen on a single day only
	Bots      int //Identified as bots or scripts by their User-Agent
}

// TrafficSources classifies the requests by their Referer header
type TrafficSources struct {
	Direct   int64 //No referer
	Search   int64 //Referred by a search engine
	Referral int64 //Referred by another external site
	Internal int64 //Referred by one of the proxied hostnames (e.g. page resources)
}

// IPVersions counts the unique visitors by IP version
type IPVersions struct {
	IPv4 int
	IPv6 int
}

// DashboardData is the payload of the statistic dashboard
type DashboardData struct {
	Start          string //Selected range, YYYY-MM-DD
	End            string
	PrevStart      string //Previous period of the same length used for comparison
	PrevEnd        string
	Days           int
	Granularity    string //"hour" for single day ranges, otherwise "day"
	Current        PeriodTotals
	Previous       PeriodTotals
	Trend          []TrendPoint
	Latency        LatencySummary
	Users          UserBreakdown
	TrafficSources TrafficSources
	IPVersions     IPVersions
	Applications   RankedList
	Domains        RankedList
	Upstreams      RankedList
	Endpoints      RankedList
	Referrers      RankedList
	Countries      RankedList
	Methods        RankedList
	StatusCodes    RankedList
	FileTypes      RankedList
	ClientIPs      RankedList
	ForwardTypes   RankedList
	UserAgents     RankedList
}

// dayRecord is a loaded daily summary together with its date
type dayRecord struct {
	Date    time.Time
	Summary *statistic.DailySummaryExport //nil if no data was recorded on that day
}

// loadDay loads the summary of the given date key, using the in-memory summary for today
func (d *DataLoader) loadDay(key string) *statistic.DailySummaryExport {
	if key == time.Now().Format(statKeyLayout) && d.StatisticCollector != nil {
		return d.StatisticCollector.GetExportSummary()
	}
	summary := statistic.DailySummaryExport{}
	if err := d.Database.Read("stats", key, &summary); err != nil {
		return nil
	}
	return &summary
}

// loadDays loads all the daily summaries between start and end (inclusive)
func (d *DataLoader) loadDays(start, end time.Time) []dayRecord {
	records := []dayRecord{}
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		records = append(records, dayRecord{
			Date:    day,
			Summary: d.loadDay(day.Format(statKeyLayout)),
		})
	}
	return records
}

// parseDashboardRange reads the start and end dates of a dashboard request. The range
// defaults to today, is put in order, clamped to today and limited to dashboardMaxDays.
func (d *DataLoader) parseDashboardRange(r *http.Request, now time.Time) (start time.Time, end time.Time, days int, err error) {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)

	startStr, endStr, err := d.GetStartAndEndDatesFromRequest(r)
	if err != nil {
		//Default to today
		startStr = today.Format(statKeyLayout)
		endStr = startStr
	}
	start, err = time.ParseInLocation(statKeyLayout, startStr, time.Local)
	if err != nil {
		return start, end, 0, errors.New("invalid start date")
	}
	end, err = time.ParseInLocation(statKeyLayout, endStr, time.Local)
	if err != nil {
		return start, end, 0, errors.New("invalid end date")
	}
	if start.After(end) {
		start, end = end, start
	}
	if end.After(today) {
		end = today
	}
	if start.After(end) {
		return start, end, 0, errors.New("given date range is in the future")
	}

	days = int(end.Sub(start).Hours()/24+0.5) + 1
	if days > dashboardMaxDays {
		return start, end, days, fmt.Errorf("date range cannot be longer than %d days", dashboardMaxDays)
	}
	return start, end, days, nil
}

// HandleDashboard returns the aggregated statistics of a date range for the statistic dashboard
func (d *DataLoader) HandleDashboard(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	start, end, days, err := d.parseDashboardRange(r, now)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	prevEnd := start.AddDate(0, 0, -1)
	prevStart := start.AddDate(0, 0, -days)

	current := d.loadDays(start, end)
	previous := d.loadDays(prevStart, prevEnd)

	js, err := json.Marshal(buildDashboardData(current, previous, start, end, prevStart, prevEnd, now))
	if err != nil {
		utils.SendErrorResponse(w, "unable to encode dashboard data")
		return
	}
	utils.SendJSONResponse(w, string(js))
}

// HandleEndpointDomains returns the top endpoints together with the hostname they were
// requested on. It is loaded on demand by the endpoint details list, keeping the main
// dashboard payload small.
func (d *DataLoader) HandleEndpointDomains(w http.ResponseWriter, r *http.Request) {
	start, end, _, err := d.parseDashboardRange(r, time.Now())
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	js, err := json.Marshal(buildEndpointDomainList(d.loadDays(start, end), endpointDomainLimit))
	if err != nil {
		utils.SendErrorResponse(w, "unable to encode endpoint data")
		return
	}
	utils.SendJSONResponse(w, string(js))
}

// buildEndpointDomainList merges the hostname + path pattern counters of the given days
func buildEndpointDomainList(records []dayRecord, limit int) EndpointDomainList {
	hostPaths := map[string]int{}
	missingDays := 0
	for _, record := range records {
		if record.Summary == nil {
			continue
		}
		if len(record.Summary.HostPaths) == 0 {
			if record.Summary.TotalRequest > 0 {
				missingDays++
			}
			continue
		}
		for key, count := range record.Summary.HostPaths {
			hostPaths[key] += count
		}
	}

	ranked := rankMap(hostPaths, limit)
	list := EndpointDomainList{
		Items:       []EndpointDomainItem{},
		Total:       ranked.Total,
		Distinct:    ranked.Distinct,
		MissingDays: missingDays,
	}
	for _, item := range ranked.Items {
		host, path := statistic.SplitHostPathKey(item.Key)
		list.Items = append(list.Items, EndpointDomainItem{Host: host, Path: path, Count: item.Count})
	}
	return list
}

// buildDashboardData aggregates the loaded days into the dashboard payload
func buildDashboardData(current, previous []dayRecord, start, end, prevStart, prevEnd, now time.Time) *DashboardData {
	data := DashboardData{
		Start:       start.Format("2006-01-02"),
		End:         end.Format("2006-01-02"),
		PrevStart:   prevStart.Format("2006-01-02"),
		PrevEnd:     prevEnd.Format("2006-01-02"),
		Days:        len(current),
		Granularity: "day",
	}

	currentSummaries := summariesOf(current)
	merged := mergeDailySummaryExports(currentSummaries)

	//KPI cards
	data.Current = periodTotals(currentSummaries, merged)
	data.Previous = periodTotals(summariesOf(previous), nil)

	//Trend graphs
	if len(current) == 1 {
		data.Granularity = "hour"
		data.Trend = hourlyTrend(current[0], now)
	} else {
		data.Trend = dailyTrend(current)
	}

	//Upstream response time
	data.Latency = LatencySummary{
		Count: merged.Latency.Count,
		AvgMs: merged.Latency.AverageMs(),
		P50Ms: merged.Latency.PercentileMs(0.50),
		P95Ms: merged.Latency.PercentileMs(0.95),
		P99Ms: merged.Latency.PercentileMs(0.99),
	}

	//Visitors
	data.Users = userBreakdown(merged, current, previous)
	for ip := range merged.RequestClientIp {
		if strings.Contains(ip, ":") {
			data.IPVersions.IPv6++
		} else if strings.Contains(ip, ".") {
			data.IPVersions.IPv4++
		}
	}

	//Traffic sources and referring sites
	ownHosts := newHostMatcher(merged.Downstreams, merged.ProxyRules)
	referrerOrigins := map[string]int{}
	for referer, count := range merged.Referer {
		switch classifyReferer(referer, ownHosts) {
		case "direct":
			data.TrafficSources.Direct += int64(count)
		case "search":
			data.TrafficSources.Search += int64(count)
		case "internal":
			data.TrafficSources.Internal += int64(count)
		default:
			data.TrafficSources.Referral += int64(count)
		}
		referrerOrigins[refererOrigin(referer)] += count
	}

	//Ranked tables
	data.Applications = rankMap(merged.ProxyRules, dashboardListLimit)
	data.Domains = rankMap(merged.Downstreams, dashboardListLimit)
	data.Upstreams = rankMap(merged.Upstreams, dashboardListLimit)
	data.Endpoints = rankMap(merged.RequestPaths, dashboardListLimit)
	data.Referrers = rankMap(referrerOrigins, dashboardListLimit)
	data.Countries = rankMap(merged.RequestOrigin, 300)
	data.Methods = rankMap(merged.RequestMethods, dashboardListLimit)
	data.StatusCodes = rankMap(merged.StatusCodes, dashboardListLimit)
	data.FileTypes = rankMap(merged.FileTypes, dashboardListLimit)
	data.ClientIPs = rankMap(merged.RequestClientIp, dashboardListLimit)
	data.ForwardTypes = rankMap(merged.ForwardTypes, dashboardListLimit)
	data.UserAgents = rankMap(merged.UserAgent, dashboardAgentLimit)

	return &data
}

func summariesOf(records []dayRecord) []*statistic.DailySummaryExport {
	summaries := []*statistic.DailySummaryExport{}
	for _, record := range records {
		if record.Summary != nil {
			summaries = append(summaries, record.Summary)
		}
	}
	return summaries
}

// statusOf returns the status class counters of a day. Requests recorded before
// the status classes were introduced are attributed using the legacy
// valid / error counters, so older days still show a meaningful split.
func statusOf(summary *statistic.DailySummaryExport) statistic.RequestStatusExport {
	status := summary.RequestStatus
	unclassified := summary.TotalRequest - status.Total()
	if unclassified <= 0 {
		return status
	}
	legacyErrors := summary.ErrorRequest - status.Blocked - status.Error
	if legacyErrors < 0 {
		legacyErrors = 0
	}
	if legacyErrors > unclassified {
		legacyErrors = unclassified
	}
	status.Error += legacyErrors
	status.Success += unclassified - legacyErrors
	return status
}

// periodTotals sums up the KPI values of the given days. merged can be nil,
// in which case the unique visitors are computed from the given days.
func periodTotals(summaries []*statistic.DailySummaryExport, merged *statistic.DailySummaryExport) PeriodTotals {
	totals := PeriodTotals{}
	uniqueIPs := map[string]struct{}{}
	for _, summary := range summaries {
		totals.TotalRequest += summary.TotalRequest
		status := statusOf(summary)
		totals.Success += status.Success
		totals.Redirect += status.Redirect
		totals.Blocked += status.Blocked
		totals.Error += status.Error
		if merged == nil {
			for ip := range summary.RequestClientIp {
				uniqueIPs[ip] = struct{}{}
			}
		}
	}
	if merged != nil {
		totals.UniqueVisitors = len(merged.RequestClientIp)
	} else {
		totals.UniqueVisitors = len(uniqueIPs)
	}
	return totals
}

func latencyPointer(value float64) *float64 {
	return &value
}

// dailyTrend returns one trend point per day of the range
func dailyTrend(records []dayRecord) []TrendPoint {
	points := []TrendPoint{}
	for _, record := range records {
		point := TrendPoint{Label: record.Date.Format("2006-01-02")}
		if record.Summary != nil {
			status := statusOf(record.Summary)
			point.Total = record.Summary.TotalRequest
			point.Blocked = status.Blocked
			point.Error = status.Error
			if record.Summary.Latency.Count > 0 {
				point.AvgMs = latencyPointer(record.Summary.Latency.AverageMs())
				point.P95Ms = latencyPointer(record.Summary.Latency.PercentileMs(0.95))
			}
		}
		points = append(points, point)
	}
	return points
}

// hourlyTrend returns one trend point per hour of a single day
func hourlyTrend(record dayRecord, now time.Time) []TrendPoint {
	lastHour := 23
	if record.Date.Year() == now.Year() && record.Date.YearDay() == now.YearDay() {
		lastHour = now.Hour()
	}

	points := []TrendPoint{}
	for hour := 0; hour <= lastHour; hour++ {
		point := TrendPoint{Label: fmt.Sprintf("%02d:00", hour)}
		if record.Summary != nil && hour < len(record.Summary.Hourly) {
			slot := record.Summary.Hourly[hour]
			point.Total = slot.Total
			point.Blocked = slot.Blocked
			point.Error = slot.Error
			if slot.Latency.Count > 0 {
				point.AvgMs = latencyPointer(slot.Latency.AverageMs())
				point.P95Ms = latencyPointer(slot.Latency.PercentileMs(0.95))
			}
		}
		points = append(points, point)
	}
	return points
}

// userBreakdown splits the unique visitors of the current range into
// bots (mostly bot User-Agent requests), returning visitors (seen on 2+
// days across the current and the previous period) and new visitors
// (seen on a single day)
func userBreakdown(merged *statistic.DailySummaryExport, current, previous []dayRecord) UserBreakdown {
	daysSeen := map[string]int{}
	for _, records := range [][]dayRecord{current, previous} {
		for _, record := range records {
			if record.Summary == nil {
				continue
			}
			for ip := range record.Summary.RequestClientIp {
				daysSeen[ip]++
			}
		}
	}

	breakdown := UserBreakdown{}
	for ip, requests := range merged.RequestClientIp {
		//An IP is a bot if most of its requests came with bot User-Agents, so a
		//single script or uptime monitor behind a shared (NAT) IP does not turn
		//all of its human visitors into bots
		if botRequests := merged.BotClientIp[ip]; botRequests*2 > requests {
			breakdown.Bots++
		} else if daysSeen[ip] >= 2 {
			breakdown.Returning++
		} else {
			breakdown.New++
		}
	}
	return breakdown
}

// rankMap sorts a statistic map by count and returns the top entries
func rankMap(m map[string]int, limit int) RankedList {
	list := RankedList{
		Items:    []RankedItem{},
		Distinct: len(m),
	}
	for key, count := range m {
		list.Total += int64(count)
		list.Items = append(list.Items, RankedItem{Key: key, Count: count})
	}
	sort.Slice(list.Items, func(i, j int) bool {
		if list.Items[i].Count == list.Items[j].Count {
			return list.Items[i].Key < list.Items[j].Key
		}
		return list.Items[i].Count > list.Items[j].Count
	})
	if len(list.Items) > limit {
		list.Items = list.Items[:limit]
	}
	return list
}

/*
	Referer classification
*/

// Labels of well known search engine domains, e.g. google in www.google.com.tw
var searchEngineLabels = map[string]bool{
	"google": true, "bing": true, "duckduckgo": true, "yahoo": true, "baidu": true,
	"yandex": true, "ecosia": true, "startpage": true, "qwant": true, "naver": true,
	"sogou": true, "seznam": true, "ask": true,
}

func isSearchEngineHost(host string) bool {
	if host == "search.brave.com" {
		return true
	}
	labels := strings.Split(host, ".")
	//The engine label must be followed by public suffix like labels only
	//(e.g. google.com, google.co.uk, google.com.tw) so google.example.org does not match
	for i := 0; i < len(labels)-1; i++ {
		if !searchEngineLabels[labels[i]] {
			continue
		}
		suffixOnly := true
		for _, suffix := range labels[i+1:] {
			if len(suffix) > 3 {
				suffixOnly = false
				break
			}
		}
		if suffixOnly {
			return true
		}
	}
	return false
}

// hostMatcher matches hostnames that are served by this Zoraxy instance
type hostMatcher struct {
	exact    map[string]bool
	wildcard []string //Suffixes of wildcard rules, e.g. .example.com
}

func newHostMatcher(hostLists ...map[string]int) *hostMatcher {
	matcher := &hostMatcher{exact: map[string]bool{}}
	for _, hosts := range hostLists {
		for host := range hosts {
			host = strings.ToLower(strings.TrimSpace(host))
			if h, _, err := splitHostPortLoose(host); err == nil {
				host = h
			}
			if host == "" || host == "/" {
				continue
			}
			if strings.HasPrefix(host, "*.") {
				matcher.wildcard = append(matcher.wildcard, host[1:])
			} else {
				matcher.exact[host] = true
			}
		}
	}
	return matcher
}

func (m *hostMatcher) match(host string) bool {
	if m.exact[host] {
		return true
	}
	for _, suffix := range m.wildcard {
		if strings.HasSuffix(host, suffix) {
			return true
		}
	}
	return false
}

// splitHostPortLoose strips the port from a host, accepting hosts without a port
func splitHostPortLoose(hostport string) (string, string, error) {
	u, err := url.Parse("//" + hostport)
	if err != nil {
		return "", "", err
	}
	return u.Hostname(), u.Port(), nil
}

// classifyReferer returns one of direct, search, internal or referral
func classifyReferer(referer string, ownHosts *hostMatcher) string {
	referer = strings.TrimSpace(referer)
	if referer == "" {
		return "direct"
	}
	u, err := url.Parse(referer)
	if err != nil || u.Hostname() == "" {
		return "referral"
	}
	host := strings.ToLower(u.Hostname())
	if ownHosts.match(host) {
		return "internal"
	}
	if isSearchEngineHost(host) {
		return "search"
	}
	return "referral"
}

// refererOrigin reduces a referer URL to its origin (scheme://host) for the referring sites table
func refererOrigin(referer string) string {
	referer = strings.TrimSpace(referer)
	if referer == "" {
		return ""
	}
	u, err := url.Parse(referer)
	if err != nil || u.Host == "" {
		return referer
	}
	return u.Scheme + "://" + strings.ToLower(u.Host)
}
