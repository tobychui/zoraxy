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
	RequestMethods  map[string]int
	Downstreams     map[string]int
	Upstreams       map[string]int

	//Dashboard statistics, empty in summaries recorded before they were introduced
	RequestStatus RequestStatusExport    //Request counts by status class
	StatusCodes   map[string]int         //HTTP status code: request count
	ProxyRules    map[string]int         //Proxy rule primary hostname: request count
	FileTypes     map[string]int         //File type of upstream responses: request count
	RequestPaths  map[string]int         //Request path pattern: request count
	HostPaths     map[string]int         //Requested hostname + path pattern: request count
	BotClientIp   map[string]int         //IPs of clients identified as bots or scripts
	Latency       LatencyHistogramExport //Upstream TTFB histogram
	Hourly        []HourlyStatExport     //Per hour counters (24 entries)
}

func SyncMapToMapStringInt(syncMap *sync.Map) map[string]int {
	result := make(map[string]int)
	if syncMap == nil {
		return result
	}
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
		RequestMethods:  make(map[string]int),
		Downstreams:     make(map[string]int),
		Upstreams:       make(map[string]int),
	}

	export.ForwardTypes = SyncMapToMapStringInt(summary.ForwardTypes)
	export.RequestOrigin = SyncMapToMapStringInt(summary.RequestOrigin)
	export.RequestClientIp = SyncMapToMapStringInt(summary.RequestClientIp)
	export.Referer = SyncMapToMapStringInt(summary.Referer)
	export.UserAgent = SyncMapToMapStringInt(summary.UserAgent)
	export.RequestURL = SyncMapToMapStringInt(summary.RequestURL)
	export.RequestMethods = SyncMapToMapStringInt(summary.RequestMethods)
	export.Downstreams = SyncMapToMapStringInt(summary.DownstreamHostnames)
	export.Upstreams = SyncMapToMapStringInt(summary.UpstreamHostnames)
	export.StatusCodes = SyncMapToMapStringInt(summary.StatusCodes)
	export.ProxyRules = SyncMapToMapStringInt(summary.ProxyRules)
	export.FileTypes = SyncMapToMapStringInt(summary.FileTypes)
	export.RequestPaths = SyncMapToMapStringInt(summary.RequestPaths)
	export.HostPaths = SyncMapToMapStringInt(summary.HostPaths)
	export.BotClientIp = SyncMapToMapStringInt(summary.BotClientIp)
	if summary.status != nil {
		export.RequestStatus = summary.status.export()
	}
	if summary.latency != nil {
		export.Latency = summary.latency.export()
	}
	if summary.hourly != nil {
		export.Hourly = summary.hourly.export()
	}

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
		ForwardTypes:        MapStringIntToSyncMap(export.ForwardTypes),
		RequestOrigin:       MapStringIntToSyncMap(export.RequestOrigin),
		RequestClientIp:     MapStringIntToSyncMap(export.RequestClientIp),
		Referer:             MapStringIntToSyncMap(export.Referer),
		UserAgent:           MapStringIntToSyncMap(export.UserAgent),
		RequestURL:          MapStringIntToSyncMap(export.RequestURL),
		RequestMethods:      MapStringIntToSyncMap(export.RequestMethods),
		DownstreamHostnames: MapStringIntToSyncMap(export.Downstreams),
		UpstreamHostnames:   MapStringIntToSyncMap(export.Upstreams),
		StatusCodes:         MapStringIntToSyncMap(export.StatusCodes),
		ProxyRules:          MapStringIntToSyncMap(export.ProxyRules),
		FileTypes:           MapStringIntToSyncMap(export.FileTypes),
		RequestPaths:        MapStringIntToSyncMap(export.RequestPaths),
		HostPaths:           MapStringIntToSyncMap(export.HostPaths),
		BotClientIp:         MapStringIntToSyncMap(export.BotClientIp),
		status:              newStatusClassCountersFromExport(export.RequestStatus),
		latency:             newLatencyHistogramFromExport(export.Latency),
		hourly:              newHourlyStatsFromExport(export.Hourly),
		bounded: boundedCounters{
			ForwardTypes:        newBoundedCounter(len(export.ForwardTypes)),
			RequestOrigin:       newBoundedCounter(len(export.RequestOrigin)),
			RequestClientIp:     newBoundedCounter(len(export.RequestClientIp)),
			Referer:             newBoundedCounter(len(export.Referer)),
			UserAgent:           newBoundedCounter(len(export.UserAgent)),
			RequestURL:          newBoundedCounter(len(export.RequestURL)),
			RequestMethods:      newBoundedCounter(len(export.RequestMethods)),
			DownstreamHostnames: newBoundedCounter(len(export.Downstreams)),
			UpstreamHostnames:   newBoundedCounter(len(export.Upstreams)),
			StatusCodes:         newBoundedCounter(len(export.StatusCodes)),
			ProxyRules:          newBoundedCounter(len(export.ProxyRules)),
			FileTypes:           newBoundedCounter(len(export.FileTypes)),
			RequestPaths:        newBoundedCounter(len(export.RequestPaths)),
			HostPaths:           newBoundedCounter(len(export.HostPaths)),
			BotClientIp:         newBoundedCounter(len(export.BotClientIp)),
		},
	}

	return summary
}

// External object function call
func (c *Collector) GetExportSummary() *DailySummaryExport {
	exportFormatDailySummary := DailySummaryToExport(*c.DailySummary)
	return &exportFormatDailySummary
}
