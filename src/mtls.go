package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"imuslab.com/zoraxy/mod/dynamicproxy"
	"imuslab.com/zoraxy/mod/mtls"
	"imuslab.com/zoraxy/mod/tlscert"
	"imuslab.com/zoraxy/mod/utils"
)

/*
	mTLS (client certificate authentication) APIs

	The CA store is global, which hosts trust which CA is configured per proxy rule.
*/

type mtlsCAEntry struct {
	*mtls.CAInfo
	UsedBy []string //Proxy rules currently trusting this CA
}

// List the stored client CAs, together with the proxy rules that use each of them
func handleMTLSListCA(w http.ResponseWriter, r *http.Request) {
	usedBy := map[string][]string{}
	dynamicProxyRouter.ProxyEndpoints.Range(func(_, value interface{}) bool {
		ept, ok := value.(*dynamicproxy.ProxyEndpoint)
		if !ok || ept.TlsOptions == nil || ept.TlsOptions.MTLSMode == "" || ept.TlsOptions.MTLSMode == mtls.ModeOff {
			return true
		}
		for _, ca := range ept.TlsOptions.MTLSCAs {
			usedBy[ca] = append(usedBy[ca], ept.RootOrMatchingDomain)
		}
		return true
	})

	result := []*mtlsCAEntry{}
	for _, info := range mtlsManager.ListCAs() {
		hosts := usedBy[info.Name]
		if hosts == nil {
			hosts = []string{}
		}
		result = append(result, &mtlsCAEntry{CAInfo: info, UsedBy: hosts})
	}
	js, _ := json.Marshal(result)
	utils.SendJSONResponse(w, string(js))
}

// Add a client CA from PEM text. Parameters: name, pem
func handleMTLSAddCA(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "Method not supported")
		return
	}
	name, err := utils.PostPara(r, "name")
	if err != nil {
		utils.SendErrorResponse(w, "Invalid CA name")
		return
	}
	pemText, err := utils.PostPara(r, "pem")
	if err != nil {
		utils.SendErrorResponse(w, "Certificate content is empty")
		return
	}
	if err := mtlsManager.AddCA(name, []byte(pemText)); err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	SystemWideLogger.PrintAndLog("mtls", "Client CA added: "+strings.TrimSpace(name), nil)
	utils.SendOK(w)
}

// Remove a client CA. Refused while a proxy rule still depends on it. Parameter: name
func handleMTLSRemoveCA(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "Method not supported")
		return
	}
	name, err := utils.PostPara(r, "name")
	if err != nil {
		utils.SendErrorResponse(w, "Invalid CA name")
		return
	}

	inUse := []string{}
	dynamicProxyRouter.ProxyEndpoints.Range(func(_, value interface{}) bool {
		ept, ok := value.(*dynamicproxy.ProxyEndpoint)
		if !ok || ept.TlsOptions == nil || ept.TlsOptions.MTLSMode == "" || ept.TlsOptions.MTLSMode == mtls.ModeOff {
			return true
		}
		for _, ca := range ept.TlsOptions.MTLSCAs {
			if ca == name {
				inUse = append(inUse, ept.RootOrMatchingDomain)
				break
			}
		}
		return true
	})
	if len(inUse) > 0 {
		utils.SendErrorResponse(w, "CA is still used by: "+strings.Join(inUse, ", "))
		return
	}

	if err := mtlsManager.RemoveCA(name); err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	SystemWideLogger.PrintAndLog("mtls", "Client CA removed: "+name, nil)
	utils.SendOK(w)
}

// Set the mTLS policy of a proxy rule.
// Parameters: ep, mode (off|optional|required), cas (JSON string array), bypassAuth (bool)
func handleMTLSSetHostConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "Method not supported")
		return
	}
	ep, err := utils.PostPara(r, "ep")
	if err != nil {
		utils.SendErrorResponse(w, "Invalid ep given")
		return
	}
	mode, err := utils.PostPara(r, "mode")
	if err != nil || (mode != mtls.ModeOff && mode != mtls.ModeOptional && mode != mtls.ModeRequired) {
		utils.SendErrorResponse(w, "Invalid mode given")
		return
	}
	bypassAuth, _ := utils.PostBool(r, "bypassAuth")

	cas := []string{}
	if casJson, err := utils.PostPara(r, "cas"); err == nil && strings.TrimSpace(casJson) != "" {
		if err := json.Unmarshal([]byte(casJson), &cas); err != nil {
			utils.SendErrorResponse(w, "Invalid CA list given")
			return
		}
	}
	for _, name := range cas {
		if !mtlsManager.CAExists(name) {
			utils.SendErrorResponse(w, "Unknown CA: "+name)
			return
		}
	}
	if mode != mtls.ModeOff && len(cas) == 0 {
		//An empty trust list would lock every client out (required) or never match (optional)
		utils.SendErrorResponse(w, "Select at least one client CA")
		return
	}

	ept, err := dynamicProxyRouter.LoadProxy(ep)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	if ept.TlsOptions == nil {
		ept.TlsOptions = tlscert.GetDefaultHostSpecificTlsBehavior()
	}
	ept.TlsOptions.MTLSMode = mode
	ept.TlsOptions.MTLSCAs = cas
	ept.TlsOptions.MTLSBypassAuth = bypassAuth

	readyRoutingRule, err := dynamicProxyRouter.PrepareProxyRoute(ept)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	dynamicProxyRouter.AddProxyRouteToRuntime(readyRoutingRule)

	if err := SaveReverseProxyConfig(ept); err != nil {
		utils.SendErrorResponse(w, "Failed to save mTLS config: "+err.Error())
		return
	}
	utils.SendOK(w)
}
