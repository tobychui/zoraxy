package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/microcosm-cc/bluemonday"

	"imuslab.com/zoraxy/mod/access"
	"imuslab.com/zoraxy/mod/plugins"
	"imuslab.com/zoraxy/mod/plugins/zoraxy_plugin"
	"imuslab.com/zoraxy/mod/utils"
)

/*
	accesslist.go

	This script file is added to extend the
	reverse proxy function to include
	banning / whitelist a specific IP address or country code
*/

/*
	General Function
*/

func handleListAccessRules(w http.ResponseWriter, r *http.Request) {
	allAccessRules := accessController.ListAllAccessRules()
	js, _ := json.Marshal(allAccessRules)
	utils.SendJSONResponse(w, string(js))
}

func handleAttachRuleToHost(w http.ResponseWriter, r *http.Request) {
	ruleid, err := utils.PostPara(r, "id")
	if err != nil {
		utils.SendErrorResponse(w, "invalid rule name")
		return
	}

	host, err := utils.PostPara(r, "host")
	if err != nil {
		utils.SendErrorResponse(w, "invalid rule name")
		return
	}

	//Check if access rule and proxy rule exists
	targetProxyEndpoint, err := dynamicProxyRouter.LoadProxy(host)
	if err != nil {
		utils.SendErrorResponse(w, "invalid host given")
		return
	}
	if !accessController.AccessRuleExists(ruleid) {
		utils.SendErrorResponse(w, "access rule not exists")
		return
	}

	//Update the proxy host acess rule id
	targetProxyEndpoint.AccessFilterUUID = ruleid
	targetProxyEndpoint.UpdateToRuntime()
	err = SaveReverseProxyConfig(targetProxyEndpoint)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

// Create a new access rule, require name and desc only
func handleCreateAccessRule(w http.ResponseWriter, r *http.Request) {
	ruleName, err := utils.PostPara(r, "name")
	if err != nil {
		utils.SendErrorResponse(w, "invalid rule name")
		return
	}
	ruleDesc, _ := utils.PostPara(r, "desc")

	//Filter out injection if any
	p := bluemonday.StripTagsPolicy()
	ruleName = p.Sanitize(ruleName)
	ruleDesc = p.Sanitize(ruleDesc)

	ruleUUID := uuid.New().String()
	newAccessRule := access.AccessRule{
		ID:               ruleUUID,
		Name:             ruleName,
		Desc:             ruleDesc,
		BlacklistEnabled: false,
		WhitelistEnabled: false,
	}

	//Add it to runtime
	err = accessController.AddNewAccessRule(&newAccessRule)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	// emit an event for the new access rule creation
	plugins.EventSystem.Emit(
		&zoraxy_plugin.AccessRuleCreatedEvent{
			ID:               ruleUUID,
			Name:             ruleName,
			Desc:             ruleDesc,
			BlacklistEnabled: false,
			WhitelistEnabled: false,
		},
	)

	utils.SendOK(w)
}

// Handle removing an access rule. All proxy endpoint using this rule will be
// set to use the default rule
func handleRemoveAccessRule(w http.ResponseWriter, r *http.Request) {
	ruleID, err := utils.PostPara(r, "id")
	if err != nil {
		utils.SendErrorResponse(w, "invalid rule id given")
		return
	}

	if ruleID == "default" {
		utils.SendErrorResponse(w, "default access rule cannot be removed")
		return
	}

	ruleID = strings.TrimSpace(ruleID)

	//Set all proxy hosts that use this access rule back to using "default"
	allProxyEndpoints := dynamicProxyRouter.GetProxyEndpointsAsMap()
	for _, proxyEndpoint := range allProxyEndpoints {
		if strings.EqualFold(proxyEndpoint.AccessFilterUUID, ruleID) {
			//This proxy endpoint is using the current access filter.
			//set it to default
			proxyEndpoint.AccessFilterUUID = "default"
			proxyEndpoint.UpdateToRuntime()
			err = SaveReverseProxyConfig(proxyEndpoint)
			if err != nil {
				SystemWideLogger.PrintAndLog("Access", "Unable to save updated proxy endpoint "+proxyEndpoint.RootOrMatchingDomain, err)
			} else {
				SystemWideLogger.PrintAndLog("Access", "Updated "+proxyEndpoint.RootOrMatchingDomain+" access filter to \"default\"", nil)
			}
		}
	}

	//Remove the access rule by ID
	err = accessController.RemoveAccessRuleByID(ruleID)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	SystemWideLogger.PrintAndLog("Access", "Access Rule "+ruleID+" removed", nil)
	utils.SendOK(w)
}

// Only the name and desc, for other properties use blacklist / whitelist api
func handleUpadateAccessRule(w http.ResponseWriter, r *http.Request) {
	ruleID, err := utils.PostPara(r, "id")
	if err != nil {
		utils.SendErrorResponse(w, "invalid rule id")
		return
	}
	ruleName, err := utils.PostPara(r, "name")
	if err != nil {
		utils.SendErrorResponse(w, "invalid rule name")
		return
	}
	ruleDesc, _ := utils.PostPara(r, "desc")

	//Filter anything weird
	p := bluemonday.StrictPolicy()
	ruleName = p.Sanitize(ruleName)
	ruleDesc = p.Sanitize(ruleDesc)

	err = accessController.UpdateAccessRule(ruleID, ruleName, ruleDesc)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

/*
	Blacklist Related
*/

// List a of blacklisted ip address or country code
func handleListBlacklisted(w http.ResponseWriter, r *http.Request) {
	bltype, err := utils.GetPara(r, "type")
	if err != nil {
		bltype = "country"
	}

	ruleID, err := utils.GetPara(r, "id")
	if err != nil {
		//Use default if not set
		ruleID = "default"
	}

	//Load the target rule from access controller
	rule, err := accessController.GetAccessRuleByID(ruleID)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	resulst := []string{}
	if bltype == "country" {
		resulst = rule.GetAllBlacklistedCountryCode()
	} else if bltype == "ip" {
		resulst = rule.GetAllBlacklistedIp()
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

	ruleID, err := utils.PostPara(r, "id")
	if err != nil {
		ruleID = "default"
	}

	comment, _ := utils.PostPara(r, "comment")
	p := bluemonday.StripTagsPolicy()
	comment = p.Sanitize(comment)

	//Load the target rule from access controller
	rule, err := accessController.GetAccessRuleByID(ruleID)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Check if the country code contains comma, if yes, split it
	if strings.Contains(countryCode, ",") {
		codes := strings.Split(countryCode, ",")
		for _, code := range codes {
			code = strings.TrimSpace(code)
			rule.AddCountryCodeToBlackList(code, comment)
		}
	} else {
		countryCode = strings.TrimSpace(countryCode)
		rule.AddCountryCodeToBlackList(countryCode, comment)
	}

	utils.SendOK(w)
}

func handleCountryBlacklistRemove(w http.ResponseWriter, r *http.Request) {
	countryCode, err := utils.PostPara(r, "cc")
	if err != nil {
		utils.SendErrorResponse(w, "invalid or empty country code")
		return
	}

	ruleID, err := utils.PostPara(r, "id")
	if err != nil {
		ruleID = "default"
	}

	//Load the target rule from access controller
	rule, err := accessController.GetAccessRuleByID(ruleID)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Check if the country code contains comma, if yes, split it
	if strings.Contains(countryCode, ",") {
		codes := strings.Split(countryCode, ",")
		for _, code := range codes {
			code = strings.TrimSpace(code)
			rule.RemoveCountryCodeFromBlackList(code)
		}
	} else {
		countryCode = strings.TrimSpace(countryCode)
		rule.RemoveCountryCodeFromBlackList(countryCode)
	}

	utils.SendOK(w)
}

func handleIpBlacklistAdd(w http.ResponseWriter, r *http.Request) {
	ipAddr, err := utils.PostPara(r, "ip")
	if err != nil {
		utils.SendErrorResponse(w, "invalid or empty ip address")
		return
	}

	ruleID, err := utils.PostPara(r, "id")
	if err != nil {
		ruleID = "default"
	}

	//Load the target rule from access controller
	rule, err := accessController.GetAccessRuleByID(ruleID)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	comment, _ := utils.GetPara(r, "comment")
	p := bluemonday.StripTagsPolicy()
	comment = p.Sanitize(comment)

	rule.AddIPToBlackList(ipAddr, comment)
	utils.SendOK(w)
}

func handleIpBlacklistRemove(w http.ResponseWriter, r *http.Request) {
	ipAddr, err := utils.PostPara(r, "ip")
	if err != nil {
		utils.SendErrorResponse(w, "invalid or empty ip address")
		return
	}

	ruleID, err := utils.PostPara(r, "id")
	if err != nil {
		ruleID = "default"
	}

	//Load the target rule from access controller
	rule, err := accessController.GetAccessRuleByID(ruleID)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	rule.RemoveIPFromBlackList(ipAddr)

	utils.SendOK(w)
}

func handleBlacklistEnable(w http.ResponseWriter, r *http.Request) {
	enable, _ := utils.PostPara(r, "enable")
	ruleID, err := utils.PostPara(r, "id")
	if err != nil {
		ruleID = "default"
	}

	rule, err := accessController.GetAccessRuleByID(ruleID)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	if enable == "" {
		//enable paramter not set
		currentEnabled := rule.BlacklistEnabled
		js, _ := json.Marshal(currentEnabled)
		utils.SendJSONResponse(w, string(js))
	} else {
		if enable == "true" {
			rule.ToggleBlacklist(true)
		} else if enable == "false" {
			rule.ToggleBlacklist(false)
		} else {
			utils.SendErrorResponse(w, "invalid enable state: only true and false is accepted")
			return
		}

		plugins.EventSystem.Emit(&zoraxy_plugin.BlacklistToggledEvent{
			RuleID:  ruleID,
			Enabled: rule.BlacklistEnabled,
		})

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

	ruleID, err := utils.GetPara(r, "id")
	if err != nil {
		ruleID = "default"
	}

	rule, err := accessController.GetAccessRuleByID(ruleID)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	resulst := []*access.WhitelistEntry{}
	if bltype == "country" {
		resulst = rule.GetAllWhitelistedCountryCode()
	} else if bltype == "ip" {
		resulst = rule.GetAllWhitelistedIp()
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

	ruleID, err := utils.PostPara(r, "id")
	if err != nil {
		ruleID = "default"
	}

	rule, err := accessController.GetAccessRuleByID(ruleID)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	comment, _ := utils.PostPara(r, "comment")
	p := bluemonday.StrictPolicy()
	comment = p.Sanitize(comment)

	//Check if the country code contains comma, if yes, split it
	if strings.Contains(countryCode, ",") {
		codes := strings.Split(countryCode, ",")
		for _, code := range codes {
			code = strings.TrimSpace(code)
			rule.AddCountryCodeToWhitelist(code, comment)
		}
	} else {
		countryCode = strings.TrimSpace(countryCode)
		rule.AddCountryCodeToWhitelist(countryCode, comment)
	}

	utils.SendOK(w)
}

func handleCountryWhitelistRemove(w http.ResponseWriter, r *http.Request) {
	countryCode, err := utils.PostPara(r, "cc")
	if err != nil {
		utils.SendErrorResponse(w, "invalid or empty country code")
		return
	}

	ruleID, err := utils.PostPara(r, "id")
	if err != nil {
		ruleID = "default"
	}

	rule, err := accessController.GetAccessRuleByID(ruleID)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Check if the country code contains comma, if yes, split it
	if strings.Contains(countryCode, ",") {
		codes := strings.Split(countryCode, ",")
		for _, code := range codes {
			code = strings.TrimSpace(code)
			rule.RemoveCountryCodeFromWhitelist(code)
		}
	} else {
		countryCode = strings.TrimSpace(countryCode)
		rule.RemoveCountryCodeFromWhitelist(countryCode)
	}

	utils.SendOK(w)
}

func handleIpWhitelistAdd(w http.ResponseWriter, r *http.Request) {
	ipAddr, err := utils.PostPara(r, "ip")
	if err != nil {
		utils.SendErrorResponse(w, "invalid or empty ip address")
		return
	}

	ruleID, err := utils.PostPara(r, "id")
	if err != nil {
		ruleID = "default"
	}

	rule, err := accessController.GetAccessRuleByID(ruleID)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	comment, _ := utils.PostPara(r, "comment")
	p := bluemonday.StrictPolicy()
	comment = p.Sanitize(comment)

	rule.AddIPToWhiteList(ipAddr, comment)
	utils.SendOK(w)
}

func handleIpWhitelistRemove(w http.ResponseWriter, r *http.Request) {
	ipAddr, err := utils.PostPara(r, "ip")
	if err != nil {
		utils.SendErrorResponse(w, "invalid or empty ip address")
		return
	}

	ruleID, err := utils.PostPara(r, "id")
	if err != nil {
		ruleID = "default"
	}

	rule, err := accessController.GetAccessRuleByID(ruleID)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	rule.RemoveIPFromWhiteList(ipAddr)

	utils.SendOK(w)
}

func handleWhitelistEnable(w http.ResponseWriter, r *http.Request) {
	enable, _ := utils.PostPara(r, "enable")
	ruleID, err := utils.PostPara(r, "id")
	if err != nil {
		ruleID = "default"
	}

	rule, err := accessController.GetAccessRuleByID(ruleID)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	if enable == "" {
		//Return the current enabled state
		currentEnabled := rule.WhitelistEnabled
		js, _ := json.Marshal(currentEnabled)
		utils.SendJSONResponse(w, string(js))
	} else {
		if enable == "true" {
			rule.ToggleWhitelist(true)
		} else if enable == "false" {
			rule.ToggleWhitelist(false)
		} else {
			utils.SendErrorResponse(w, "invalid enable state: only true and false is accepted")
			return
		}

		utils.SendOK(w)
	}
}

func handleWhitelistAllowLoopback(w http.ResponseWriter, r *http.Request) {
	enable, _ := utils.PostPara(r, "enable")
	ruleID, err := utils.PostPara(r, "id")
	if err != nil {
		ruleID = "default"
	}

	rule, err := accessController.GetAccessRuleByID(ruleID)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	if enable == "" {
		//Return the current enabled state
		currentEnabled := rule.WhitelistAllowLocalAndLoopback
		js, _ := json.Marshal(currentEnabled)
		utils.SendJSONResponse(w, string(js))
	} else {
		if enable == "true" {
			rule.ToggleAllowLoopback(true)
		} else if enable == "false" {
			rule.ToggleAllowLoopback(false)
		} else {
			utils.SendErrorResponse(w, "invalid enable state: only true and false is accepted")
			return
		}

		utils.SendOK(w)
	}
}

// List all quick ban ip address
func handleListQuickBan(w http.ResponseWriter, r *http.Request) {
	currentSummary := statisticCollector.GetCurrentDailySummary()
	type quickBanEntry struct {
		IpAddr      string
		Count       int
		CountryCode string
	}
	result := []quickBanEntry{}
	currentSummary.RequestClientIp.Range(func(key, value interface{}) bool {
		ip := key.(string)
		count := value.(int)
		thisEntry := quickBanEntry{
			IpAddr: ip,
			Count:  count,
		}

		//Get the country code
		geoinfo, err := geodbStore.ResolveCountryCodeFromIP(ip)
		if err == nil {
			thisEntry.CountryCode = geoinfo.CountryIsoCode
		}

		result = append(result, thisEntry)
		return true
	})

	//Sort result based on count
	sort.Slice(result, func(i, j int) bool {
		return result[i].Count > result[j].Count
	})

	js, _ := json.Marshal(result)
	utils.SendJSONResponse(w, string(js))
}
