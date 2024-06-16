package main

import (
	"encoding/json"
	"net/http"
	"net/http/pprof"

	"imuslab.com/zoraxy/mod/acme/acmedns"
	"imuslab.com/zoraxy/mod/acme/acmewizard"
	"imuslab.com/zoraxy/mod/auth"
	"imuslab.com/zoraxy/mod/netstat"
	"imuslab.com/zoraxy/mod/netutils"
	"imuslab.com/zoraxy/mod/utils"
)

/*
	API.go

	This file contains all the API called by the web management interface

*/

var requireAuth = true

func initAPIs() {

	authRouter := auth.NewManagedHTTPRouter(auth.RouterOption{
		AuthAgent:   authAgent,
		RequireAuth: requireAuth,
		DeniedHandler: func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "401 - Unauthorized", http.StatusUnauthorized)
		},
	})

	//Register the standard web services urls
	fs := http.FileServer(http.FS(webres))
	if development {
		fs = http.FileServer(http.Dir("web/"))
	}
	//Add a layer of middleware for advance  control
	advHandler := FSHandler(fs)
	http.Handle("/", advHandler)

	//Authentication APIs
	registerAuthAPIs(requireAuth)

	//Reverse proxy
	authRouter.HandleFunc("/api/proxy/enable", ReverseProxyHandleOnOff)
	authRouter.HandleFunc("/api/proxy/add", ReverseProxyHandleAddEndpoint)
	authRouter.HandleFunc("/api/proxy/status", ReverseProxyStatus)
	authRouter.HandleFunc("/api/proxy/toggle", ReverseProxyToggleRuleSet)
	authRouter.HandleFunc("/api/proxy/list", ReverseProxyList)
	authRouter.HandleFunc("/api/proxy/detail", ReverseProxyListDetail)
	authRouter.HandleFunc("/api/proxy/edit", ReverseProxyHandleEditEndpoint)
	authRouter.HandleFunc("/api/proxy/setAlias", ReverseProxyHandleAlias)
	authRouter.HandleFunc("/api/proxy/del", DeleteProxyEndpoint)
	authRouter.HandleFunc("/api/proxy/updateCredentials", UpdateProxyBasicAuthCredentials)
	authRouter.HandleFunc("/api/proxy/tlscheck", HandleCheckSiteSupportTLS)
	authRouter.HandleFunc("/api/proxy/setIncoming", HandleIncomingPortSet)
	authRouter.HandleFunc("/api/proxy/useHttpsRedirect", HandleUpdateHttpsRedirect)
	authRouter.HandleFunc("/api/proxy/listenPort80", HandleUpdatePort80Listener)
	authRouter.HandleFunc("/api/proxy/requestIsProxied", HandleManagementProxyCheck)
	authRouter.HandleFunc("/api/proxy/developmentMode", HandleDevelopmentModeChange)
	//Reverse proxy virtual directory APIs
	authRouter.HandleFunc("/api/proxy/vdir/list", ReverseProxyListVdir)
	authRouter.HandleFunc("/api/proxy/vdir/add", ReverseProxyAddVdir)
	authRouter.HandleFunc("/api/proxy/vdir/del", ReverseProxyDeleteVdir)
	authRouter.HandleFunc("/api/proxy/vdir/edit", ReverseProxyEditVdir)
	//Reverse proxy user define header apis
	authRouter.HandleFunc("/api/proxy/header/list", HandleCustomHeaderList)
	authRouter.HandleFunc("/api/proxy/header/add", HandleCustomHeaderAdd)
	authRouter.HandleFunc("/api/proxy/header/remove", HandleCustomHeaderRemove)
	authRouter.HandleFunc("/api/proxy/header/handleHSTS", HandleHSTSState)
	authRouter.HandleFunc("/api/proxy/header/handlePermissionPolicy", HandlePermissionPolicy)
	//Reverse proxy auth related APIs
	authRouter.HandleFunc("/api/proxy/auth/exceptions/list", ListProxyBasicAuthExceptionPaths)
	authRouter.HandleFunc("/api/proxy/auth/exceptions/add", AddProxyBasicAuthExceptionPaths)
	authRouter.HandleFunc("/api/proxy/auth/exceptions/delete", RemoveProxyBasicAuthExceptionPaths)

	//TLS / SSL config
	authRouter.HandleFunc("/api/cert/tls", handleToggleTLSProxy)
	authRouter.HandleFunc("/api/cert/tlsRequireLatest", handleSetTlsRequireLatest)
	authRouter.HandleFunc("/api/cert/upload", handleCertUpload)
	authRouter.HandleFunc("/api/cert/list", handleListCertificate)
	authRouter.HandleFunc("/api/cert/listdomains", handleListDomains)
	authRouter.HandleFunc("/api/cert/checkDefault", handleDefaultCertCheck)
	authRouter.HandleFunc("/api/cert/delete", handleCertRemove)

	//Redirection config
	authRouter.HandleFunc("/api/redirect/list", handleListRedirectionRules)
	authRouter.HandleFunc("/api/redirect/add", handleAddRedirectionRule)
	authRouter.HandleFunc("/api/redirect/delete", handleDeleteRedirectionRule)
	authRouter.HandleFunc("/api/redirect/regex", handleToggleRedirectRegexpSupport)

	//Access Rules API
	authRouter.HandleFunc("/api/access/list", handleListAccessRules)
	authRouter.HandleFunc("/api/access/attach", handleAttachRuleToHost)
	authRouter.HandleFunc("/api/access/create", handleCreateAccessRule)
	authRouter.HandleFunc("/api/access/remove", handleRemoveAccessRule)
	authRouter.HandleFunc("/api/access/update", handleUpadateAccessRule)
	//Blacklist APIs
	authRouter.HandleFunc("/api/blacklist/list", handleListBlacklisted)
	authRouter.HandleFunc("/api/blacklist/country/add", handleCountryBlacklistAdd)
	authRouter.HandleFunc("/api/blacklist/country/remove", handleCountryBlacklistRemove)
	authRouter.HandleFunc("/api/blacklist/ip/add", handleIpBlacklistAdd)
	authRouter.HandleFunc("/api/blacklist/ip/remove", handleIpBlacklistRemove)
	authRouter.HandleFunc("/api/blacklist/enable", handleBlacklistEnable)
	//Whitelist APIs
	authRouter.HandleFunc("/api/whitelist/list", handleListWhitelisted)
	authRouter.HandleFunc("/api/whitelist/country/add", handleCountryWhitelistAdd)
	authRouter.HandleFunc("/api/whitelist/country/remove", handleCountryWhitelistRemove)
	authRouter.HandleFunc("/api/whitelist/ip/add", handleIpWhitelistAdd)
	authRouter.HandleFunc("/api/whitelist/ip/remove", handleIpWhitelistRemove)
	authRouter.HandleFunc("/api/whitelist/enable", handleWhitelistEnable)

	//Path Blocker APIs
	authRouter.HandleFunc("/api/pathrule/add", pathRuleHandler.HandleAddBlockingPath)
	authRouter.HandleFunc("/api/pathrule/list", pathRuleHandler.HandleListBlockingPath)
	authRouter.HandleFunc("/api/pathrule/remove", pathRuleHandler.HandleRemoveBlockingPath)

	//Statistic & uptime monitoring API
	authRouter.HandleFunc("/api/stats/summary", statisticCollector.HandleTodayStatLoad)
	authRouter.HandleFunc("/api/stats/countries", HandleCountryDistrSummary)
	authRouter.HandleFunc("/api/stats/netstat", netstat.HandleGetNetworkInterfaceStats)
	authRouter.HandleFunc("/api/stats/netstatgraph", netstatBuffers.HandleGetBufferedNetworkInterfaceStats)
	authRouter.HandleFunc("/api/stats/listnic", netstat.HandleListNetworkInterfaces)
	authRouter.HandleFunc("/api/utm/list", HandleUptimeMonitorListing)

	//Global Area Network APIs
	authRouter.HandleFunc("/api/gan/network/info", ganManager.HandleGetNodeID)
	authRouter.HandleFunc("/api/gan/network/add", ganManager.HandleAddNetwork)
	authRouter.HandleFunc("/api/gan/network/remove", ganManager.HandleRemoveNetwork)
	authRouter.HandleFunc("/api/gan/network/list", ganManager.HandleListNetwork)
	authRouter.HandleFunc("/api/gan/network/name", ganManager.HandleNetworkNaming)
	//authRouter.HandleFunc("/api/gan/network/detail", ganManager.HandleNetworkDetails)
	authRouter.HandleFunc("/api/gan/network/setRange", ganManager.HandleSetRanges)
	authRouter.HandleFunc("/api/gan/network/join", ganManager.HandleServerJoinNetwork)
	authRouter.HandleFunc("/api/gan/network/leave", ganManager.HandleServerLeaveNetwork)
	authRouter.HandleFunc("/api/gan/members/list", ganManager.HandleMemberList)
	authRouter.HandleFunc("/api/gan/members/ip", ganManager.HandleMemberIP)
	authRouter.HandleFunc("/api/gan/members/name", ganManager.HandleMemberNaming)
	authRouter.HandleFunc("/api/gan/members/authorize", ganManager.HandleMemberAuthorization)
	authRouter.HandleFunc("/api/gan/members/delete", ganManager.HandleMemberDelete)

	//TCP Proxy
	authRouter.HandleFunc("/api/streamprox/config/add", streamProxyManager.HandleAddProxyConfig)
	authRouter.HandleFunc("/api/streamprox/config/edit", streamProxyManager.HandleEditProxyConfigs)
	authRouter.HandleFunc("/api/streamprox/config/list", streamProxyManager.HandleListConfigs)
	authRouter.HandleFunc("/api/streamprox/config/start", streamProxyManager.HandleStartProxy)
	authRouter.HandleFunc("/api/streamprox/config/stop", streamProxyManager.HandleStopProxy)
	authRouter.HandleFunc("/api/streamprox/config/delete", streamProxyManager.HandleRemoveProxy)
	authRouter.HandleFunc("/api/streamprox/config/status", streamProxyManager.HandleGetProxyStatus)

	//mDNS APIs
	authRouter.HandleFunc("/api/mdns/list", HandleMdnsListing)
	authRouter.HandleFunc("/api/mdns/discover", HandleMdnsScanning)

	//Zoraxy Analytic
	authRouter.HandleFunc("/api/analytic/list", AnalyticLoader.HandleSummaryList)
	authRouter.HandleFunc("/api/analytic/load", AnalyticLoader.HandleLoadTargetDaySummary)
	authRouter.HandleFunc("/api/analytic/loadRange", AnalyticLoader.HandleLoadTargetRangeSummary)
	authRouter.HandleFunc("/api/analytic/exportRange", AnalyticLoader.HandleRangeExport)
	authRouter.HandleFunc("/api/analytic/resetRange", AnalyticLoader.HandleRangeReset)

	//Network utilities
	authRouter.HandleFunc("/api/tools/ipscan", HandleIpScan)
	authRouter.HandleFunc("/api/tools/traceroute", netutils.HandleTraceRoute)
	authRouter.HandleFunc("/api/tools/ping", netutils.HandlePing)
	authRouter.HandleFunc("/api/tools/whois", netutils.HandleWhois)
	authRouter.HandleFunc("/api/tools/webssh", HandleCreateProxySession)
	authRouter.HandleFunc("/api/tools/websshSupported", HandleWebSshSupportCheck)
	authRouter.HandleFunc("/api/tools/wol", HandleWakeOnLan)
	authRouter.HandleFunc("/api/tools/smtp/get", HandleSMTPGet)
	authRouter.HandleFunc("/api/tools/smtp/set", HandleSMTPSet)
	authRouter.HandleFunc("/api/tools/smtp/admin", HandleAdminEmailGet)
	authRouter.HandleFunc("/api/tools/smtp/test", HandleTestEmailSend)
	authRouter.HandleFunc("/api/tools/fwdproxy/enable", forwardProxy.HandleToogle)
	authRouter.HandleFunc("/api/tools/fwdproxy/port", forwardProxy.HandlePort)

	//Account Reset
	http.HandleFunc("/api/account/reset", HandleAdminAccountResetEmail)
	http.HandleFunc("/api/account/new", HandleNewPasswordSetup)

	//ACME & Auto Renewer
	authRouter.HandleFunc("/api/acme/listExpiredDomains", acmeHandler.HandleGetExpiredDomains)
	authRouter.HandleFunc("/api/acme/obtainCert", AcmeCheckAndHandleRenewCertificate)
	authRouter.HandleFunc("/api/acme/autoRenew/enable", acmeAutoRenewer.HandleAutoRenewEnable)
	authRouter.HandleFunc("/api/acme/autoRenew/ca", HandleACMEPreferredCA)
	authRouter.HandleFunc("/api/acme/autoRenew/email", acmeAutoRenewer.HandleACMEEmail)
	authRouter.HandleFunc("/api/acme/autoRenew/setDomains", acmeAutoRenewer.HandleSetAutoRenewDomains)
	authRouter.HandleFunc("/api/acme/autoRenew/setEAB", acmeAutoRenewer.HanldeSetEAB)
	authRouter.HandleFunc("/api/acme/autoRenew/setDNS", acmeAutoRenewer.HanldeSetDNS)
	authRouter.HandleFunc("/api/acme/autoRenew/listDomains", acmeAutoRenewer.HandleLoadAutoRenewDomains)
	authRouter.HandleFunc("/api/acme/autoRenew/renewPolicy", acmeAutoRenewer.HandleRenewPolicy)
	authRouter.HandleFunc("/api/acme/autoRenew/renewNow", acmeAutoRenewer.HandleRenewNow)
	authRouter.HandleFunc("/api/acme/dns/providers", acmedns.HandleServeProvidersJson)
	authRouter.HandleFunc("/api/acme/wizard", acmewizard.HandleGuidedStepCheck) //ACME Wizard

	//Static Web Server
	authRouter.HandleFunc("/api/webserv/status", staticWebServer.HandleGetStatus)
	authRouter.HandleFunc("/api/webserv/start", staticWebServer.HandleStartServer)
	authRouter.HandleFunc("/api/webserv/stop", staticWebServer.HandleStopServer)
	authRouter.HandleFunc("/api/webserv/setPort", HandleStaticWebServerPortChange)
	authRouter.HandleFunc("/api/webserv/setDirList", staticWebServer.SetEnableDirectoryListing)
	if *allowWebFileManager {
		//Web Directory Manager file operation functions
		authRouter.HandleFunc("/api/fs/list", staticWebServer.FileManager.HandleList)
		authRouter.HandleFunc("/api/fs/upload", staticWebServer.FileManager.HandleUpload)
		authRouter.HandleFunc("/api/fs/download", staticWebServer.FileManager.HandleDownload)
		authRouter.HandleFunc("/api/fs/newFolder", staticWebServer.FileManager.HandleNewFolder)
		authRouter.HandleFunc("/api/fs/copy", staticWebServer.FileManager.HandleFileCopy)
		authRouter.HandleFunc("/api/fs/move", staticWebServer.FileManager.HandleFileMove)
		authRouter.HandleFunc("/api/fs/properties", staticWebServer.FileManager.HandleFileProperties)
		authRouter.HandleFunc("/api/fs/del", staticWebServer.FileManager.HandleFileDelete)
	}

	//Docker UX Optimizations
	authRouter.HandleFunc("/api/docker/available", HandleDockerAvailable)
	authRouter.HandleFunc("/api/docker/containers", HandleDockerContainersList)

	//Others
	http.HandleFunc("/api/info/x", HandleZoraxyInfo)
	authRouter.HandleFunc("/api/info/geoip", HandleGeoIpLookup)
	authRouter.HandleFunc("/api/conf/export", ExportConfigAsZip)
	authRouter.HandleFunc("/api/conf/import", ImportConfigFromZip)

	//Debug
	authRouter.HandleFunc("/api/info/pprof", pprof.Index)

	//If you got APIs to add, append them here
	// get available docker containers

}

// Function to renders Auth related APIs
func registerAuthAPIs(requireAuth bool) {
	//Auth APIs
	http.HandleFunc("/api/auth/login", authAgent.HandleLogin)
	http.HandleFunc("/api/auth/logout", authAgent.HandleLogout)
	http.HandleFunc("/api/auth/checkLogin", func(w http.ResponseWriter, r *http.Request) {
		if requireAuth {
			authAgent.CheckLogin(w, r)
		} else {
			utils.SendJSONResponse(w, "true")
		}
	})
	http.HandleFunc("/api/auth/username", func(w http.ResponseWriter, r *http.Request) {
		username, err := authAgent.GetUserName(w, r)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}

		js, _ := json.Marshal(username)
		utils.SendJSONResponse(w, string(js))
	})
	http.HandleFunc("/api/auth/userCount", func(w http.ResponseWriter, r *http.Request) {
		uc := authAgent.GetUserCounts()
		js, _ := json.Marshal(uc)
		utils.SendJSONResponse(w, string(js))
	})
	http.HandleFunc("/api/auth/register", func(w http.ResponseWriter, r *http.Request) {
		if authAgent.GetUserCounts() == 0 {
			//Allow register root admin
			authAgent.HandleRegisterWithoutEmail(w, r, func(username, reserved string) {

			})
		} else {
			//This function is disabled
			utils.SendErrorResponse(w, "Root management account already exists")
		}
	})
	http.HandleFunc("/api/auth/changePassword", func(w http.ResponseWriter, r *http.Request) {
		username, err := authAgent.GetUserName(w, r)
		if err != nil {
			http.Error(w, "401 - Unauthorized", http.StatusUnauthorized)
			return
		}

		oldPassword, err := utils.PostPara(r, "oldPassword")
		if err != nil {
			utils.SendErrorResponse(w, "empty current password")
			return
		}
		newPassword, err := utils.PostPara(r, "newPassword")
		if err != nil {
			utils.SendErrorResponse(w, "empty new password")
			return
		}
		confirmPassword, _ := utils.PostPara(r, "confirmPassword")

		if newPassword != confirmPassword {
			utils.SendErrorResponse(w, "confirm password not match")
			return
		}

		//Check if the old password correct
		oldPasswordCorrect, _ := authAgent.ValidateUsernameAndPasswordWithReason(username, oldPassword)
		if !oldPasswordCorrect {
			utils.SendErrorResponse(w, "Invalid current password given")
			return
		}

		//Change the password of the root user
		authAgent.UnregisterUser(username)
		authAgent.CreateUserAccount(username, newPassword, "")
	})

}
