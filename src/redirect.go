package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"imuslab.com/zoraxy/mod/utils"
)

/*
	Redirect.go

	This script handle all the http handlers
	related to redirection function in the reverse proxy
*/

// Handle request for listing all stored redirection rules
func handleListRedirectionRules(w http.ResponseWriter, r *http.Request) {
	rules := redirectTable.GetAllRedirectRules()
	js, _ := json.Marshal(rules)
	utils.SendJSONResponse(w, string(js))
}

// Handle request for adding new redirection rule
func handleAddRedirectionRule(w http.ResponseWriter, r *http.Request) {
	redirectUrl, err := utils.PostPara(r, "redirectUrl")
	if err != nil {
		utils.SendErrorResponse(w, "redirect url cannot be empty")
		return
	}
	destUrl, err := utils.PostPara(r, "destUrl")
	if err != nil {
		utils.SendErrorResponse(w, "destination url cannot be empty")
	}

	forwardChildpath, err := utils.PostPara(r, "forwardChildpath")
	if err != nil {
		//Assume true
		forwardChildpath = "true"
	}

	requireExactMatch, err := utils.PostPara(r, "requireExactMatch")
	if err != nil {
		//Assume false
		requireExactMatch = "false"
	}

	redirectTypeString, err := utils.PostPara(r, "redirectType")
	if err != nil {
		redirectTypeString = "307"
	}

	redirectionStatusCode, err := strconv.Atoi(redirectTypeString)
	if err != nil {
		utils.SendErrorResponse(w, "invalid status code number")
		return
	}

	err = redirectTable.AddRedirectRule(redirectUrl, destUrl, forwardChildpath == "true", redirectionStatusCode, requireExactMatch == "true")
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

// Handle remove of a given redirection rule
func handleDeleteRedirectionRule(w http.ResponseWriter, r *http.Request) {
	redirectUrl, err := utils.PostPara(r, "redirectUrl")
	if err != nil {
		utils.SendErrorResponse(w, "redirect url cannot be empty")
		return
	}

	err = redirectTable.DeleteRedirectRule(redirectUrl)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

func handleEditRedirectionRule(w http.ResponseWriter, r *http.Request) {
	originalRedirectUrl, err := utils.PostPara(r, "originalRedirectUrl")
	if err != nil {
		utils.SendErrorResponse(w, "original redirect url cannot be empty")
		return
	}

	newRedirectUrl, err := utils.PostPara(r, "newRedirectUrl")
	if err != nil {
		utils.SendErrorResponse(w, "redirect url cannot be empty")
		return
	}
	destUrl, err := utils.PostPara(r, "destUrl")
	if err != nil {
		utils.SendErrorResponse(w, "destination url cannot be empty")
	}

	forwardChildpath, err := utils.PostPara(r, "forwardChildpath")
	if err != nil {
		//Assume true
		forwardChildpath = "true"
	}

	requireExactMatch, err := utils.PostPara(r, "requireExactMatch")
	if err != nil {
		//Assume false
		requireExactMatch = "false"
	}

	redirectTypeString, err := utils.PostPara(r, "redirectType")
	if err != nil {
		redirectTypeString = "307"
	}

	redirectionStatusCode, err := strconv.Atoi(redirectTypeString)
	if err != nil {
		utils.SendErrorResponse(w, "invalid status code number")
		return
	}

	err = redirectTable.EditRedirectRule(originalRedirectUrl, newRedirectUrl, destUrl, forwardChildpath == "true", redirectionStatusCode, requireExactMatch == "true")
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

// Toggle redirection regex support. Note that this cost another O(n) time complexity to each page load
func handleToggleRedirectRegexpSupport(w http.ResponseWriter, r *http.Request) {
	enabled, err := utils.PostPara(r, "enable")
	if err != nil {
		//Return the current state of the regex support
		js, _ := json.Marshal(redirectTable.AllowRegex)
		utils.SendJSONResponse(w, string(js))
		return
	}

	//Update the current regex support rule enable state
	enableRegexSupport := strings.EqualFold(strings.TrimSpace(enabled), "true")
	redirectTable.AllowRegex = enableRegexSupport
	err = sysdb.Write("redirect", "regex", enableRegexSupport)

	if enableRegexSupport {
		SystemWideLogger.PrintAndLog("redirect", "Regex redirect rule enabled", nil)
	} else {
		SystemWideLogger.PrintAndLog("redirect", "Regex redirect rule disabled", nil)
	}
	if err != nil {
		utils.SendErrorResponse(w, "unable to save settings")
		return
	}
	utils.SendOK(w)
}

// Toggle redirection case sensitivity. Note that this affects all redirection rules
func handleToggleRedirectCaseSensitivity(w http.ResponseWriter, r *http.Request) {
	enabled, err := utils.PostPara(r, "enable")
	if err != nil {
		//Return the current state of the case sensitivity
		js, _ := json.Marshal(redirectTable.CaseSensitive)
		utils.SendJSONResponse(w, string(js))
		return
	}

	//Update the current case sensitivity rule enable state
	enableCaseSensitivity := strings.EqualFold(strings.TrimSpace(enabled), "true")
	redirectTable.CaseSensitive = enableCaseSensitivity
	err = sysdb.Write("redirect", "case_sensitive", enableCaseSensitivity)

	if enableCaseSensitivity {
		SystemWideLogger.PrintAndLog("redirect", "Case sensitive redirect rule enabled", nil)
	} else {
		SystemWideLogger.PrintAndLog("redirect", "Case sensitive redirect rule disabled", nil)
	}
	if err != nil {
		utils.SendErrorResponse(w, "unable to save settings")
		return
	}
	utils.SendOK(w)
}
