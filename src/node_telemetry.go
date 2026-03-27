package main

import (
	"encoding/json"
	"sort"
	"time"

	"imuslab.com/zoraxy/mod/node"
	"imuslab.com/zoraxy/mod/statistic"
	"imuslab.com/zoraxy/mod/uptime"
)

func buildNodeTelemetrySnapshot() (*node.TelemetrySnapshot, error) {
	results := &node.TelemetrySnapshot{
		GeneratedAt: time.Now(),
		UptimeLogs:  map[string][]*uptime.Record{},
		StreamProxy: map[string]*node.StreamProxyRuntime{},
	}

	if statisticCollector != nil {
		todaySummary := statisticCollector.GetExportSummary()
		if todaySummary != nil {
			clonedSummary := *todaySummary
			results.Today = &clonedSummary
		}
	}

	analyticsRecords, err := loadNodeAnalyticsRecords()
	if err != nil {
		return nil, err
	}
	results.Analytics = analyticsRecords

	if uptimeMonitor != nil {
		results.UptimeLogs = uptimeMonitor.ExportOnlineStatusLog()
	}

	streamRuntime, err := buildNodeStreamProxyRuntimeSnapshot()
	if err != nil {
		return nil, err
	}
	results.StreamProxy = streamRuntime

	return results, nil
}

func buildNodeStreamProxyRuntimeSnapshot() (map[string]*node.StreamProxyRuntime, error) {
	results := map[string]*node.StreamProxyRuntime{}
	generatedAt := time.Now()

	if streamProxyManager == nil {
		return results, nil
	}

	for _, config := range streamProxyManager.Configs {
		if config == nil {
			continue
		}

		results[config.UUID] = &node.StreamProxyRuntime{
			UUID:             config.UUID,
			Name:             config.Name,
			ListeningAddress: config.ListeningAddress,
			Running:          config.IsRunning() || config.Running,
			UpdatedAt:        generatedAt,
		}
	}

	return results, nil
}

func loadNodeAnalyticsRecords() ([]*node.AnalyticsRecord, error) {
	results := []*node.AnalyticsRecord{}
	if sysdb == nil || !sysdb.TableExists("stats") {
		return results, nil
	}

	recordMap := map[string]*node.AnalyticsRecord{}
	entries, err := sysdb.ListTable("stats")
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		summary := &statistic.DailySummaryExport{}
		if err := json.Unmarshal(entry[1], summary); err != nil {
			return nil, err
		}

		recordMap[string(entry[0])] = &node.AnalyticsRecord{
			Date:    string(entry[0]),
			Summary: summary,
		}
	}

	if statisticCollector != nil {
		todayKey := time.Now().Format("2006_01_02")
		todaySummary := statisticCollector.GetExportSummary()
		if todaySummary != nil {
			clonedSummary := *todaySummary
			recordMap[todayKey] = &node.AnalyticsRecord{
				Date:    todayKey,
				Summary: &clonedSummary,
			}
		}
	}

	keys := make([]string, 0, len(recordMap))
	for key := range recordMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		results = append(results, recordMap[key])
	}

	return results, nil
}
