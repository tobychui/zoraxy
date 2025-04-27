package statistic

import "sync"

type DailySummaryExport struct {
	TotalRequest int64 //Total request of the day
	ErrorRequest int64 //Invalid request of the day, including error or not found
	ValidRequest int64 //Valid request of the day

	ForwardTypes    map[string]int
	RequestOrigin   map[string]int
	RequestClientIp map[string]int
	Referer         map[string]int
	UserAgent       map[string]int
	RequestURL      map[string]int
	Downstreams     map[string]int
	Upstreams       map[string]int
}

func SyncMapToMapStringInt(syncMap *sync.Map) map[string]int {
	result := make(map[string]int)
	syncMap.Range(func(key, value interface{}) bool {
		strKey, okKey := key.(string)
		intValue, okValue := value.(int)
		if okKey && okValue {
			result[strKey] = intValue
		}
		return true
	})
	return result
}

func DailySummaryToExport(summary DailySummary) DailySummaryExport {
	export := DailySummaryExport{
		TotalRequest:    summary.TotalRequest,
		ErrorRequest:    summary.ErrorRequest,
		ValidRequest:    summary.ValidRequest,
		ForwardTypes:    make(map[string]int),
		RequestOrigin:   make(map[string]int),
		RequestClientIp: make(map[string]int),
		Referer:         make(map[string]int),
		UserAgent:       make(map[string]int),
		RequestURL:      make(map[string]int),
		Downstreams:     make(map[string]int),
		Upstreams:       make(map[string]int),
	}

	export.ForwardTypes = SyncMapToMapStringInt(summary.ForwardTypes)
	export.RequestOrigin = SyncMapToMapStringInt(summary.RequestOrigin)
	export.RequestClientIp = SyncMapToMapStringInt(summary.RequestClientIp)
	export.Referer = SyncMapToMapStringInt(summary.Referer)
	export.UserAgent = SyncMapToMapStringInt(summary.UserAgent)
	export.RequestURL = SyncMapToMapStringInt(summary.RequestURL)
	export.Downstreams = SyncMapToMapStringInt(summary.DownstreamHostnames)
	export.Upstreams = SyncMapToMapStringInt(summary.UpstreamHostnames)

	return export
}

func MapStringIntToSyncMap(m map[string]int) *sync.Map {
	syncMap := &sync.Map{}
	for k, v := range m {
		syncMap.Store(k, v)
	}
	return syncMap
}

func DailySummaryExportToSummary(export DailySummaryExport) DailySummary {
	summary := DailySummary{
		TotalRequest:        export.TotalRequest,
		ErrorRequest:        export.ErrorRequest,
		ValidRequest:        export.ValidRequest,
		ForwardTypes:        &sync.Map{},
		RequestOrigin:       &sync.Map{},
		RequestClientIp:     &sync.Map{},
		Referer:             &sync.Map{},
		UserAgent:           &sync.Map{},
		RequestURL:          &sync.Map{},
		DownstreamHostnames: &sync.Map{},
		UpstreamHostnames:   &sync.Map{},
	}

	summary.ForwardTypes = MapStringIntToSyncMap(export.ForwardTypes)
	summary.RequestOrigin = MapStringIntToSyncMap(export.RequestOrigin)
	summary.RequestClientIp = MapStringIntToSyncMap(export.RequestClientIp)
	summary.Referer = MapStringIntToSyncMap(export.Referer)
	summary.UserAgent = MapStringIntToSyncMap(export.UserAgent)
	summary.RequestURL = MapStringIntToSyncMap(export.RequestURL)
	summary.DownstreamHostnames = MapStringIntToSyncMap(export.Downstreams)
	summary.UpstreamHostnames = MapStringIntToSyncMap(export.Upstreams)

	return summary
}

// External object function call
func (c *Collector) GetExportSummary() *DailySummaryExport {
	exportFormatDailySummary := DailySummaryToExport(*c.DailySummary)
	return &exportFormatDailySummary
}
