package analytic

import (
	"errors"
	"net/http"
	"strings"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/statistic"
	"imuslab.com/zoraxy/mod/utils"
)

type DataLoader struct {
	Database           *database.Database
	StatisticCollector *statistic.Collector
}

// Create a new data loader for loading statistic from database
func NewDataLoader(db *database.Database, sc *statistic.Collector) *DataLoader {
	return &DataLoader{
		Database:           db,
		StatisticCollector: sc,
	}
}

// GetAllStatisticSummaryInRange return all the statisics within the time frame. The second array is the key (dates) of the statistic
func (d *DataLoader) GetAllStatisticSummaryInRange(start, end string) ([]*statistic.DailySummaryExport, []string, error) {
	dailySummaries := []*statistic.DailySummaryExport{}
	collectedDates := []string{}
	//Generate all the dates in between the range
	keys, err := generateDateRange(start, end)
	if err != nil {
		return dailySummaries, collectedDates, err
	}

	//Load all the data from database
	for _, key := range keys {
		thisStat := statistic.DailySummaryExport{}
		err = d.Database.Read("stats", key, &thisStat)
		if err == nil {
			dailySummaries = append(dailySummaries, &thisStat)
			collectedDates = append(collectedDates, key)
		}
	}

	return dailySummaries, collectedDates, nil

}

func (d *DataLoader) GetStartAndEndDatesFromRequest(r *http.Request) (string, string, error) {
	// Get the start date from POST para
	start, err := utils.GetPara(r, "start")
	if err != nil {
		return "", "", errors.New("start date cannot be empty")
	}
	if strings.Contains(start, "-") {
		//Must be underscore
		start = strings.ReplaceAll(start, "-", "_")
	}
	// Get end date from POST para
	end, err := utils.GetPara(r, "end")
	if err != nil {
		return "", "", errors.New("end date cannot be empty")
	}
	if strings.Contains(end, "-") {
		//Must be underscore
		end = strings.ReplaceAll(end, "-", "_")
	}

	return start, end, nil
}
