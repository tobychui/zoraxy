package analytic

import (
	"fmt"
	"time"

	"imuslab.com/zoraxy/mod/statistic"
)

// Generate all the record keys from a given start and end dates
func generateDateRange(startDate, endDate string) ([]string, error) {
	layout := "2006_01_02"
	start, err := time.Parse(layout, startDate)
	if err != nil {
		return nil, fmt.Errorf("error parsing start date: %v", err)
	}

	end, err := time.Parse(layout, endDate)
	if err != nil {
		return nil, fmt.Errorf("error parsing end date: %v", err)
	}

	var dateRange []string
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		dateRange = append(dateRange, d.Format(layout))
	}

	return dateRange, nil
}

func mergeDailySummaryExports(exports []*statistic.DailySummaryExport) *statistic.DailySummaryExport {
	mergedExport := &statistic.DailySummaryExport{
		ForwardTypes:    make(map[string]int),
		RequestOrigin:   make(map[string]int),
		RequestClientIp: make(map[string]int),
		Referer:         make(map[string]int),
		UserAgent:       make(map[string]int),
		RequestURL:      make(map[string]int),
	}

	for _, export := range exports {
		mergedExport.TotalRequest += export.TotalRequest
		mergedExport.ErrorRequest += export.ErrorRequest
		mergedExport.ValidRequest += export.ValidRequest

		for key, value := range export.ForwardTypes {
			mergedExport.ForwardTypes[key] += value
		}

		for key, value := range export.RequestOrigin {
			mergedExport.RequestOrigin[key] += value
		}

		for key, value := range export.RequestClientIp {
			mergedExport.RequestClientIp[key] += value
		}

		for key, value := range export.Referer {
			mergedExport.Referer[key] += value
		}

		for key, value := range export.UserAgent {
			mergedExport.UserAgent[key] += value
		}

		for key, value := range export.RequestURL {
			mergedExport.RequestURL[key] += value
		}
	}

	return mergedExport
}
