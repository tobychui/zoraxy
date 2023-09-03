package main

import (
	"encoding/json"
	"log"
	"net/http"

	"imuslab.com/zoraxy/mod/utils"
)

/*
	accesslist.go

	This script file is added to extend the
	reverse proxy function to include
	banning / whitelist a specific IP address or country code
*/

/*
	Blacklist Related
*/

// List a of blacklisted ip address or country code
func handleListBlacklisted(w http.ResponseWriter, r *http.Request) {
	bltype, err := utils.GetPara(r, "type")
	if err != nil {
		log.Println("invalid or empty blacklist type, default to country")
	}

	resulst := []string{}
	switch bltype {
	case "country":
		resulst = geodbStore.GetAllBlacklistedCountryCode()
	case "ip":
		resulst = geodbStore.GetAllBlacklistedIp()
	default:
		resulst = geodbStore.GetAllBlacklistedCountryCode()
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
		currentEnabled := geodbStore.BlacklistEnabled
		js, _ := json.Marshal(currentEnabled)
		utils.SendJSONResponse(w, string(js))
		return
	}
	switch enable {
	case "true":
		geodbStore.ToggleBlacklist(true)
	case "false":
		geodbStore.ToggleBlacklist(false)
	default:
		utils.SendErrorResponse(w, "invalid enable state: only true and false is accepted")
		return
	}

	utils.SendOK(w)
}

/*
	Whitelist Related
*/

func handleListWhitelisted(w http.ResponseWriter, r *http.Request) {
	wltype, err := utils.GetPara(r, "type")
	if err != nil {
		log.Println("invalid or empty whitelist type, default to country")
	}

	resulst := []string{}
	switch wltype {
	case "country":
		resulst = geodbStore.GetAllWhitelistedCountryCode()
	case "ip":
		resulst = geodbStore.GetAllWhitelistedIp()
	default:
		resulst = geodbStore.GetAllWhitelistedCountryCode()
	}

	js, _ := json.Marshal(resulst)
	utils.SendJSONResponse(w, string(js))

}

func handleCountryWhitelistAdd(w http.ResponseWriter, r *http.Request) {
	countryCode, err := utils.PostPara(r, "cc")
	if err != nil {
		utils.SendErrorResponse(w, "invalid or empty country code")
		return
	}

	geodbStore.AddCountryCodeToWhitelist(countryCode)

	utils.SendOK(w)
}

func handleCountryWhitelistRemove(w http.ResponseWriter, r *http.Request) {
	countryCode, err := utils.PostPara(r, "cc")
	if err != nil {
		utils.SendErrorResponse(w, "invalid or empty country code")
		return
	}

	geodbStore.RemoveCountryCodeFromWhitelist(countryCode)

	utils.SendOK(w)
}

func handleIpWhitelistAdd(w http.ResponseWriter, r *http.Request) {
	ipAddr, err := utils.PostPara(r, "ip")
	if err != nil {
		utils.SendErrorResponse(w, "invalid or empty ip address")
		return
	}

	geodbStore.AddIPToWhiteList(ipAddr)
}

func handleIpWhitelistRemove(w http.ResponseWriter, r *http.Request) {
	ipAddr, err := utils.PostPara(r, "ip")
	if err != nil {
		utils.SendErrorResponse(w, "invalid or empty ip address")
		return
	}

	geodbStore.RemoveIPFromWhiteList(ipAddr)

	utils.SendOK(w)
}

func handleWhitelistEnable(w http.ResponseWriter, r *http.Request) {
	enable, err := utils.PostPara(r, "enable")
	if err != nil {
		//Return the current enabled state
		currentEnabled := geodbStore.WhitelistEnabled
		js, _ := json.Marshal(currentEnabled)
		utils.SendJSONResponse(w, string(js))
		return
	}
	switch enable {
	case "true":
		geodbStore.ToggleWhitelist(true)
	case "false":
		geodbStore.ToggleWhitelist(false)
	default:
		utils.SendErrorResponse(w, "invalid enable state: only true and false is accepted")
		return
	}

	utils.SendOK(w)
}
