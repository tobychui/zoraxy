package analytic

import (
	"encoding/csv"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"imuslab.com/zoraxy/mod/statistic"
	"imuslab.com/zoraxy/mod/utils"
)

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
	start, end, err := d.GetStartAndEndDatesFromRequest(r)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	dailySummaries, _, err := d.GetAllStatisticSummaryInRange(start, end)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
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

// Handle exporting of a given range statistics
func (d *DataLoader) HandleRangeExport(w http.ResponseWriter, r *http.Request) {
	start, end, err := d.GetStartAndEndDatesFromRequest(r)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	dailySummaries, dates, err := d.GetAllStatisticSummaryInRange(start, end)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	format, err := utils.GetPara(r, "format")
	if err != nil {
		format = "json"
	}

	if format == "csv" {
		// Create a buffer to store CSV content
		var csvContent strings.Builder

		// Create a CSV writer
		writer := csv.NewWriter(&csvContent)

		// Write the header row
		header := []string{"Date", "TotalRequest", "ErrorRequest", "ValidRequest", "ForwardTypes", "RequestOrigin", "RequestClientIp", "Referer", "UserAgent", "RequestURL"}
		err := writer.Write(header)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Write each data row
		for i, item := range dailySummaries {
			row := []string{
				dates[i],
				strconv.FormatInt(item.TotalRequest, 10),
				strconv.FormatInt(item.ErrorRequest, 10),
				strconv.FormatInt(item.ValidRequest, 10),
				// Convert map values to a comma-separated string
				strings.Join(mapToStringSlice(item.ForwardTypes), ","),
				strings.Join(mapToStringSlice(item.RequestOrigin), ","),
				strings.Join(mapToStringSlice(item.RequestClientIp), ","),
				strings.Join(mapToStringSlice(item.Referer), ","),
				strings.Join(mapToStringSlice(item.UserAgent), ","),
				strings.Join(mapToStringSlice(item.RequestURL), ","),
			}
			err = writer.Write(row)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		// Flush the CSV writer
		writer.Flush()

		// Check for any errors during writing
		if err := writer.Error(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Set the response headers
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=analytics_"+start+"_to_"+end+".csv")

		// Write the CSV content to the response writer
		_, err = w.Write([]byte(csvContent.String()))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else if format == "json" {
		type exportData struct {
			Stats []*statistic.DailySummaryExport
			Dates []string
		}

		results := exportData{
			Stats: dailySummaries,
			Dates: dates,
		}

		js, _ := json.MarshalIndent(results, "", " ")
		w.Header().Set("Content-Disposition", "attachment; filename=analytics_"+start+"_to_"+end+".json")
		utils.SendJSONResponse(w, string(js))
	} else {
		utils.SendErrorResponse(w, "Unsupported export format")
	}
}

// Reset all the keys within the given time period
func (d *DataLoader) HandleRangeReset(w http.ResponseWriter, r *http.Request) {
	start, end, err := d.GetStartAndEndDatesFromRequest(r)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	keys, err := generateDateRange(start, end)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	for _, key := range keys {
		log.Println("DELETING statistics " + key)
		d.Database.Delete("stats", key)

		if isTodayDate(key) {
			//It is today's date. Also reset statistic collector value
			log.Println("RESETING today's in-memory statistics")
			d.StatisticCollector.ResetSummaryOfDay()
		}
	}

	utils.SendOK(w)
}

// Reset all statistics from the system
func (d *DataLoader) HandleResetAllStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	entries, err := d.Database.ListTable("stats")
	if err != nil {
		utils.SendErrorResponse(w, "unable to load data from database")
		return
	}

	for _, keypairs := range entries {
		key := string(keypairs[0])
		log.Println("DELETING statistics " + key)
		d.Database.Delete("stats", key)
	}

	//Also reset the in-memory statistic collector
	log.Println("RESETING in-memory statistics")
	d.StatisticCollector.ResetSummaryOfDay()

	utils.SendOK(w)
}
