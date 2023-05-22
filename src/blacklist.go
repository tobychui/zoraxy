package main

import (
	"encoding/json"
	"net/http"

	"imuslab.com/zoraxy/mod/utils"
)

/*
	blacklist.go

	This script file is added to extend the
	reverse proxy function to include
	banning a specific IP address or country code
*/

//List a of blacklisted ip address or country code
func handleListBlacklisted(w http.ResponseWriter, r *http.Request) {
	bltype, err := utils.GetPara(r, "type")
	if err != nil {
		bltype = "country"
	}

	resulst := []string{}
	if bltype == "country" {
		resulst = geodbStore.GetAllBlacklistedCountryCode()
	} else if bltype == "ip" {
		resulst = geodbStore.GetAllBlacklistedIp()
	}

	js, _ := json.Marshal(resulst)
	utils.SendJSONResponse(w, string(js))

}

func handleCountryBlacklistAdd(w http.ResponseWriter, r *http.Request) {
	countryCode, err := utils.PostPara(r, "cc")
	if err != nil {
		utils.SendErrorResponse(w, "invalid or empty country code")
		return
	}

	geodbStore.AddCountryCodeToBlackList(countryCode)

	utils.SendOK(w)
}

func handleCountryBlacklistRemove(w http.ResponseWriter, r *http.Request) {
	countryCode, err := utils.PostPara(r, "cc")
	if err != nil {
		utils.SendErrorResponse(w, "invalid or empty country code")
		return
	}

	geodbStore.RemoveCountryCodeFromBlackList(countryCode)

	utils.SendOK(w)
}

func handleIpBlacklistAdd(w http.ResponseWriter, r *http.Request) {
	ipAddr, err := utils.PostPara(r, "ip")
	if err != nil {
		utils.SendErrorResponse(w, "invalid or empty ip address")
		return
	}

	geodbStore.AddIPToBlackList(ipAddr)
}

func handleIpBlacklistRemove(w http.ResponseWriter, r *http.Request) {
	ipAddr, err := utils.PostPara(r, "ip")
	if err != nil {
		utils.SendErrorResponse(w, "invalid or empty ip address")
		return
	}

	geodbStore.RemoveIPFromBlackList(ipAddr)

	utils.SendOK(w)
}

func handleBlacklistEnable(w http.ResponseWriter, r *http.Request) {
	enable, err := utils.PostPara(r, "enable")
	if err != nil {
		//Return the current enabled state
		currentEnabled := geodbStore.Enabled
		js, _ := json.Marshal(currentEnabled)
		utils.SendJSONResponse(w, string(js))
	} else {
		if enable == "true" {
			geodbStore.ToggleBlacklist(true)
		} else if enable == "false" {
			geodbStore.ToggleBlacklist(false)
		} else {
			utils.SendErrorResponse(w, "invalid enable state: only true and false is accepted")
			return
		}

		utils.SendOK(w)
	}
}
