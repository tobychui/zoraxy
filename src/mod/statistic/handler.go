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
		exported := DailySummaryExport{
			TotalRequest:    d.TotalRequest,
			ErrorRequest:    d.ErrorRequest,
			ValidRequest:    d.ValidRequest,
			ForwardTypes:    make(map[string]int),
			RequestOrigin:   make(map[string]int),
			RequestClientIp: make(map[string]int),
		}

		// Export ForwardTypes sync.Map
		d.ForwardTypes.Range(func(key, value interface{}) bool {
			exported.ForwardTypes[key.(string)] = value.(int)
			return true
		})

		// Export RequestOrigin sync.Map
		d.RequestOrigin.Range(func(key, value interface{}) bool {
			exported.RequestOrigin[key.(string)] = value.(int)
			return true
		})

		// Export RequestClientIp sync.Map
		d.RequestClientIp.Range(func(key, value interface{}) bool {
			exported.RequestClientIp[key.(string)] = value.(int)
			return true
		})

		js, _ := json.Marshal(exported)

		utils.SendJSONResponse(w, string(js))
	}

}
