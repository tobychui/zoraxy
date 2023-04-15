package main

import (
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"imuslab.com/zoraxy/mod/dynamicproxy"
	"imuslab.com/zoraxy/mod/uptime"
	"imuslab.com/zoraxy/mod/utils"
)

var (
	dynamicProxyRouter *dynamicproxy.Router
)

// Add user customizable reverse proxy
func ReverseProxtInit() {
	inboundPort := 80
	if sysdb.KeyExists("settings", "inbound") {
		sysdb.Read("settings", "inbound", &inboundPort)
		log.Println("Serving inbound port ", inboundPort)
	} else {
		log.Println("Inbound port not set. Using default (80)")
	}

	useTls := false
	sysdb.Read("settings", "usetls", &useTls)
	if useTls {
		log.Println("TLS mode enabled. Serving proxxy request with TLS")
	} else {
		log.Println("TLS mode disabled. Serving proxy request with plain http")
	}

	forceHttpsRedirect := false
	sysdb.Read("settings", "redirect", &forceHttpsRedirect)
	if forceHttpsRedirect {
		log.Println("Force HTTPS mode enabled")
	} else {
		log.Println("Force HTTPS mode disabled")
	}

	dprouter, err := dynamicproxy.NewDynamicProxy(dynamicproxy.RouterOption{
		Port:               inboundPort,
		UseTls:             useTls,
		ForceHttpsRedirect: forceHttpsRedirect,
		TlsManager:         tlsCertManager,
		RedirectRuleTable:  redirectTable,
		GeodbStore:         geodbStore,
		StatisticCollector: statisticCollector,
	})
	if err != nil {
		log.Println(err.Error())
		return
	}

	dynamicProxyRouter = dprouter

	//Load all conf from files
	confs, _ := filepath.Glob("./conf/*.config")
	for _, conf := range confs {
		record, err := LoadReverseProxyConfig(conf)
		if err != nil {
			log.Println("Failed to load "+filepath.Base(conf), err.Error())
			return
		}

		if record.ProxyType == "root" {
			dynamicProxyRouter.SetRootProxy(record.ProxyTarget, record.UseTLS)
		} else if record.ProxyType == "subd" {
			dynamicProxyRouter.AddSubdomainRoutingService(record.Rootname, record.ProxyTarget, record.UseTLS)
		} else if record.ProxyType == "vdir" {
			dynamicProxyRouter.AddVirtualDirectoryProxyService(record.Rootname, record.ProxyTarget, record.UseTLS)
		} else {
			log.Println("Unsupported endpoint type: " + record.ProxyType + ". Skipping " + filepath.Base(conf))
		}
	}

	/*
		dynamicProxyRouter.SetRootProxy("192.168.0.107:8080", false)
		dynamicProxyRouter.AddSubdomainRoutingService("aroz.localhost", "192.168.0.107:8080/private/AOB/", false)
		dynamicProxyRouter.AddSubdomainRoutingService("loopback.localhost", "localhost:8080", false)
		dynamicProxyRouter.AddSubdomainRoutingService("git.localhost", "mc.alanyeung.co:3000", false)
		dynamicProxyRouter.AddVirtualDirectoryProxyService("/git/server/", "mc.alanyeung.co:3000", false)
	*/

	//Start Service
	//Not sure why but delay must be added if you have another
	//reverse proxy server in front of this service
	time.Sleep(300 * time.Millisecond)
	dynamicProxyRouter.StartProxyService()
	log.Println("Dynamic Reverse Proxy service started")

	//Add all proxy services to uptime monitor
	//Create a uptime monitor service
	go func() {
		//This must be done in go routine to prevent blocking on system startup
		uptimeMonitor, _ = uptime.NewUptimeMonitor(&uptime.Config{
			Targets:         GetUptimeTargetsFromReverseProxyRules(dynamicProxyRouter),
			Interval:        300, //5 minutes
			MaxRecordsStore: 288, //1 day
		})
		log.Println("Uptime Monitor background service started")
	}()

}

func ReverseProxyHandleOnOff(w http.ResponseWriter, r *http.Request) {

	enable, _ := utils.PostPara(r, "enable") //Support root, vdir and subd
	if enable == "true" {
		err := dynamicProxyRouter.StartProxyService()
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}
	} else {
		//Check if it is loopback
		if dynamicProxyRouter.IsProxiedSubdomain(r) {
			//Loopback routing. Turning it off will make the user lost control
			//of the whole system. Do not allow shutdown
			utils.SendErrorResponse(w, "Unable to shutdown in loopback rp mode. Remove proxy rules for management interface and retry.")
			return
		}

		err := dynamicProxyRouter.StopProxyService()
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}
	}

	utils.SendOK(w)
}

func ReverseProxyHandleAddEndpoint(w http.ResponseWriter, r *http.Request) {
	eptype, err := utils.PostPara(r, "type") //Support root, vdir and subd
	if err != nil {
		utils.SendErrorResponse(w, "type not defined")
		return
	}

	endpoint, err := utils.PostPara(r, "ep")
	if err != nil {
		utils.SendErrorResponse(w, "endpoint not defined")
		return
	}

	tls, _ := utils.PostPara(r, "tls")
	if tls == "" {
		tls = "false"
	}

	useTLS := (tls == "true")
	rootname := ""
	if eptype == "vdir" {
		vdir, err := utils.PostPara(r, "rootname")
		if err != nil {
			utils.SendErrorResponse(w, "vdir not defined")
			return
		}

		if !strings.HasPrefix(vdir, "/") {
			vdir = "/" + vdir
		}
		rootname = vdir
		dynamicProxyRouter.AddVirtualDirectoryProxyService(vdir, endpoint, useTLS)

	} else if eptype == "subd" {
		subdomain, err := utils.PostPara(r, "rootname")
		if err != nil {
			utils.SendErrorResponse(w, "subdomain not defined")
			return
		}
		rootname = subdomain
		dynamicProxyRouter.AddSubdomainRoutingService(subdomain, endpoint, useTLS)
	} else if eptype == "root" {
		rootname = "root"
		dynamicProxyRouter.SetRootProxy(endpoint, useTLS)
	} else {
		//Invalid eptype
		utils.SendErrorResponse(w, "Invalid endpoint type")
		return
	}

	//Save it
	SaveReverseProxyConfig(eptype, rootname, endpoint, useTLS)

	//Update utm if exists
	if uptimeMonitor != nil {
		uptimeMonitor.Config.Targets = GetUptimeTargetsFromReverseProxyRules(dynamicProxyRouter)
		uptimeMonitor.CleanRecords()
	}

	utils.SendOK(w)

}

func DeleteProxyEndpoint(w http.ResponseWriter, r *http.Request) {
	ep, err := utils.GetPara(r, "ep")
	if err != nil {
		utils.SendErrorResponse(w, "Invalid ep given")
	}

	ptype, err := utils.PostPara(r, "ptype")
	if err != nil {
		utils.SendErrorResponse(w, "Invalid ptype given")
	}

	err = dynamicProxyRouter.RemoveProxy(ptype, ep)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
	}

	RemoveReverseProxyConfig(ep)

	//Update utm if exists
	if uptimeMonitor != nil {
		uptimeMonitor.Config.Targets = GetUptimeTargetsFromReverseProxyRules(dynamicProxyRouter)
		uptimeMonitor.CleanRecords()
	}

	utils.SendOK(w)
}

func ReverseProxyStatus(w http.ResponseWriter, r *http.Request) {
	js, _ := json.Marshal(dynamicProxyRouter)
	utils.SendJSONResponse(w, string(js))
}

func ReverseProxyList(w http.ResponseWriter, r *http.Request) {
	eptype, err := utils.PostPara(r, "type") //Support root, vdir and subd
	if err != nil {
		utils.SendErrorResponse(w, "type not defined")
		return
	}

	if eptype == "vdir" {
		results := []*dynamicproxy.ProxyEndpoint{}
		dynamicProxyRouter.ProxyEndpoints.Range(func(key, value interface{}) bool {
			results = append(results, value.(*dynamicproxy.ProxyEndpoint))
			return true
		})

		sort.Slice(results, func(i, j int) bool {
			return results[i].Domain < results[j].Domain
		})

		js, _ := json.Marshal(results)
		utils.SendJSONResponse(w, string(js))
	} else if eptype == "subd" {
		results := []*dynamicproxy.SubdomainEndpoint{}
		dynamicProxyRouter.SubdomainEndpoint.Range(func(key, value interface{}) bool {
			results = append(results, value.(*dynamicproxy.SubdomainEndpoint))
			return true
		})

		sort.Slice(results, func(i, j int) bool {
			return results[i].MatchingDomain < results[j].MatchingDomain
		})

		js, _ := json.Marshal(results)
		utils.SendJSONResponse(w, string(js))
	} else if eptype == "root" {
		js, _ := json.Marshal(dynamicProxyRouter.Root)
		utils.SendJSONResponse(w, string(js))
	} else {
		utils.SendErrorResponse(w, "Invalid type given")
	}
}

// Handle https redirect
func HandleUpdateHttpsRedirect(w http.ResponseWriter, r *http.Request) {
	useRedirect, err := utils.GetPara(r, "set")
	if err != nil {
		currentRedirectToHttps := false
		//Load the current status
		err = sysdb.Read("settings", "redirect", &currentRedirectToHttps)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}
		js, _ := json.Marshal(currentRedirectToHttps)
		utils.SendJSONResponse(w, string(js))
	} else {
		if useRedirect == "true" {
			sysdb.Write("settings", "redirect", true)
			log.Println("Updating force HTTPS redirection to true")
			dynamicProxyRouter.UpdateHttpToHttpsRedirectSetting(true)
		} else if useRedirect == "false" {
			sysdb.Write("settings", "redirect", false)
			log.Println("Updating force HTTPS redirection to false")
			dynamicProxyRouter.UpdateHttpToHttpsRedirectSetting(false)
		}

		utils.SendOK(w)
	}
}

//Handle checking if the current user is accessing via the reverse proxied interface
//Of the management interface.
func HandleManagementProxyCheck(w http.ResponseWriter, r *http.Request) {
	isProxied := dynamicProxyRouter.IsProxiedSubdomain(r)
	js, _ := json.Marshal(isProxied)
	utils.SendJSONResponse(w, string(js))
}

// Handle incoming port set. Change the current proxy incoming port
func HandleIncomingPortSet(w http.ResponseWriter, r *http.Request) {
	newIncomingPort, err := utils.PostPara(r, "incoming")
	if err != nil {
		utils.SendErrorResponse(w, "invalid incoming port given")
		return
	}

	newIncomingPortInt, err := strconv.Atoi(newIncomingPort)
	if err != nil {
		utils.SendErrorResponse(w, "invalid incoming port given")
		return
	}

	//Stop and change the setting of the reverse proxy service
	if dynamicProxyRouter.Running {
		dynamicProxyRouter.StopProxyService()
		dynamicProxyRouter.Option.Port = newIncomingPortInt
		dynamicProxyRouter.StartProxyService()
	} else {
		//Only change setting but not starting the proxy service
		dynamicProxyRouter.Option.Port = newIncomingPortInt
	}

	sysdb.Write("settings", "inbound", newIncomingPortInt)

	utils.SendOK(w)
}
