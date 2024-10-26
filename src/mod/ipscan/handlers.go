package ipscan

/*
	ipscan http handlers

	This script provide http handlers for ipscan module
*/

import (
	"encoding/json"
	"net"
	"net/http"

	"imuslab.com/zoraxy/mod/utils"
)

// HandleScanPort is the HTTP handler for scanning opened ports on a given IP address
func HandleScanPort(w http.ResponseWriter, r *http.Request) {
	targetIp, err := utils.GetPara(r, "ip")
	if err != nil {
		utils.SendErrorResponse(w, "target IP address not given")
		return
	}

	// Check if the IP is a valid IP address
	ip := net.ParseIP(targetIp)
	if ip == nil {
		utils.SendErrorResponse(w, "invalid IP address")
		return
	}

	// Scan the ports
	openPorts := ScanPorts(targetIp)
	jsonData, err := json.Marshal(openPorts)
	if err != nil {
		utils.SendErrorResponse(w, "failed to marshal JSON")
		return
	}

	utils.SendJSONResponse(w, string(jsonData))
}

// HandleIpScan is the HTTP handler for scanning IP addresses in a given range or CIDR
func HandleIpScan(w http.ResponseWriter, r *http.Request) {
	cidr, err := utils.PostPara(r, "cidr")
	if err != nil {
		//Ip range mode
		start, err := utils.PostPara(r, "start")
		if err != nil {
			utils.SendErrorResponse(w, "missing start ip")
			return
		}

		end, err := utils.PostPara(r, "end")
		if err != nil {
			utils.SendErrorResponse(w, "missing end ip")
			return
		}

		discoveredHosts, err := ScanIpRange(start, end)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}

		js, _ := json.Marshal(discoveredHosts)
		utils.SendJSONResponse(w, string(js))
	} else {
		//CIDR mode
		discoveredHosts, err := ScanCIDRRange(cidr)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}

		js, _ := json.Marshal(discoveredHosts)
		utils.SendJSONResponse(w, string(js))
	}
}
