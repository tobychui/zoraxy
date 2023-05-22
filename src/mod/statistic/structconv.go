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
	}

	summary.ForwardTypes.Range(func(key, value interface{}) bool {
		export.ForwardTypes[key.(string)] = value.(int)
		return true
	})

	summary.RequestOrigin.Range(func(key, value interface{}) bool {
		export.RequestOrigin[key.(string)] = value.(int)
		return true
	})

	summary.RequestClientIp.Range(func(key, value interface{}) bool {
		export.RequestClientIp[key.(string)] = value.(int)
		return true
	})

	summary.Referer.Range(func(key, value interface{}) bool {
		export.Referer[key.(string)] = value.(int)
		return true
	})

	summary.UserAgent.Range(func(key, value interface{}) bool {
		export.UserAgent[key.(string)] = value.(int)
		return true
	})

	summary.RequestURL.Range(func(key, value interface{}) bool {
		export.RequestURL[key.(string)] = value.(int)
		return true
	})

	return export
}

func DailySummaryExportToSummary(export DailySummaryExport) DailySummary {
	summary := DailySummary{
		TotalRequest:    export.TotalRequest,
		ErrorRequest:    export.ErrorRequest,
		ValidRequest:    export.ValidRequest,
		ForwardTypes:    &sync.Map{},
		RequestOrigin:   &sync.Map{},
		RequestClientIp: &sync.Map{},
		Referer:         &sync.Map{},
		UserAgent:       &sync.Map{},
		RequestURL:      &sync.Map{},
	}

	for k, v := range export.ForwardTypes {
		summary.ForwardTypes.Store(k, v)
	}

	for k, v := range export.RequestOrigin {
		summary.RequestOrigin.Store(k, v)
	}

	for k, v := range export.RequestClientIp {
		summary.RequestClientIp.Store(k, v)
	}

	for k, v := range export.Referer {
		summary.Referer.Store(k, v)
	}

	for k, v := range export.UserAgent {
		summary.UserAgent.Store(k, v)
	}

	for k, v := range export.RequestURL {
		summary.RequestURL.Store(k, v)
	}

	return summary
}

// External object function call
func (c *Collector) GetExportSummary() *DailySummaryExport {
	exportFormatDailySummary := DailySummaryToExport(*c.DailySummary)
	return &exportFormatDailySummary
}
