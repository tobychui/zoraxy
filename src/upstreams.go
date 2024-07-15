package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"imuslab.com/zoraxy/mod/dynamicproxy/loadbalance"
	"imuslab.com/zoraxy/mod/utils"
)

/*
	Upstreams.go

	This script handle upstream and load balancer
	related API
*/

// List upstreams from a endpoint
func ReverseProxyUpstreamList(w http.ResponseWriter, r *http.Request) {
	endpoint, err := utils.PostPara(r, "ep")
	if err != nil {
		utils.SendErrorResponse(w, "endpoint not defined")
		return
	}

	targetEndpoint, err := dynamicProxyRouter.LoadProxy(endpoint)
	if err != nil {
		utils.SendErrorResponse(w, "target endpoint not found")
		return
	}

	activeUpstreams := targetEndpoint.ActiveOrigins
	inactiveUpstreams := targetEndpoint.InactiveOrigins
	// Sort the upstreams slice by weight, then by origin domain alphabetically
	sort.Slice(activeUpstreams, func(i, j int) bool {
		if activeUpstreams[i].Weight != activeUpstreams[j].Weight {
			return activeUpstreams[i].Weight > activeUpstreams[j].Weight
		}
		return activeUpstreams[i].OriginIpOrDomain < activeUpstreams[j].OriginIpOrDomain
	})

	sort.Slice(inactiveUpstreams, func(i, j int) bool {
		if inactiveUpstreams[i].Weight != inactiveUpstreams[j].Weight {
			return inactiveUpstreams[i].Weight > inactiveUpstreams[j].Weight
		}
		return inactiveUpstreams[i].OriginIpOrDomain < inactiveUpstreams[j].OriginIpOrDomain
	})

	type UpstreamCombinedList struct {
		ActiveOrigins   []*loadbalance.Upstream
		InactiveOrigins []*loadbalance.Upstream
	}

	js, _ := json.Marshal(UpstreamCombinedList{
		ActiveOrigins:   activeUpstreams,
		InactiveOrigins: inactiveUpstreams,
	})
	utils.SendJSONResponse(w, string(js))
}

// Add an upstream to a given proxy upstream endpoint
func ReverseProxyUpstreamAdd(w http.ResponseWriter, r *http.Request) {
	endpoint, err := utils.PostPara(r, "ep")
	if err != nil {
		utils.SendErrorResponse(w, "endpoint not defined")
		return
	}

	targetEndpoint, err := dynamicProxyRouter.LoadProxy(endpoint)
	if err != nil {
		utils.SendErrorResponse(w, "target endpoint not found")
		return
	}

	upstreamOrigin, err := utils.PostPara(r, "origin")
	if err != nil {
		utils.SendErrorResponse(w, "upstream origin not set")
		return
	}
	requireTLS, _ := utils.PostBool(r, "tls")
	skipTlsValidation, _ := utils.PostBool(r, "tlsval")
	bpwsorg, _ := utils.PostBool(r, "bpwsorg")
	preactivate, _ := utils.PostBool(r, "active")

	//Create a new upstream object
	newUpstream := loadbalance.Upstream{
		OriginIpOrDomain:         upstreamOrigin,
		RequireTLS:               requireTLS,
		SkipCertValidations:      skipTlsValidation,
		SkipWebSocketOriginCheck: bpwsorg,
		Weight:                   1,
		MaxConn:                  0,
	}

	//Add the new upstream to endpoint
	err = targetEndpoint.AddUpstreamOrigin(&newUpstream, preactivate)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Save changes to configs
	err = SaveReverseProxyConfig(targetEndpoint)
	if err != nil {
		SystemWideLogger.PrintAndLog("INFO", "Unable to save new upstream to proxy config", err)
		utils.SendErrorResponse(w, "Failed to save new upstream config")
		return
	}

	//Update Uptime Monitor
	UpdateUptimeMonitorTargets()

	utils.SendOK(w)
}

// Update the connection configuration of this origin
// pass in the whole new upstream origin json via "payload" POST variable
// for missing fields, original value will be used instead
func ReverseProxyUpstreamUpdate(w http.ResponseWriter, r *http.Request) {
	endpoint, err := utils.PostPara(r, "ep")
	if err != nil {
		utils.SendErrorResponse(w, "endpoint not defined")
		return
	}

	targetEndpoint, err := dynamicProxyRouter.LoadProxy(endpoint)
	if err != nil {
		utils.SendErrorResponse(w, "target endpoint not found")
		return
	}

	//Editing upstream origin IP
	originIP, err := utils.PostPara(r, "origin")
	if err != nil {
		utils.SendErrorResponse(w, "origin ip or matching address not set")
		return
	}
	originIP = strings.TrimSpace(originIP)

	//Update content payload
	payload, err := utils.PostPara(r, "payload")
	if err != nil {
		utils.SendErrorResponse(w, "update payload not set")
		return
	}

	isActive, _ := utils.PostBool(r, "active")

	targetUpstream, err := targetEndpoint.GetUpstreamOriginByMatchingIP(originIP)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Deep copy the upstream so other request handling goroutine won't be effected
	newUpstream := targetUpstream.Clone()

	//Overwrite the new value into the old upstream
	err = json.Unmarshal([]byte(payload), &newUpstream)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Replace the old upstream with the new one
	err = targetEndpoint.RemoveUpstreamOrigin(originIP)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	err = targetEndpoint.AddUpstreamOrigin(newUpstream, isActive)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Save changes to configs
	err = SaveReverseProxyConfig(targetEndpoint)
	if err != nil {
		SystemWideLogger.PrintAndLog("INFO", "Unable to save upstream update to proxy config", err)
		utils.SendErrorResponse(w, "Failed to save updated upstream config")
		return
	}

	//Update Uptime Monitor
	UpdateUptimeMonitorTargets()
	utils.SendOK(w)
}

func ReverseProxyUpstreamSetPriority(w http.ResponseWriter, r *http.Request) {
	endpoint, err := utils.PostPara(r, "ep")
	if err != nil {
		utils.SendErrorResponse(w, "endpoint not defined")
		return
	}

	targetEndpoint, err := dynamicProxyRouter.LoadProxy(endpoint)
	if err != nil {
		utils.SendErrorResponse(w, "target endpoint not found")
		return
	}

	weight, err := utils.PostInt(r, "weight")
	if err != nil {
		utils.SendErrorResponse(w, "priority not defined")
		return
	}

	if weight < 0 {
		utils.SendErrorResponse(w, "invalid weight given")
		return
	}

	//Editing upstream origin IP
	originIP, err := utils.PostPara(r, "origin")
	if err != nil {
		utils.SendErrorResponse(w, "origin ip or matching address not set")
		return
	}
	originIP = strings.TrimSpace(originIP)

	editingUpstream, err := targetEndpoint.GetUpstreamOriginByMatchingIP(originIP)
	editingUpstream.Weight = weight
	// The editing upstream is a pointer to the runtime object
	// and the change of weight do not requre a respawn of the proxy object
	// so no need to remove & re-prepare the upstream on weight changes

	err = SaveReverseProxyConfig(targetEndpoint)
	if err != nil {
		SystemWideLogger.PrintAndLog("INFO", "Unable to update upstream weight", err)
		utils.SendErrorResponse(w, "Failed to update upstream weight")
		return
	}

	utils.SendOK(w)
}

func ReverseProxyUpstreamDelete(w http.ResponseWriter, r *http.Request) {
	endpoint, err := utils.PostPara(r, "ep")
	if err != nil {
		utils.SendErrorResponse(w, "endpoint not defined")
		return
	}

	targetEndpoint, err := dynamicProxyRouter.LoadProxy(endpoint)
	if err != nil {
		utils.SendErrorResponse(w, "target endpoint not found")
		return
	}

	//Editing upstream origin IP
	originIP, err := utils.PostPara(r, "origin")
	if err != nil {
		utils.SendErrorResponse(w, "origin ip or matching address not set")
		return
	}
	originIP = strings.TrimSpace(originIP)

	if !targetEndpoint.UpstreamOriginExists(originIP) {
		utils.SendErrorResponse(w, "target upstream not found")
		return
	}

	err = targetEndpoint.RemoveUpstreamOrigin(originIP)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	//Save changes to configs
	err = SaveReverseProxyConfig(targetEndpoint)
	if err != nil {
		SystemWideLogger.PrintAndLog("INFO", "Unable to remove upstream", err)
		utils.SendErrorResponse(w, "Failed to remove upstream from proxy rule")
		return
	}

	//Update uptime monitor
	UpdateUptimeMonitorTargets()

	utils.SendOK(w)
}
