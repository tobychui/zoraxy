package main

/*
	Vdir.go

	This script handle virtual directory functions
	in global scopes

	Author: tobychui
*/

import (
	"encoding/json"
	"net/http"
	"strings"

	"imuslab.com/zoraxy/mod/dynamicproxy"
	"imuslab.com/zoraxy/mod/utils"
)

// List the Virtual directory under given proxy rule
func ReverseProxyListVdir(w http.ResponseWriter, r *http.Request) {
	eptype, err := utils.PostPara(r, "type") //Support root and host
	if err != nil {
		utils.SendErrorResponse(w, "type not defined")
		return
	}

	var targetEndpoint *dynamicproxy.ProxyEndpoint
	if eptype == "host" {
		endpoint, err := utils.PostPara(r, "ep") //Support root and host
		if err != nil {
			utils.SendErrorResponse(w, "endpoint not defined")
			return
		}

		targetEndpoint, err = dynamicProxyRouter.LoadProxy(endpoint)
		if err != nil {
			utils.SendErrorResponse(w, "target endpoint not found")
			return
		}
	} else if eptype == "root" {
		targetEndpoint = dynamicProxyRouter.Root
	} else {
		utils.SendErrorResponse(w, "invalid type given")
		return
	}

	//Parse result to json
	vdirs := targetEndpoint.VirtualDirectories
	if targetEndpoint.VirtualDirectories == nil {
		//Avoid returning null to front-end
		vdirs = []*dynamicproxy.VirtualDirectoryEndpoint{}
	}
	js, _ := json.Marshal(vdirs)
	utils.SendJSONResponse(w, string(js))
}

// Add Virtual Directory to a host
func ReverseProxyAddVdir(w http.ResponseWriter, r *http.Request) {
	eptype, err := utils.PostPara(r, "type") //Support root and host
	if err != nil {
		utils.SendErrorResponse(w, "type not defined")
		return
	}

	matchingPath, err := utils.PostPara(r, "path")
	if err != nil {
		utils.SendErrorResponse(w, "matching path not defined")
		return
	}

	//Must start with /
	if !strings.HasPrefix(matchingPath, "/") {
		matchingPath = "/" + matchingPath
	}

	domain, err := utils.PostPara(r, "domain")
	if err != nil {
		utils.SendErrorResponse(w, "target domain not defined")
		return
	}

	reqTLSStr, err := utils.PostPara(r, "reqTLS")
	if err != nil {
		//Assume false
		reqTLSStr = "false"
	}
	reqTLS := (reqTLSStr == "true")

	skipValidStr, err := utils.PostPara(r, "skipValid")
	if err != nil {
		//Assume false
		skipValidStr = "false"
	}

	skipValid := (skipValidStr == "true")

	//Load the target proxy endpoint from runtime
	var targetProxyEndpoint *dynamicproxy.ProxyEndpoint
	if eptype == "root" {
		//Check if root is running at reverse proxy mode
		if dynamicProxyRouter.Root.DefaultSiteOption != dynamicproxy.DefaultSite_ReverseProxy {
			utils.SendErrorResponse(w, "virtual directory can only be added to root router under proxy mode")
			return
		}
		targetProxyEndpoint = dynamicProxyRouter.Root
	} else if eptype == "host" {
		endpointID, err := utils.PostPara(r, "endpoint")
		if err != nil {
			utils.SendErrorResponse(w, "endpoint not defined")
			return
		}

		loadedEndpoint, err := dynamicProxyRouter.LoadProxy(endpointID)
		if err != nil {
			utils.SendErrorResponse(w, "selected proxy host not exists")
			return
		}

		targetProxyEndpoint = loadedEndpoint
	} else {
		utils.SendErrorResponse(w, "invalid proxy type given")
		return
	}

	// Create a virtual directory entry base on the above info
	newVirtualDirectoryRouter := dynamicproxy.VirtualDirectoryEndpoint{
		MatchingPath:        matchingPath,
		Domain:              domain,
		RequireTLS:          reqTLS,
		SkipCertValidations: skipValid,
	}

	//Add Virtual Directory Rule to this Proxy Endpoint
	activatedProxyEndpoint, err := targetProxyEndpoint.AddVirtualDirectoryRule(&newVirtualDirectoryRouter)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Save it to file
	SaveReverseProxyConfig(activatedProxyEndpoint)

	// Update uptime monitor
	UpdateUptimeMonitorTargets()
	utils.SendOK(w)
}

func ReverseProxyDeleteVdir(w http.ResponseWriter, r *http.Request) {
	eptype, err := utils.PostPara(r, "type") //Support root and host
	if err != nil {
		utils.SendErrorResponse(w, "type not defined")
		return
	}

	vdir, err := utils.PostPara(r, "vdir")
	if err != nil {
		utils.SendErrorResponse(w, "vdir matching key not defined")
		return
	}

	var targetEndpoint *dynamicproxy.ProxyEndpoint
	if eptype == "root" {
		targetEndpoint = dynamicProxyRouter.Root
	} else if eptype == "host" {
		//Proxy rule
		matchingPath, err := utils.PostPara(r, "path")
		if err != nil {
			utils.SendErrorResponse(w, "matching path not defined")
			return
		}

		ept, err := dynamicProxyRouter.LoadProxy(matchingPath)
		if err != nil {
			utils.SendErrorResponse(w, "target proxy rule not found")
			return
		}

		targetEndpoint = ept
	} else {
		utils.SendErrorResponse(w, "invalid endpoint type")
		return
	}

	//Delete the Vdir from endpoint
	err = targetEndpoint.RemoveVirtualDirectoryRuleByMatchingPath(vdir)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	err = SaveReverseProxyConfig(targetEndpoint)
	if err != nil {
		SystemWideLogger.PrintAndLog("Config", "Fail to write vdir rules update to config file", err)
		utils.SendErrorResponse(w, "unable to write changes to file")
		return
	}

	utils.SendOK(w)
}

// Handle update of reverse proxy vdir rules
func ReverseProxyEditVdir(w http.ResponseWriter, r *http.Request) {
	eptype, err := utils.PostPara(r, "type") //Support root and host
	if err != nil {
		utils.SendErrorResponse(w, "type not defined")
		return
	}

	vdir, err := utils.PostPara(r, "vdir")
	if err != nil {
		utils.SendErrorResponse(w, "vdir matching key not defined")
		return
	}

	domain, err := utils.PostPara(r, "domain")
	if err != nil {
		utils.SendErrorResponse(w, "target domain not defined")
		return
	}

	reqTLSStr, err := utils.PostPara(r, "reqTLS")
	if err != nil {
		//Assume false
		reqTLSStr = "false"
	}
	reqTLS := (reqTLSStr == "true")

	skipValidStr, err := utils.PostPara(r, "skipValid")
	if err != nil {
		//Assume false
		skipValidStr = "false"
	}

	skipValid := (skipValidStr == "true")

	var targetEndpoint *dynamicproxy.ProxyEndpoint
	if eptype == "root" {
		targetEndpoint = dynamicProxyRouter.Root

	} else if eptype == "host" {
		//Proxy rule
		matchingPath, err := utils.PostPara(r, "path")
		if err != nil {
			utils.SendErrorResponse(w, "matching path not defined")
			return
		}

		ept, err := dynamicProxyRouter.LoadProxy(matchingPath)
		if err != nil {
			utils.SendErrorResponse(w, "target proxy rule not found")
			return
		}

		targetEndpoint = ept
	} else {
		utils.SendErrorResponse(w, "invalid endpoint type given")
		return
	}

	//Check if the target vdir exists
	if targetEndpoint.GetVirtualDirectoryRuleByMatchingPath(vdir) == nil {
		utils.SendErrorResponse(w, "target virtual directory rule not exists")
		return
	}

	//Overwrite the target endpoint
	newVdirRule := dynamicproxy.VirtualDirectoryEndpoint{
		MatchingPath:        vdir,
		Domain:              domain,
		RequireTLS:          reqTLS,
		SkipCertValidations: skipValid,
		Disabled:            false,
	}

	targetEndpoint.RemoveVirtualDirectoryRuleByMatchingPath(vdir)
	activatedProxyEndpoint, err := targetEndpoint.AddVirtualDirectoryRule(&newVdirRule)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Save changes to file
	SaveReverseProxyConfig(activatedProxyEndpoint)

	UpdateUptimeMonitorTargets()

	utils.SendOK(w)
}
