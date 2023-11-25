package main

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"imuslab.com/zoraxy/mod/auth"
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
		SystemWideLogger.Println("Serving inbound port ", inboundPort)
	} else {
		SystemWideLogger.Println("Inbound port not set. Using default (80)")
	}

	useTls := false
	sysdb.Read("settings", "usetls", &useTls)
	if useTls {
		SystemWideLogger.Println("TLS mode enabled. Serving proxxy request with TLS")
	} else {
		SystemWideLogger.Println("TLS mode disabled. Serving proxy request with plain http")
	}

	forceLatestTLSVersion := false
	sysdb.Read("settings", "forceLatestTLS", &forceLatestTLSVersion)
	if forceLatestTLSVersion {
		SystemWideLogger.Println("Force latest TLS mode enabled. Minimum TLS LS version is set to v1.2")
	} else {
		SystemWideLogger.Println("Force latest TLS mode disabled. Minimum TLS version is set to v1.0")
	}

	forceHttpsRedirect := false
	sysdb.Read("settings", "redirect", &forceHttpsRedirect)
	if forceHttpsRedirect {
		SystemWideLogger.Println("Force HTTPS mode enabled")
	} else {
		SystemWideLogger.Println("Force HTTPS mode disabled")
	}

	dprouter, err := dynamicproxy.NewDynamicProxy(dynamicproxy.RouterOption{
		HostUUID:           nodeUUID,
		Port:               inboundPort,
		UseTls:             useTls,
		ForceTLSLatest:     forceLatestTLSVersion,
		ForceHttpsRedirect: forceHttpsRedirect,
		TlsManager:         tlsCertManager,
		RedirectRuleTable:  redirectTable,
		GeodbStore:         geodbStore,
		StatisticCollector: statisticCollector,
		WebDirectory:       *staticWebServerRoot,
	})
	if err != nil {
		SystemWideLogger.PrintAndLog("Proxy", "Unable to create dynamic proxy router", err)
		return
	}

	dynamicProxyRouter = dprouter

	//Load all conf from files
	confs, _ := filepath.Glob("./conf/proxy/*.config")
	for _, conf := range confs {
		record, err := LoadReverseProxyConfig(conf)
		if err != nil {
			SystemWideLogger.PrintAndLog("Proxy", "Failed to load config file: "+filepath.Base(conf), err)
			return
		}

		if record.ProxyType == "root" {
			dynamicProxyRouter.SetRootProxy(&dynamicproxy.RootOptions{
				ProxyLocation: record.ProxyTarget,
				RequireTLS:    record.UseTLS,
			})
		} else if record.ProxyType == "subd" {
			dynamicProxyRouter.AddSubdomainRoutingService(&dynamicproxy.SubdOptions{
				MatchingDomain:          record.Rootname,
				Domain:                  record.ProxyTarget,
				RequireTLS:              record.UseTLS,
				BypassGlobalTLS:         record.BypassGlobalTLS,
				SkipCertValidations:     record.SkipTlsValidation,
				RequireBasicAuth:        record.RequireBasicAuth,
				BasicAuthCredentials:    record.BasicAuthCredentials,
				BasicAuthExceptionRules: record.BasicAuthExceptionRules,
			})
		} else if record.ProxyType == "vdir" {
			dynamicProxyRouter.AddVirtualDirectoryProxyService(&dynamicproxy.VdirOptions{
				RootName:                record.Rootname,
				Domain:                  record.ProxyTarget,
				RequireTLS:              record.UseTLS,
				BypassGlobalTLS:         record.BypassGlobalTLS,
				SkipCertValidations:     record.SkipTlsValidation,
				RequireBasicAuth:        record.RequireBasicAuth,
				BasicAuthCredentials:    record.BasicAuthCredentials,
				BasicAuthExceptionRules: record.BasicAuthExceptionRules,
			})
		} else {
			SystemWideLogger.PrintAndLog("Proxy", "Unsupported endpoint type: "+record.ProxyType+". Skipping "+filepath.Base(conf), nil)
		}
	}

	//Start Service
	//Not sure why but delay must be added if you have another
	//reverse proxy server in front of this service
	time.Sleep(300 * time.Millisecond)
	dynamicProxyRouter.StartProxyService()
	SystemWideLogger.Println("Dynamic Reverse Proxy service started")

	//Add all proxy services to uptime monitor
	//Create a uptime monitor service
	go func() {
		//This must be done in go routine to prevent blocking on system startup
		uptimeMonitor, _ = uptime.NewUptimeMonitor(&uptime.Config{
			Targets:         GetUptimeTargetsFromReverseProxyRules(dynamicProxyRouter),
			Interval:        300, //5 minutes
			MaxRecordsStore: 288, //1 day
		})
		SystemWideLogger.Println("Uptime Monitor background service started")
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

	bypassGlobalTLS, _ := utils.PostPara(r, "bypassGlobalTLS")
	if bypassGlobalTLS == "" {
		bypassGlobalTLS = "false"
	}

	useBypassGlobalTLS := bypassGlobalTLS == "true"

	stv, _ := utils.PostPara(r, "tlsval")
	if stv == "" {
		stv = "false"
	}

	skipTlsValidation := (stv == "true")

	rba, _ := utils.PostPara(r, "bauth")
	if rba == "" {
		rba = "false"
	}

	requireBasicAuth := (rba == "true")

	//Prase the basic auth to correct structure
	cred, _ := utils.PostPara(r, "cred")
	basicAuthCredentials := []*dynamicproxy.BasicAuthCredentials{}
	if requireBasicAuth {
		preProcessCredentials := []*dynamicproxy.BasicAuthUnhashedCredentials{}
		err = json.Unmarshal([]byte(cred), &preProcessCredentials)
		if err != nil {
			utils.SendErrorResponse(w, "invalid user credentials")
			return
		}

		//Check if there are empty password credentials
		for _, credObj := range preProcessCredentials {
			if strings.TrimSpace(credObj.Password) == "" {
				utils.SendErrorResponse(w, credObj.Username+" has empty password")
				return
			}
		}

		//Convert and hash the passwords
		for _, credObj := range preProcessCredentials {
			basicAuthCredentials = append(basicAuthCredentials, &dynamicproxy.BasicAuthCredentials{
				Username:     credObj.Username,
				PasswordHash: auth.Hash(credObj.Password),
			})
		}
	}

	rootname := ""
	if eptype == "vdir" {
		vdir, err := utils.PostPara(r, "rootname")
		if err != nil {
			utils.SendErrorResponse(w, "vdir not defined")
			return
		}

		//Vdir must start with /
		if !strings.HasPrefix(vdir, "/") {
			vdir = "/" + vdir
		}
		rootname = vdir

		thisOption := dynamicproxy.VdirOptions{
			RootName:             vdir,
			Domain:               endpoint,
			RequireTLS:           useTLS,
			BypassGlobalTLS:      useBypassGlobalTLS,
			SkipCertValidations:  skipTlsValidation,
			RequireBasicAuth:     requireBasicAuth,
			BasicAuthCredentials: basicAuthCredentials,
		}
		dynamicProxyRouter.AddVirtualDirectoryProxyService(&thisOption)

	} else if eptype == "subd" {
		subdomain, err := utils.PostPara(r, "rootname")
		if err != nil {
			utils.SendErrorResponse(w, "subdomain not defined")
			return
		}
		rootname = subdomain
		thisOption := dynamicproxy.SubdOptions{
			MatchingDomain:       subdomain,
			Domain:               endpoint,
			RequireTLS:           useTLS,
			BypassGlobalTLS:      useBypassGlobalTLS,
			SkipCertValidations:  skipTlsValidation,
			RequireBasicAuth:     requireBasicAuth,
			BasicAuthCredentials: basicAuthCredentials,
		}
		dynamicProxyRouter.AddSubdomainRoutingService(&thisOption)
	} else if eptype == "root" {
		rootname = "root"
		thisOption := dynamicproxy.RootOptions{
			ProxyLocation: endpoint,
			RequireTLS:    useTLS,
		}
		dynamicProxyRouter.SetRootProxy(&thisOption)
	} else {
		//Invalid eptype
		utils.SendErrorResponse(w, "Invalid endpoint type")
		return
	}

	//Save it
	thisProxyConfigRecord := Record{
		ProxyType:            eptype,
		Rootname:             rootname,
		ProxyTarget:          endpoint,
		UseTLS:               useTLS,
		BypassGlobalTLS:      useBypassGlobalTLS,
		SkipTlsValidation:    skipTlsValidation,
		RequireBasicAuth:     requireBasicAuth,
		BasicAuthCredentials: basicAuthCredentials,
	}
	SaveReverseProxyConfigToFile(&thisProxyConfigRecord)

	//Update utm if exists
	if uptimeMonitor != nil {
		uptimeMonitor.Config.Targets = GetUptimeTargetsFromReverseProxyRules(dynamicProxyRouter)
		uptimeMonitor.CleanRecords()
	}

	utils.SendOK(w)
}

/*
ReverseProxyHandleEditEndpoint handles proxy endpoint edit
This endpoint do not handle
basic auth credential update. The credential
will be loaded from old config and reused
*/
func ReverseProxyHandleEditEndpoint(w http.ResponseWriter, r *http.Request) {
	eptype, err := utils.PostPara(r, "type") //Support root, vdir and subd
	if err != nil {
		utils.SendErrorResponse(w, "type not defined")
		return
	}

	rootNameOrMatchingDomain, err := utils.PostPara(r, "rootname")
	if err != nil {
		utils.SendErrorResponse(w, "Target proxy rule not defined")
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

	stv, _ := utils.PostPara(r, "tlsval")
	if stv == "" {
		stv = "false"
	}

	skipTlsValidation := (stv == "true")

	rba, _ := utils.PostPara(r, "bauth")
	if rba == "" {
		rba = "false"
	}

	requireBasicAuth := (rba == "true")

	//Load the previous basic auth credentials from current proxy rules
	targetProxyEntry, err := dynamicProxyRouter.LoadProxy(eptype, rootNameOrMatchingDomain)
	if err != nil {
		utils.SendErrorResponse(w, "Target proxy config not found or could not be loaded")
		return
	}

	if eptype == "vdir" {
		thisOption := dynamicproxy.VdirOptions{
			RootName:             targetProxyEntry.RootOrMatchingDomain,
			Domain:               endpoint,
			RequireTLS:           useTLS,
			SkipCertValidations:  skipTlsValidation,
			RequireBasicAuth:     requireBasicAuth,
			BasicAuthCredentials: targetProxyEntry.BasicAuthCredentials,
		}
		targetProxyEntry.Remove()
		dynamicProxyRouter.AddVirtualDirectoryProxyService(&thisOption)

	} else if eptype == "subd" {
		thisOption := dynamicproxy.SubdOptions{
			MatchingDomain:       targetProxyEntry.RootOrMatchingDomain,
			Domain:               endpoint,
			RequireTLS:           useTLS,
			SkipCertValidations:  skipTlsValidation,
			RequireBasicAuth:     requireBasicAuth,
			BasicAuthCredentials: targetProxyEntry.BasicAuthCredentials,
		}
		targetProxyEntry.Remove()
		dynamicProxyRouter.AddSubdomainRoutingService(&thisOption)
	}

	//Save it to file
	thisProxyConfigRecord := Record{
		ProxyType:            eptype,
		Rootname:             targetProxyEntry.RootOrMatchingDomain,
		ProxyTarget:          endpoint,
		UseTLS:               useTLS,
		SkipTlsValidation:    skipTlsValidation,
		RequireBasicAuth:     requireBasicAuth,
		BasicAuthCredentials: targetProxyEntry.BasicAuthCredentials,
	}
	SaveReverseProxyConfigToFile(&thisProxyConfigRecord)

	//Update uptime monitor
	UpdateUptimeMonitorTargets()

	utils.SendOK(w)
}

func DeleteProxyEndpoint(w http.ResponseWriter, r *http.Request) {
	ep, err := utils.GetPara(r, "ep")
	if err != nil {
		utils.SendErrorResponse(w, "Invalid ep given")
		return
	}

	ptype, err := utils.PostPara(r, "ptype")
	if err != nil {
		utils.SendErrorResponse(w, "Invalid ptype given")
		return
	}

	//Remove the config from runtime
	err = dynamicProxyRouter.RemoveProxyEndpointByRootname(ptype, ep)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Remove the config from file
	RemoveReverseProxyConfigFile(ep)

	//Update utm if exists
	if uptimeMonitor != nil {
		uptimeMonitor.Config.Targets = GetUptimeTargetsFromReverseProxyRules(dynamicProxyRouter)
		uptimeMonitor.CleanRecords()
	}

	//Update uptime monitor
	UpdateUptimeMonitorTargets()

	utils.SendOK(w)
}

/*
Handle update request for basic auth credential
Require paramter: ep (Endpoint) and pytype (proxy Type)
if request with GET, the handler will return current credentials
on this endpoint by its username

if request is POST, the handler will write the results to proxy config
*/
func UpdateProxyBasicAuthCredentials(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		ep, err := utils.GetPara(r, "ep")
		if err != nil {
			utils.SendErrorResponse(w, "Invalid ep given")
			return
		}

		ptype, err := utils.GetPara(r, "ptype")
		if err != nil {
			utils.SendErrorResponse(w, "Invalid ptype given")
			return
		}

		//Load the target proxy object from router
		targetProxy, err := dynamicProxyRouter.LoadProxy(ptype, ep)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}

		usernames := []string{}
		for _, cred := range targetProxy.BasicAuthCredentials {
			usernames = append(usernames, cred.Username)
		}

		js, _ := json.Marshal(usernames)
		utils.SendJSONResponse(w, string(js))

	} else if r.Method == http.MethodPost {
		//Write to target
		ep, err := utils.PostPara(r, "ep")
		if err != nil {
			utils.SendErrorResponse(w, "Invalid ep given")
			return
		}

		ptype, err := utils.PostPara(r, "ptype")
		if err != nil {
			utils.SendErrorResponse(w, "Invalid ptype given")
			return
		}

		if ptype != "vdir" && ptype != "subd" {
			utils.SendErrorResponse(w, "Invalid ptype given")
			return
		}

		creds, err := utils.PostPara(r, "creds")
		if err != nil {
			utils.SendErrorResponse(w, "Invalid ptype given")
			return
		}

		//Load the target proxy object from router
		targetProxy, err := dynamicProxyRouter.LoadProxy(ptype, ep)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}

		//Try to marshal the content of creds into the suitable structure
		newCredentials := []*dynamicproxy.BasicAuthUnhashedCredentials{}
		err = json.Unmarshal([]byte(creds), &newCredentials)
		if err != nil {
			utils.SendErrorResponse(w, "Malformed credential data")
			return
		}

		//Merge the credentials into the original config
		//If a new username exists in old config with no pw given, keep the old pw hash
		//If a new username is found with new password, hash it and push to credential slice
		mergedCredentials := []*dynamicproxy.BasicAuthCredentials{}
		for _, credential := range newCredentials {
			if credential.Password == "" {
				//Check if exists in the old credential files
				keepUnchange := false
				for _, oldCredEntry := range targetProxy.BasicAuthCredentials {
					if oldCredEntry.Username == credential.Username {
						//Exists! Reuse the old hash
						mergedCredentials = append(mergedCredentials, &dynamicproxy.BasicAuthCredentials{
							Username:     oldCredEntry.Username,
							PasswordHash: oldCredEntry.PasswordHash,
						})
						keepUnchange = true
					}
				}

				if !keepUnchange {
					//This is a new username with no pw given
					utils.SendErrorResponse(w, "Access password for "+credential.Username+" is empty!")
					return
				}
			} else {
				//This username have given password
				mergedCredentials = append(mergedCredentials, &dynamicproxy.BasicAuthCredentials{
					Username:     credential.Username,
					PasswordHash: auth.Hash(credential.Password),
				})
			}
		}

		targetProxy.BasicAuthCredentials = mergedCredentials

		//Save it to file
		SaveReverseProxyEndpointToFile(targetProxy)

		//Replace runtime configuration
		targetProxy.UpdateToRuntime()
		utils.SendOK(w)
	} else {
		http.Error(w, "invalid usage", http.StatusMethodNotAllowed)
	}

}

// List, Update or Remove the exception paths for basic auth.
func ListProxyBasicAuthExceptionPaths(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
	ep, err := utils.GetPara(r, "ep")
	if err != nil {
		utils.SendErrorResponse(w, "Invalid ep given")
		return
	}

	ptype, err := utils.GetPara(r, "ptype")
	if err != nil {
		utils.SendErrorResponse(w, "Invalid ptype given")
		return
	}

	//Load the target proxy object from router
	targetProxy, err := dynamicProxyRouter.LoadProxy(ptype, ep)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//List all the exception paths for this proxy
	results := targetProxy.BasicAuthExceptionRules
	if results == nil {
		//It is a config from a really old version of zoraxy. Overwrite it with empty array
		results = []*dynamicproxy.BasicAuthExceptionRule{}
	}
	js, _ := json.Marshal(results)
	utils.SendJSONResponse(w, string(js))
	return
}

func AddProxyBasicAuthExceptionPaths(w http.ResponseWriter, r *http.Request) {
	ep, err := utils.PostPara(r, "ep")
	if err != nil {
		utils.SendErrorResponse(w, "Invalid ep given")
		return
	}

	ptype, err := utils.PostPara(r, "ptype")
	if err != nil {
		utils.SendErrorResponse(w, "Invalid ptype given")
		return
	}

	matchingPrefix, err := utils.PostPara(r, "prefix")
	if err != nil {
		utils.SendErrorResponse(w, "Invalid matching prefix given")
		return
	}

	//Load the target proxy object from router
	targetProxy, err := dynamicProxyRouter.LoadProxy(ptype, ep)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Check if the prefix starts with /. If not, prepend it
	if !strings.HasPrefix(matchingPrefix, "/") {
		matchingPrefix = "/" + matchingPrefix
	}

	//Add a new exception rule if it is not already exists
	alreadyExists := false
	for _, thisExceptionRule := range targetProxy.BasicAuthExceptionRules {
		if thisExceptionRule.PathPrefix == matchingPrefix {
			alreadyExists = true
			break
		}
	}
	if alreadyExists {
		utils.SendErrorResponse(w, "This matching path already exists")
		return
	}
	targetProxy.BasicAuthExceptionRules = append(targetProxy.BasicAuthExceptionRules, &dynamicproxy.BasicAuthExceptionRule{
		PathPrefix: strings.TrimSpace(matchingPrefix),
	})

	//Save configs to runtime and file
	targetProxy.UpdateToRuntime()
	SaveReverseProxyEndpointToFile(targetProxy)

	utils.SendOK(w)
}

func RemoveProxyBasicAuthExceptionPaths(w http.ResponseWriter, r *http.Request) {
	// Delete a rule
	ep, err := utils.PostPara(r, "ep")
	if err != nil {
		utils.SendErrorResponse(w, "Invalid ep given")
		return
	}

	ptype, err := utils.PostPara(r, "ptype")
	if err != nil {
		utils.SendErrorResponse(w, "Invalid ptype given")
		return
	}

	matchingPrefix, err := utils.PostPara(r, "prefix")
	if err != nil {
		utils.SendErrorResponse(w, "Invalid matching prefix given")
		return
	}

	// Load the target proxy object from router
	targetProxy, err := dynamicProxyRouter.LoadProxy(ptype, ep)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	newExceptionRuleList := []*dynamicproxy.BasicAuthExceptionRule{}
	matchingExists := false
	for _, thisExceptionalRule := range targetProxy.BasicAuthExceptionRules {
		if thisExceptionalRule.PathPrefix != matchingPrefix {
			newExceptionRuleList = append(newExceptionRuleList, thisExceptionalRule)
		} else {
			matchingExists = true
		}
	}

	if !matchingExists {
		utils.SendErrorResponse(w, "target matching rule not exists")
		return
	}

	targetProxy.BasicAuthExceptionRules = newExceptionRuleList

	// Save configs to runtime and file
	targetProxy.UpdateToRuntime()
	SaveReverseProxyEndpointToFile(targetProxy)

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
		results := []*dynamicproxy.ProxyEndpoint{}
		dynamicProxyRouter.SubdomainEndpoint.Range(func(key, value interface{}) bool {
			results = append(results, value.(*dynamicproxy.ProxyEndpoint))
			return true
		})

		sort.Slice(results, func(i, j int) bool {
			return results[i].RootOrMatchingDomain < results[j].RootOrMatchingDomain
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
		if dynamicProxyRouter.Option.Port == 80 {
			utils.SendErrorResponse(w, "This option is not available when listening on port 80")
			return
		}
		if useRedirect == "true" {
			sysdb.Write("settings", "redirect", true)
			SystemWideLogger.Println("Updating force HTTPS redirection to true")
			dynamicProxyRouter.UpdateHttpToHttpsRedirectSetting(true)
		} else if useRedirect == "false" {
			sysdb.Write("settings", "redirect", false)
			SystemWideLogger.Println("Updating force HTTPS redirection to false")
			dynamicProxyRouter.UpdateHttpToHttpsRedirectSetting(false)
		}

		utils.SendOK(w)
	}
}

// Handle checking if the current user is accessing via the reverse proxied interface
// Of the management interface.
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
		utils.SendErrorResponse(w, "Invalid incoming port given")
		return
	}

	//Check if it is identical as proxy root (recursion!)
	if dynamicProxyRouter.Root == nil || dynamicProxyRouter.Root.Domain == "" {
		//Check if proxy root is set before checking recursive listen
		//Fixing issue #43
		utils.SendErrorResponse(w, "Set Proxy Root before changing inbound port")
		return
	}

	proxyRoot := strings.TrimSuffix(dynamicProxyRouter.Root.Domain, "/")
	if strings.HasPrefix(proxyRoot, "localhost:"+strconv.Itoa(newIncomingPortInt)) || strings.HasPrefix(proxyRoot, "127.0.0.1:"+strconv.Itoa(newIncomingPortInt)) {
		//Listening port is same as proxy root
		//Not allow recursive settings
		utils.SendErrorResponse(w, "Recursive listening port! Check your proxy root settings.")
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

// Handle list of root route options
func HandleRootRouteOptionList(w http.ResponseWriter, r *http.Request) {
	js, _ := json.Marshal(dynamicProxyRouter.RootRoutingOptions)
	utils.SendJSONResponse(w, string(js))
}

// Handle update of the root route edge case options. See dynamicproxy/rootRoute.go
func HandleRootRouteOptionsUpdate(w http.ResponseWriter, r *http.Request) {
	enableUnsetSubdomainRedirect, err := utils.PostBool(r, "unsetRedirect")
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	unsetRedirectTarget, _ := utils.PostPara(r, "unsetRedirectTarget")

	newRootOption := dynamicproxy.RootRoutingOptions{
		EnableRedirectForUnsetRules: enableUnsetSubdomainRedirect,
		UnsetRuleRedirectTarget:     unsetRedirectTarget,
	}

	dynamicProxyRouter.RootRoutingOptions = &newRootOption
	err = newRootOption.SaveToFile()
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}
