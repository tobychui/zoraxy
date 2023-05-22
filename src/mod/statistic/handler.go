package statistic

import (
	"encoding/json"
	"net/http"

	"imuslab.com/zoraxy/mod/utils"
)

/*
	Handler.go

	This script handles incoming request for loading the statistic of the day

*/

func (c *Collector) HandleTodayStatLoad(w http.ResponseWriter, r *http.Request) {

	fast, err := utils.GetPara(r, "fast")
	if err != nil {
		fast = "false"
	}
	d := c.DailySummary
	if fast == "true" {
		//Only return the counter
		exported := DailySummaryExport{
			TotalRequest: d.TotalRequest,
			ErrorRequest: d.ErrorRequest,
			ValidRequest: d.ValidRequest,
		}
		js, _ := json.Marshal(exported)
		utils.SendJSONResponse(w, string(js))
	} else {
		//Return everything
		exported := c.GetExportSummary()
		js, _ := json.Marshal(exported)
		utils.SendJSONResponse(w, string(js))
	}

}
