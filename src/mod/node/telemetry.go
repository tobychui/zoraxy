package node

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"imuslab.com/zoraxy/mod/statistic"
	"imuslab.com/zoraxy/mod/uptime"
)

const nodeTelemetryMaxUptimeRecordsPerTarget = 288

type AnalyticsRecord struct {
	Date    string                        `json:"date"`
	Summary *statistic.DailySummaryExport `json:"summary,omitempty"`
}

type StreamProxyRuntime struct {
	UUID             string    `json:"uuid"`
	Name             string    `json:"name,omitempty"`
	ListeningAddress string    `json:"listening_address,omitempty"`
	Running          bool      `json:"running"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type TelemetrySnapshot struct {
	GeneratedAt time.Time                      `json:"generated_at"`
	Today       *statistic.DailySummaryExport  `json:"today,omitempty"`
	Analytics   []*AnalyticsRecord             `json:"analytics,omitempty"`
	UptimeLogs  map[string][]*uptime.Record    `json:"uptime_logs,omitempty"`
	StreamProxy map[string]*StreamProxyRuntime `json:"stream_proxy,omitempty"`
}

type TelemetryOverview struct {
	GeneratedAt   time.Time `json:"generated_at"`
	TodayTotal    int64     `json:"today_total"`
	TodayValid    int64     `json:"today_valid"`
	TodayError    int64     `json:"today_error"`
	AnalyticsDays int       `json:"analytics_days"`
	UptimeTargets int       `json:"uptime_targets"`
	UptimeOffline int       `json:"uptime_offline"`
}

type TelemetrySummary struct {
	GeneratedAt          time.Time `json:"generated_at"`
	PrimaryVersion       string    `json:"primary_version,omitempty"`
	RequireVersionMatch  bool      `json:"require_version_match"`
	TotalNodes           int       `json:"total_nodes"`
	EnabledNodes         int       `json:"enabled_nodes"`
	OnlineNodes          int       `json:"online_nodes"`
	LocalOverrideNodes   int       `json:"local_override_nodes"`
	VersionMismatchNodes int       `json:"version_mismatch_nodes"`
	NodesWithTelemetry   int       `json:"nodes_with_telemetry"`
	TodayTotal           int64     `json:"today_total"`
	TodayValid           int64     `json:"today_valid"`
	TodayError           int64     `json:"today_error"`
	UptimeTargets        int       `json:"uptime_targets"`
	UptimeOffline        int       `json:"uptime_offline"`
}

func BuildTelemetryOverview(snapshot *TelemetrySnapshot) *TelemetryOverview {
	if snapshot == nil {
		return nil
	}

	overview := &TelemetryOverview{
		GeneratedAt:   snapshot.GeneratedAt,
		AnalyticsDays: len(snapshot.Analytics),
		UptimeTargets: len(snapshot.UptimeLogs),
	}

	if snapshot.Today != nil {
		overview.TodayTotal = snapshot.Today.TotalRequest
		overview.TodayValid = snapshot.Today.ValidRequest
		overview.TodayError = snapshot.Today.ErrorRequest
	}

	for _, records := range snapshot.UptimeLogs {
		if len(records) == 0 {
			continue
		}
		if !records[len(records)-1].Online {
			overview.UptimeOffline++
		}
	}

	return overview
}

func (m *Manager) getTelemetryFilePath(nodeID string) string {
	return filepath.Join(m.Options.ConfigStore, nodeID+".telemetry.json")
}

func (m *Manager) SaveNodeTelemetry(nodeID string, snapshot *TelemetrySnapshot) error {
	if snapshot == nil {
		return nil
	}

	content, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(m.getTelemetryFilePath(nodeID), content, 0644)
}

func cloneTelemetrySummary(summary *statistic.DailySummaryExport) *statistic.DailySummaryExport {
	if summary == nil {
		return nil
	}

	cloned := *summary
	cloned.ForwardTypes = cloneStringIntMap(summary.ForwardTypes)
	cloned.RequestOrigin = cloneStringIntMap(summary.RequestOrigin)
	cloned.RequestClientIp = cloneStringIntMap(summary.RequestClientIp)
	cloned.Referer = cloneStringIntMap(summary.Referer)
	cloned.UserAgent = cloneStringIntMap(summary.UserAgent)
	cloned.RequestURL = cloneStringIntMap(summary.RequestURL)
	cloned.Downstreams = cloneStringIntMap(summary.Downstreams)
	cloned.Upstreams = cloneStringIntMap(summary.Upstreams)
	return &cloned
}

func cloneStringIntMap(source map[string]int) map[string]int {
	if source == nil {
		return map[string]int{}
	}

	cloned := make(map[string]int, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func cloneAnalyticsRecord(record *AnalyticsRecord) *AnalyticsRecord {
	if record == nil {
		return nil
	}

	return &AnalyticsRecord{
		Date:    record.Date,
		Summary: cloneTelemetrySummary(record.Summary),
	}
}

func cloneUptimeRecord(record *uptime.Record) *uptime.Record {
	if record == nil {
		return nil
	}

	cloned := *record
	return &cloned
}

func cloneStreamProxyRuntime(runtime *StreamProxyRuntime) *StreamProxyRuntime {
	if runtime == nil {
		return nil
	}

	cloned := *runtime
	return &cloned
}

func cloneTelemetrySnapshot(snapshot *TelemetrySnapshot) *TelemetrySnapshot {
	if snapshot == nil {
		return nil
	}

	cloned := &TelemetrySnapshot{
		GeneratedAt: snapshot.GeneratedAt,
		Today:       cloneTelemetrySummary(snapshot.Today),
		Analytics:   make([]*AnalyticsRecord, 0, len(snapshot.Analytics)),
		UptimeLogs:  map[string][]*uptime.Record{},
		StreamProxy: map[string]*StreamProxyRuntime{},
	}

	for _, record := range snapshot.Analytics {
		cloned.Analytics = append(cloned.Analytics, cloneAnalyticsRecord(record))
	}

	for targetID, records := range snapshot.UptimeLogs {
		clonedRecords := make([]*uptime.Record, 0, len(records))
		for _, record := range records {
			if record == nil {
				continue
			}
			clonedRecords = append(clonedRecords, cloneUptimeRecord(record))
		}
		cloned.UptimeLogs[targetID] = clonedRecords
	}

	for uuid, runtime := range snapshot.StreamProxy {
		cloned.StreamProxy[uuid] = cloneStreamProxyRuntime(runtime)
	}

	return cloned
}

func mergeAnalyticsRecords(existing []*AnalyticsRecord, incoming []*AnalyticsRecord) []*AnalyticsRecord {
	recordMap := map[string]*AnalyticsRecord{}
	for _, record := range existing {
		if record == nil || record.Date == "" {
			continue
		}
		recordMap[record.Date] = cloneAnalyticsRecord(record)
	}
	for _, record := range incoming {
		if record == nil || record.Date == "" {
			continue
		}
		recordMap[record.Date] = cloneAnalyticsRecord(record)
	}

	keys := make([]string, 0, len(recordMap))
	for key := range recordMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	results := make([]*AnalyticsRecord, 0, len(keys))
	for _, key := range keys {
		results = append(results, recordMap[key])
	}
	return results
}

func mergeUptimeRecords(existing []*uptime.Record, incoming []*uptime.Record) []*uptime.Record {
	seen := map[string]bool{}
	results := make([]*uptime.Record, 0, len(existing)+len(incoming))
	appendRecord := func(record *uptime.Record) {
		if record == nil {
			return
		}
		recordKey := fmt.Sprintf("%d|%t|%d|%d|%s", record.Timestamp, record.Online, record.StatusCode, record.Latency, record.URL)
		if seen[recordKey] {
			return
		}
		seen[recordKey] = true
		results = append(results, cloneUptimeRecord(record))
	}

	for _, record := range existing {
		appendRecord(record)
	}
	for _, record := range incoming {
		appendRecord(record)
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].Timestamp == results[j].Timestamp {
			return results[i].URL < results[j].URL
		}
		return results[i].Timestamp < results[j].Timestamp
	})

	if len(results) > nodeTelemetryMaxUptimeRecordsPerTarget {
		results = results[len(results)-nodeTelemetryMaxUptimeRecordsPerTarget:]
	}

	return results
}

func MergeTelemetrySnapshots(existing *TelemetrySnapshot, incoming *TelemetrySnapshot) *TelemetrySnapshot {
	if existing == nil {
		return cloneTelemetrySnapshot(incoming)
	}
	if incoming == nil {
		return cloneTelemetrySnapshot(existing)
	}

	merged := cloneTelemetrySnapshot(existing)
	if incoming.GeneratedAt.After(merged.GeneratedAt) || merged.GeneratedAt.IsZero() {
		merged.GeneratedAt = incoming.GeneratedAt
	}
	if incoming.Today != nil {
		merged.Today = cloneTelemetrySummary(incoming.Today)
	}

	merged.Analytics = mergeAnalyticsRecords(merged.Analytics, incoming.Analytics)

	if merged.UptimeLogs == nil {
		merged.UptimeLogs = map[string][]*uptime.Record{}
	}
	for targetID, records := range incoming.UptimeLogs {
		merged.UptimeLogs[targetID] = mergeUptimeRecords(merged.UptimeLogs[targetID], records)
	}

	if incoming.StreamProxy != nil {
		if merged.StreamProxy == nil {
			merged.StreamProxy = map[string]*StreamProxyRuntime{}
		}
		for uuid, runtime := range incoming.StreamProxy {
			merged.StreamProxy[uuid] = cloneStreamProxyRuntime(runtime)
		}
	}

	return merged
}

func (m *Manager) MergeNodeTelemetry(nodeID string, snapshot *TelemetrySnapshot) error {
	if snapshot == nil {
		return nil
	}

	existing, err := m.LoadNodeTelemetry(nodeID)
	if err != nil {
		return err
	}

	return m.SaveNodeTelemetry(nodeID, MergeTelemetrySnapshots(existing, snapshot))
}

func (m *Manager) UpdateNodeStreamProxyRuntime(nodeID string, streamRuntime map[string]*StreamProxyRuntime) error {
	snapshot, err := m.LoadNodeTelemetry(nodeID)
	if err != nil {
		return err
	}
	if snapshot == nil {
		snapshot = &TelemetrySnapshot{}
	}

	if streamRuntime == nil {
		streamRuntime = map[string]*StreamProxyRuntime{}
	}

	snapshot.GeneratedAt = time.Now()
	snapshot.StreamProxy = streamRuntime
	return m.SaveNodeTelemetry(nodeID, snapshot)
}

func (m *Manager) LoadNodeTelemetry(nodeID string) (*TelemetrySnapshot, error) {
	content, err := os.ReadFile(m.getTelemetryFilePath(nodeID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	telemetry := &TelemetrySnapshot{}
	if err := json.Unmarshal(content, telemetry); err != nil {
		return nil, err
	}

	return telemetry, nil
}

func (m *Manager) BuildTelemetrySummary() (*TelemetrySummary, error) {
	summary := &TelemetrySummary{
		GeneratedAt:         time.Now(),
		PrimaryVersion:      m.GetPrimaryVersion(),
		RequireVersionMatch: m.IsVersionMatchRequired(),
	}

	if m == nil {
		return summary, nil
	}

	summary.TotalNodes = len(m.Nodes)
	for _, currentNode := range m.Nodes {
		if currentNode == nil {
			continue
		}

		if currentNode.Enabled {
			summary.EnabledNodes++
		}
		if m.IsNodeOnline(currentNode) {
			summary.OnlineNodes++
		}
		if currentNode.LocalOverride {
			summary.LocalOverrideNodes++
		}
		if mismatch, _ := m.GetNodeVersionMismatch(currentNode); mismatch {
			summary.VersionMismatchNodes++
		}

		telemetry, err := m.LoadNodeTelemetry(currentNode.ID)
		if err != nil {
			return nil, err
		}
		overview := BuildTelemetryOverview(telemetry)
		if overview == nil {
			continue
		}

		summary.NodesWithTelemetry++
		summary.TodayTotal += overview.TodayTotal
		summary.TodayValid += overview.TodayValid
		summary.TodayError += overview.TodayError
		summary.UptimeTargets += overview.UptimeTargets
		summary.UptimeOffline += overview.UptimeOffline
	}

	return summary, nil
}
