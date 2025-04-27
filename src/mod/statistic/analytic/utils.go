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
		Downstreams:     make(map[string]int),
		Upstreams:       make(map[string]int),
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

		for key, value := range export.Downstreams {
			mergedExport.Downstreams[key] += value
		}

		for key, value := range export.Upstreams {
			mergedExport.Upstreams[key] += value
		}
	}

	return mergedExport
}

func mapToStringSlice(m map[string]int) []string {
	slice := make([]string, 0, len(m))
	for k := range m {
		slice = append(slice, k)
	}
	return slice
}

func isTodayDate(dateStr string) bool {
	today := time.Now().Local().Format("2006-01-02")
	inputDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		inputDate, err = time.Parse("2006_01_02", dateStr)
		if err != nil {
			fmt.Println("Invalid date format")
			return false
		}
	}

	return inputDate.Format("2006-01-02") == today
}
