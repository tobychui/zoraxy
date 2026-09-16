package main

import (
	"encoding/json"
	"net/http"

	"imuslab.com/zoraxy/mod/utils"
)

/*
	geodbupdate.go

	This script handles the web UI facing APIs for the Geo-IP database
	manual update and the scheduled (weekly) update, see mod/geodb/updater.go
*/

// HandleGeoDBUpdateStatus return the current Geo-IP database and updater status
func HandleGeoDBUpdateStatus(w http.ResponseWriter, r *http.Request) {
	js, _ := json.Marshal(geodbStore.GetUpdaterStatus())
	utils.SendJSONResponse(w, string(js))
}

// HandleGeoDBUpdateNow trigger a manual Geo-IP database update. The updated
// database is hot reloaded into the running instance, no restart required
func HandleGeoDBUpdateNow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "Method not allowed")
		return
	}

	err := geodbStore.UpdateGeoDBNow()
	if err != nil {
		SystemWideLogger.PrintAndLog("GeoDB", "Geo-IP database update failed", err)
		utils.SendErrorResponse(w, err.Error())
		return
	}

	SystemWideLogger.PrintAndLog("GeoDB", "Geo-IP database updated by administrator", nil)
	utils.SendOK(w)
}

// HandleGeoDBAutoUpdate get (GET) or set (POST with enable=true/false) the
// scheduled Geo-IP database update setting
func HandleGeoDBAutoUpdate(w http.ResponseWriter, r *http.Request) {
	enable, err := utils.PostBool(r, "enable")
	if err != nil {
		//Not a set request. Return the current setting
		js, _ := json.Marshal(geodbStore.GetUpdaterStatus().AutoUpdateEnabled)
		utils.SendJSONResponse(w, string(js))
		return
	}

	err = geodbStore.SetGeoDBAutoUpdate(enable)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	if enable {
		SystemWideLogger.PrintAndLog("GeoDB", "Scheduled Geo-IP database update enabled", nil)
	} else {
		SystemWideLogger.PrintAndLog("GeoDB", "Scheduled Geo-IP database update disabled", nil)
	}
	utils.SendOK(w)
}
