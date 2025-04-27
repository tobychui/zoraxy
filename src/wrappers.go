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
	"net/http"
	"strconv"
	"strings"

	"imuslab.com/zoraxy/mod/dynamicproxy"
	"imuslab.com/zoraxy/mod/dynamicproxy/loadbalance"
	"imuslab.com/zoraxy/mod/mdns"
	"imuslab.com/zoraxy/mod/uptime"
	"imuslab.com/zoraxy/mod/utils"
	"imuslab.com/zoraxy/mod/wakeonlan"
)

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
		uptimeMonitor.CleanRecords()
		go func() {
			uptimeMonitor.ExecuteUptimeCheck()
		}()

		SystemWideLogger.PrintAndLog("uptime-monitor", "Uptime monitor config updated", nil)
	}
}

// Generate uptime monitor targets from reverse proxy rules
func GetUptimeTargetsFromReverseProxyRules(dp *dynamicproxy.Router) []*uptime.Target {
	hosts := dp.GetProxyEndpointsAsMap()

	UptimeTargets := []*uptime.Target{}
	for hostid, target := range hosts {
		if target.Disabled || target.DisableUptimeMonitor {
			//Skip those proxy rules that is disabled
			continue
		}
		isMultipleUpstreams := len(target.ActiveOrigins) > 1
		for i, origin := range target.ActiveOrigins {
			url := "http://" + origin.OriginIpOrDomain
			protocol := "http"
			if origin.RequireTLS {
				url = "https://" + origin.OriginIpOrDomain
				protocol = "https"
			}

			//Add the root url
			hostIdAndName := hostid
			if isMultipleUpstreams {
				hostIdAndName = hostIdAndName + " (upstream:" + strconv.Itoa(i) + ")"
			}
			UptimeTargets = append(UptimeTargets, &uptime.Target{
				ID:        hostIdAndName,
				Name:      hostIdAndName,
				URL:       url,
				Protocol:  protocol,
				ProxyType: uptime.ProxyType_Host,
			})

			//Add each virtual directory into the list
			for _, vdir := range target.VirtualDirectories {
				url := "http://" + vdir.Domain
				protocol := "http"
				if origin.RequireTLS {
					url = "https://" + vdir.Domain
					protocol = "https"
				}
				//Add the root url
				UptimeTargets = append(UptimeTargets, &uptime.Target{
					ID:        hostid + vdir.MatchingPath,
					Name:      hostid + vdir.MatchingPath,
					URL:       url,
					Protocol:  protocol,
					ProxyType: uptime.ProxyType_Vdir,
				})

			}
		}
	}

	return UptimeTargets
}

// Handle rendering up time monitor data
func HandleUptimeMonitorListing(w http.ResponseWriter, r *http.Request) {
	if uptimeMonitor != nil {
		uptimeMonitor.HandleUptimeLogRead(w, r)
	} else {
		http.Error(w, "500 - Internal Server Error (Still initializing)", http.StatusInternalServerError)
		return
	}
}

/*
	Static Web Server
*/

// Handle port change, if root router is using internal static web server
// update the root router as well
func HandleStaticWebServerPortChange(w http.ResponseWriter, r *http.Request) {
	newPort, err := utils.PostInt(r, "port")
	if err != nil {
		utils.SendErrorResponse(w, "invalid port number given")
		return
	}

	if dynamicProxyRouter.Root.DefaultSiteOption == dynamicproxy.DefaultSite_InternalStaticWebServer {
		//Update the root site as well
		newDraftingRoot := dynamicProxyRouter.Root.Clone()

		newDraftingRoot.ActiveOrigins = []*loadbalance.Upstream{
			{
				OriginIpOrDomain:         "127.0.0.1:" + strconv.Itoa(newPort),
				RequireTLS:               false,
				SkipCertValidations:      false,
				SkipWebSocketOriginCheck: true,
				Weight:                   0,
			},
		}
		activatedNewRoot, err := dynamicProxyRouter.PrepareProxyRoute(newDraftingRoot)
		if err != nil {
			utils.SendErrorResponse(w, "unable to update root routing rule")
			return
		}

		//Replace the root
		dynamicProxyRouter.Root = activatedNewRoot

		SaveReverseProxyConfig(newDraftingRoot)
	}

	err = staticWebServer.ChangePort(strconv.Itoa(newPort))
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

/*
	mDNS Scanning
*/

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

	displayUUID := nodeUUID
	displayAllowSSHLB := *allowSshLoopback
	displayBootTime := bootTime

	if !authAgent.CheckAuth(r) {
		displayUUID = "Unauthorized"
		displayAllowSSHLB = false
		displayBootTime = 0
	}

	info := ZoraxyInfo{
		Version:           SYSTEM_VERSION,
		NodeUUID:          displayUUID,
		Development:       *development_build,
		BootTime:          displayBootTime,
		EnableSshLoopback: displayAllowSSHLB,
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
