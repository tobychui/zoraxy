package statistic

import "sync"

type DailySummaryExport struct {
	TotalRequest int64 //Total request of the day
	ErrorRequest int64 //Invalid request of the day, including error or not found
	ValidRequest int64 //Valid request of the day

	ForwardTypes    map[string]int
	RequestOrigin   map[string]int
	RequestClientIp map[string]int
}

func DailySummaryToExport(summary DailySummary) DailySummaryExport {
	export := DailySummaryExport{
		TotalRequest:    summary.TotalRequest,
		ErrorRequest:    summary.ErrorRequest,
		ValidRequest:    summary.ValidRequest,
		ForwardTypes:    make(map[string]int),
		RequestOrigin:   make(map[string]int),
		RequestClientIp: make(map[string]int),
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

	return summary
}
