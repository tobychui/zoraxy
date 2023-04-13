package main

import (
	"encoding/json"
	"net/http"
	"strconv"

	"imuslab.com/zoraxy/mod/utils"
)

/*
	Redirect.go

	This script handle all the http handlers
	related to redirection function in the reverse proxy
*/

func handleListRedirectionRules(w http.ResponseWriter, r *http.Request) {
	rules := redirectTable.GetAllRedirectRules()
	js, _ := json.Marshal(rules)
	utils.SendJSONResponse(w, string(js))
}

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

	redirectTypeString, err := utils.PostPara(r, "redirectType")
	if err != nil {
		redirectTypeString = "307"
	}

	redirectionStatusCode, err := strconv.Atoi(redirectTypeString)
	if err != nil {
		utils.SendErrorResponse(w, "invalid status code number")
		return
	}

	err = redirectTable.AddRedirectRule(redirectUrl, destUrl, forwardChildpath == "true", redirectionStatusCode)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

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
