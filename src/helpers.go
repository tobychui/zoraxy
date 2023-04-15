package main

import (
	"encoding/json"
	"net/http"

	"imuslab.com/zoraxy/mod/dynamicproxy"
	"imuslab.com/zoraxy/mod/uptime"
	"imuslab.com/zoraxy/mod/utils"
)

/*
	Statistic Summary
*/
//Handle conversion of statistic daily summary to country summary
func HandleCountryDistrSummary(w http.ResponseWriter, r *http.Request) {
	requestClientCountry := map[string]int{}
	statisticCollector.DailySummary.RequestClientIp.Range(func(key, value interface{}) bool {
		//Get this client country of original
		clientIp := key.(string)
		//requestCount := value.(int)

		ci, err := geodbStore.ResolveCountryCodeFromIP(clientIp)
		if err != nil {
			return true
		}

		isoCode := ci.CountryIsoCode
		if isoCode == "" {
			//local or reserved addr
			isoCode = "local"
		}
		uc, ok := requestClientCountry[isoCode]
		if !ok {
			//Create the counter
			requestClientCountry[isoCode] = 1
		} else {
			requestClientCountry[isoCode] = uc + 1
		}
		return true
	})

	js, _ := json.Marshal(requestClientCountry)
	utils.SendJSONResponse(w, string(js))
}

/*
	Up Time Monitor
*/
//Generate uptime monitor targets from reverse proxy rules
func GetUptimeTargetsFromReverseProxyRules(dp *dynamicproxy.Router) []*uptime.Target {
	subds := dp.GetSDProxyEndpointsAsMap()
	vdirs := dp.GetVDProxyEndpointsAsMap()

	UptimeTargets := []*uptime.Target{}
	for subd, target := range subds {
		url := "http://" + target.Domain
		protocol := "http"
		if target.RequireTLS {
			url = "https://" + target.Domain
			protocol = "https"
		}
		UptimeTargets = append(UptimeTargets, &uptime.Target{
			ID:       subd,
			Name:     subd,
			URL:      url,
			Protocol: protocol,
		})
	}

	for vdir, target := range vdirs {
		url := "http://" + target.Domain
		protocol := "http"
		if target.RequireTLS {
			url = "https://" + target.Domain
			protocol = "https"
		}
		UptimeTargets = append(UptimeTargets, &uptime.Target{
			ID:       vdir,
			Name:     "*" + vdir,
			URL:      url,
			Protocol: protocol,
		})
	}

	return UptimeTargets
}

//Handle rendering up time monitor data
func HandleUptimeMonitorListing(w http.ResponseWriter, r *http.Request) {
	if uptimeMonitor != nil {
		uptimeMonitor.HandleUptimeLogRead(w, r)
	} else {
		http.Error(w, "500 - Internal Server Error", http.StatusInternalServerError)
		return
	}
}
