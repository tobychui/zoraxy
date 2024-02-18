package main

import (
	"encoding/json"
	"net/http"

	strip "github.com/grokify/html-strip-tags-go"
	"imuslab.com/zoraxy/mod/geodb"
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
		currentEnabled := geodbStore.BlacklistEnabled
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

/*
	Whitelist Related
*/

func handleListWhitelisted(w http.ResponseWriter, r *http.Request) {
	bltype, err := utils.GetPara(r, "type")
	if err != nil {
		bltype = "country"
	}

	resulst := []*geodb.WhitelistEntry{}
	if bltype == "country" {
		resulst = geodbStore.GetAllWhitelistedCountryCode()
	} else if bltype == "ip" {
		resulst = geodbStore.GetAllWhitelistedIp()
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

	comment, _ := utils.PostPara(r, "comment")
	comment = strip.StripTags(comment)

	geodbStore.AddCountryCodeToWhitelist(countryCode, comment)

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

	comment, _ := utils.PostPara(r, "comment")
	comment = strip.StripTags(comment)

	geodbStore.AddIPToWhiteList(ipAddr, comment)
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
	} else {
		if enable == "true" {
			geodbStore.ToggleWhitelist(true)
		} else if enable == "false" {
			geodbStore.ToggleWhitelist(false)
		} else {
			utils.SendErrorResponse(w, "invalid enable state: only true and false is accepted")
			return
		}

		utils.SendOK(w)
	}
}
