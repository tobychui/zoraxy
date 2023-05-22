package analytic

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

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

func (d *DataLoader) HandleSummaryList(w http.ResponseWriter, r *http.Request) {
	entries, err := d.Database.ListTable("stats")
	if err != nil {
		utils.SendErrorResponse(w, "unable to load data from database")
		return
	}

	entryDates := []string{}
	for _, keypairs := range entries {
		entryDates = append(entryDates, string(keypairs[0]))
	}

	js, _ := json.MarshalIndent(entryDates, "", " ")
	utils.SendJSONResponse(w, string(js))
}

func (d *DataLoader) HandleLoadTargetDaySummary(w http.ResponseWriter, r *http.Request) {
	day, err := utils.GetPara(r, "id")
	if err != nil {
		utils.SendErrorResponse(w, "id cannot be empty")
		return
	}

	if strings.Contains(day, "-") {
		//Must be underscore
		day = strings.ReplaceAll(day, "-", "_")
	}

	if !statistic.IsBeforeToday(day) {
		utils.SendErrorResponse(w, "given date is in the future")
		return
	}

	var targetDailySummary statistic.DailySummaryExport

	if day == time.Now().Format("2006_01_02") {
		targetDailySummary = *d.StatisticCollector.GetExportSummary()
	} else {
		//Not today data
		err = d.Database.Read("stats", day, &targetDailySummary)
		if err != nil {
			utils.SendErrorResponse(w, "target day data not found")
			return
		}
	}

	js, _ := json.Marshal(targetDailySummary)
	utils.SendJSONResponse(w, string(js))
}

func (d *DataLoader) HandleLoadTargetRangeSummary(w http.ResponseWriter, r *http.Request) {
	//Get the start date from POST para
	start, err := utils.GetPara(r, "start")
	if err != nil {
		utils.SendErrorResponse(w, "start date cannot be empty")
		return
	}
	if strings.Contains(start, "-") {
		//Must be underscore
		start = strings.ReplaceAll(start, "-", "_")
	}
	//Get end date from POST para
	end, err := utils.GetPara(r, "end")
	if err != nil {
		utils.SendErrorResponse(w, "emd date cannot be empty")
		return
	}
	if strings.Contains(end, "-") {
		//Must be underscore
		end = strings.ReplaceAll(end, "-", "_")
	}

	//Generate all the dates in between the range
	keys, err := generateDateRange(start, end)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Load all the data from database
	dailySummaries := []*statistic.DailySummaryExport{}
	for _, key := range keys {
		thisStat := statistic.DailySummaryExport{}
		err = d.Database.Read("stats", key, &thisStat)
		if err == nil {
			dailySummaries = append(dailySummaries, &thisStat)
		}
	}

	//Merge the summaries into one
	mergedSummary := mergeDailySummaryExports(dailySummaries)

	js, _ := json.Marshal(struct {
		Summary *statistic.DailySummaryExport
		Records []*statistic.DailySummaryExport
	}{
		Summary: mergedSummary,
		Records: dailySummaries,
	})

	utils.SendJSONResponse(w, string(js))
}
