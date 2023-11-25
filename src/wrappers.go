package main

/*
	Wrappers.go

	This script provide wrapping functions
	for modules that do not provide
	handler interface within the modules

	--- NOTES ---
	If your module have more than one layer
	or require state keeping, please move
	the abstraction up one layer into
	your own module. Do not keep state on
	the global scope other than single
	Manager instance
*/

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"imuslab.com/zoraxy/mod/dynamicproxy"
	"imuslab.com/zoraxy/mod/ipscan"
	"imuslab.com/zoraxy/mod/mdns"
	"imuslab.com/zoraxy/mod/uptime"
	"imuslab.com/zoraxy/mod/utils"
	"imuslab.com/zoraxy/mod/wakeonlan"
)

/*
	Proxy Utils
*/
//Check if site support TLS
func HandleCheckSiteSupportTLS(w http.ResponseWriter, r *http.Request) {
	targetURL, err := utils.PostPara(r, "url")
	if err != nil {
		utils.SendErrorResponse(w, "invalid url given")
		return
	}

	httpsUrl := fmt.Sprintf("https://%s", targetURL)
	httpUrl := fmt.Sprintf("http://%s", targetURL)

	client := http.Client{Timeout: 5 * time.Second}

	resp, err := client.Head(httpsUrl)
	if err == nil && resp.StatusCode == http.StatusOK {
		js, _ := json.Marshal("https")
		utils.SendJSONResponse(w, string(js))
		return
	}

	resp, err = client.Head(httpUrl)
	if err == nil && resp.StatusCode == http.StatusOK {
		js, _ := json.Marshal("http")
		utils.SendJSONResponse(w, string(js))
		return
	}

	utils.SendErrorResponse(w, "invalid url given")
}

/*
	Statistic Summary
*/

// Handle conversion of statistic daily summary to country summary
func HandleCountryDistrSummary(w http.ResponseWriter, r *http.Request) {
	requestClientCountry := map[string]int{}
	statisticCollector.DailySummary.RequestClientIp.Range(func(key, value interface{}) bool {
		//Get this client country of original
		clientIp := key.(string)
		//requestCount := value.(int)

		ci, err := geodbStore.ResolveCountryCodeFromIP(clientIp)
		if err != nil {
			return true
		}

		isoCode := ci.CountryIsoCode
		if isoCode == "" {
			//local or reserved addr
			isoCode = "local"
		}
		uc, ok := requestClientCountry[isoCode]
		if !ok {
			//Create the counter
			requestClientCountry[isoCode] = 1
		} else {
			requestClientCountry[isoCode] = uc + 1
		}
		return true
	})

	js, _ := json.Marshal(requestClientCountry)
	utils.SendJSONResponse(w, string(js))
}

/*
	Up Time Monitor
*/

// Update uptime monitor targets after rules updated
// See https://github.com/tobychui/zoraxy/issues/77
func UpdateUptimeMonitorTargets() {
	if uptimeMonitor != nil {
		uptimeMonitor.Config.Targets = GetUptimeTargetsFromReverseProxyRules(dynamicProxyRouter)
		go func() {
			uptimeMonitor.ExecuteUptimeCheck()
		}()

		SystemWideLogger.PrintAndLog("Uptime", "Uptime monitor config updated", nil)
	}
}

// Generate uptime monitor targets from reverse proxy rules
func GetUptimeTargetsFromReverseProxyRules(dp *dynamicproxy.Router) []*uptime.Target {
	subds := dp.GetSDProxyEndpointsAsMap()
	vdirs := dp.GetVDProxyEndpointsAsMap()

	UptimeTargets := []*uptime.Target{}
	for subd, target := range subds {
		url := "http://" + target.Domain
		protocol := "http"
		if target.RequireTLS {
			url = "https://" + target.Domain
			protocol = "https"
		}

		UptimeTargets = append(UptimeTargets, &uptime.Target{
			ID:       subd,
			Name:     subd,
			URL:      url,
			Protocol: protocol,
		})
	}

	for vdir, target := range vdirs {
		url := "http://" + target.Domain
		protocol := "http"
		if target.RequireTLS {
			url = "https://" + target.Domain
			protocol = "https"
		}
		UptimeTargets = append(UptimeTargets, &uptime.Target{
			ID:       vdir,
			Name:     "*" + vdir,
			URL:      url,
			Protocol: protocol,
		})
	}

	return UptimeTargets
}

// Handle rendering up time monitor data
func HandleUptimeMonitorListing(w http.ResponseWriter, r *http.Request) {
	if uptimeMonitor != nil {
		uptimeMonitor.HandleUptimeLogRead(w, r)
	} else {
		http.Error(w, "500 - Internal Server Error", http.StatusInternalServerError)
		return
	}
}

// Handle listing current registered mdns nodes
func HandleMdnsListing(w http.ResponseWriter, r *http.Request) {
	if mdnsScanner == nil {
		utils.SendErrorResponse(w, "mDNS scanner is disabled on this host")
		return
	}
	js, _ := json.Marshal(previousmdnsScanResults)
	utils.SendJSONResponse(w, string(js))
}

func HandleMdnsScanning(w http.ResponseWriter, r *http.Request) {
	if mdnsScanner == nil {
		utils.SendErrorResponse(w, "mDNS scanner is disabled on this host")
		return
	}
	domain, err := utils.PostPara(r, "domain")
	var hosts []*mdns.NetworkHost
	if err != nil {
		//Search for arozos node
		hosts = mdnsScanner.Scan(30, "")
		previousmdnsScanResults = hosts
	} else {
		//Search for other nodes
		hosts = mdnsScanner.Scan(30, domain)
	}

	js, _ := json.Marshal(hosts)
	utils.SendJSONResponse(w, string(js))
}

// handle ip scanning
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

		discoveredHosts, err := ipscan.ScanIpRange(start, end)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}

		js, _ := json.Marshal(discoveredHosts)
		utils.SendJSONResponse(w, string(js))
	} else {
		//CIDR mode
		discoveredHosts, err := ipscan.ScanCIDRRange(cidr)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}

		js, _ := json.Marshal(discoveredHosts)
		utils.SendJSONResponse(w, string(js))
	}
}

/*
	WAKE ON LAN

	Handle wake on LAN
	Support following methods
	/?set=xxx&name=xxx Record a new MAC address into the database
	/?wake=xxx Wake a server given its MAC address
	/?del=xxx Delete a server given its MAC address
	/ Default: list all recorded WoL MAC address
*/

func HandleWakeOnLan(w http.ResponseWriter, r *http.Request) {
	set, _ := utils.PostPara(r, "set")
	del, _ := utils.PostPara(r, "del")
	wake, _ := utils.PostPara(r, "wake")
	if set != "" {
		//Get the name of the describing server
		servername, err := utils.PostPara(r, "name")
		if err != nil {
			utils.SendErrorResponse(w, "invalid server name given")
			return
		}

		//Check if the given mac address is a valid mac address
		set = strings.TrimSpace(set)
		if !wakeonlan.IsValidMacAddress(set) {
			utils.SendErrorResponse(w, "invalid mac address given")
			return
		}

		//Store this into the database
		sysdb.Write("wolmac", set, servername)

		utils.SendOK(w)
	} else if wake != "" {
		//Wake the target up by MAC address
		if !wakeonlan.IsValidMacAddress(wake) {
			utils.SendErrorResponse(w, "invalid mac address given")
			return
		}

		SystemWideLogger.PrintAndLog("WoL", "Sending Wake on LAN magic packet to "+wake, nil)
		err := wakeonlan.WakeTarget(wake)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}

		utils.SendOK(w)
	} else if del != "" {
		if !wakeonlan.IsValidMacAddress(del) {
			utils.SendErrorResponse(w, "invalid mac address given")
			return
		}

		sysdb.Delete("wolmac", del)
		utils.SendOK(w)
	} else {
		//List all the saved WoL MAC Address
		entries, err := sysdb.ListTable("wolmac")
		if err != nil {
			utils.SendErrorResponse(w, "unknown error occured")
			return
		}

		type MacAddrRecord struct {
			ServerName string
			MacAddr    string
		}

		results := []*MacAddrRecord{}
		for _, keypairs := range entries {
			macAddr := string(keypairs[0])
			serverName := ""
			json.Unmarshal(keypairs[1], &serverName)

			results = append(results, &MacAddrRecord{
				ServerName: serverName,
				MacAddr:    macAddr,
			})
		}

		js, _ := json.Marshal(results)
		utils.SendJSONResponse(w, string(js))
	}
}

/*
	Zoraxy Host Info
*/

func HandleZoraxyInfo(w http.ResponseWriter, r *http.Request) {
	type ZoraxyInfo struct {
		Version           string
		NodeUUID          string
		Development       bool
		BootTime          int64
		EnableSshLoopback bool
		ZerotierConnected bool
	}

	info := ZoraxyInfo{
		Version:           version,
		NodeUUID:          nodeUUID,
		Development:       development,
		BootTime:          bootTime,
		EnableSshLoopback: *allowSshLoopback,
		ZerotierConnected: ganManager.ControllerID != "",
	}

	js, _ := json.MarshalIndent(info, "", " ")
	utils.SendJSONResponse(w, string(js))
}

func HandleGeoIpLookup(w http.ResponseWriter, r *http.Request) {
	ip, err := utils.GetPara(r, "ip")
	if err != nil {
		utils.SendErrorResponse(w, "ip not given")
		return
	}

	cc, err := geodbStore.ResolveCountryCodeFromIP(ip)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	js, _ := json.Marshal(cc)
	utils.SendJSONResponse(w, string(js))
}
