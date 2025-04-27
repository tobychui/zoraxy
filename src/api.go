package main

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/pprof"

	"imuslab.com/zoraxy/mod/acme/acmedns"
	"imuslab.com/zoraxy/mod/acme/acmewizard"
	"imuslab.com/zoraxy/mod/auth"
	"imuslab.com/zoraxy/mod/dynamicproxy/domainsniff"
	"imuslab.com/zoraxy/mod/ipscan"
	"imuslab.com/zoraxy/mod/netstat"
	"imuslab.com/zoraxy/mod/netutils"
	"imuslab.com/zoraxy/mod/utils"
)

/*
	API.go

	This file contains all the API called by the web management interface
*/

// Register the APIs for HTTP proxy management functions
func RegisterHTTPProxyAPIs(authRouter *auth.RouterDef) {
	/* Reverse Proxy Settings & Status */
	authRouter.HandleFunc("/api/proxy/enable", ReverseProxyHandleOnOff)
	authRouter.HandleFunc("/api/proxy/add", ReverseProxyHandleAddEndpoint)
	authRouter.HandleFunc("/api/proxy/status", ReverseProxyStatus)
	authRouter.HandleFunc("/api/proxy/toggle", ReverseProxyToggleRuleSet)
	authRouter.HandleFunc("/api/proxy/list", ReverseProxyList)
	authRouter.HandleFunc("/api/proxy/listTags", ReverseProxyListTags)
	authRouter.HandleFunc("/api/proxy/detail", ReverseProxyListDetail)
	authRouter.HandleFunc("/api/proxy/edit", ReverseProxyHandleEditEndpoint)
	authRouter.HandleFunc("/api/proxy/setAlias", ReverseProxyHandleAlias)
	authRouter.HandleFunc("/api/proxy/del", DeleteProxyEndpoint)
	authRouter.HandleFunc("/api/proxy/updateCredentials", UpdateProxyBasicAuthCredentials)
	authRouter.HandleFunc("/api/proxy/tlscheck", domainsniff.HandleCheckSiteSupportTLS)
	authRouter.HandleFunc("/api/proxy/setIncoming", HandleIncomingPortSet)
	authRouter.HandleFunc("/api/proxy/useHttpsRedirect", HandleUpdateHttpsRedirect)
	authRouter.HandleFunc("/api/proxy/listenPort80", HandleUpdatePort80Listener)
	authRouter.HandleFunc("/api/proxy/requestIsProxied", HandleManagementProxyCheck)
	authRouter.HandleFunc("/api/proxy/developmentMode", HandleDevelopmentModeChange)
	/* Reverse proxy upstream (load balance) */
	authRouter.HandleFunc("/api/proxy/upstream/list", ReverseProxyUpstreamList)
	authRouter.HandleFunc("/api/proxy/upstream/add", ReverseProxyUpstreamAdd)
	authRouter.HandleFunc("/api/proxy/upstream/setPriority", ReverseProxyUpstreamSetPriority)
	authRouter.HandleFunc("/api/proxy/upstream/update", ReverseProxyUpstreamUpdate)
	authRouter.HandleFunc("/api/proxy/upstream/remove", ReverseProxyUpstreamDelete)
	/* Reverse proxy virtual directory */
	authRouter.HandleFunc("/api/proxy/vdir/list", ReverseProxyListVdir)
	authRouter.HandleFunc("/api/proxy/vdir/add", ReverseProxyAddVdir)
	authRouter.HandleFunc("/api/proxy/vdir/del", ReverseProxyDeleteVdir)
	authRouter.HandleFunc("/api/proxy/vdir/edit", ReverseProxyEditVdir)
	/* Reverse proxy user-defined header */
	authRouter.HandleFunc("/api/proxy/header/list", HandleCustomHeaderList)
	authRouter.HandleFunc("/api/proxy/header/add", HandleCustomHeaderAdd)
	authRouter.HandleFunc("/api/proxy/header/remove", HandleCustomHeaderRemove)
	authRouter.HandleFunc("/api/proxy/header/handleHSTS", HandleHSTSState)
	authRouter.HandleFunc("/api/proxy/header/handleHopByHop", HandleHopByHop)
	authRouter.HandleFunc("/api/proxy/header/handleHostOverwrite", HandleHostOverwrite)
	authRouter.HandleFunc("/api/proxy/header/handlePermissionPolicy", HandlePermissionPolicy)
	authRouter.HandleFunc("/api/proxy/header/handleWsHeaderBehavior", HandleWsHeaderBehavior)
	/* Reverse proxy auth related */
	authRouter.HandleFunc("/api/proxy/auth/exceptions/list", ListProxyBasicAuthExceptionPaths)
	authRouter.HandleFunc("/api/proxy/auth/exceptions/add", AddProxyBasicAuthExceptionPaths)
	authRouter.HandleFunc("/api/proxy/auth/exceptions/delete", RemoveProxyBasicAuthExceptionPaths)
}

// Register the APIs for TLS / SSL certificate management functions
func RegisterTLSAPIs(authRouter *auth.RouterDef) {
	authRouter.HandleFunc("/api/cert/tls", handleToggleTLSProxy)
	authRouter.HandleFunc("/api/cert/tlsRequireLatest", handleSetTlsRequireLatest)
	authRouter.HandleFunc("/api/cert/upload", handleCertUpload)
	authRouter.HandleFunc("/api/cert/download", handleCertDownload)
	authRouter.HandleFunc("/api/cert/list", handleListCertificate)
	authRouter.HandleFunc("/api/cert/listdomains", handleListDomains)
	authRouter.HandleFunc("/api/cert/checkDefault", handleDefaultCertCheck)
	authRouter.HandleFunc("/api/cert/delete", handleCertRemove)
}

// Register the APIs for Authentication handlers like Authelia and OAUTH2
func RegisterAuthenticationHandlerAPIs(authRouter *auth.RouterDef) {
	authRouter.HandleFunc("/api/sso/Authelia", autheliaRouter.HandleSetAutheliaURLAndHTTPS)
	authRouter.HandleFunc("/api/sso/Authentik", authentikRouter.HandleSetAuthentikURLAndHTTPS)
}

// Register the APIs for redirection rules management functions
func RegisterRedirectionAPIs(authRouter *auth.RouterDef) {
	authRouter.HandleFunc("/api/redirect/list", handleListRedirectionRules)
	authRouter.HandleFunc("/api/redirect/add", handleAddRedirectionRule)
	authRouter.HandleFunc("/api/redirect/delete", handleDeleteRedirectionRule)
	authRouter.HandleFunc("/api/redirect/edit", handleEditRedirectionRule)
	authRouter.HandleFunc("/api/redirect/regex", handleToggleRedirectRegexpSupport)
}

// Register the APIs for access rules management functions
func RegisterAccessRuleAPIs(authRouter *auth.RouterDef) {
	/* Access Rules Settings & Status */
	authRouter.HandleFunc("/api/access/list", handleListAccessRules)
	authRouter.HandleFunc("/api/access/attach", handleAttachRuleToHost)
	authRouter.HandleFunc("/api/access/create", handleCreateAccessRule)
	authRouter.HandleFunc("/api/access/remove", handleRemoveAccessRule)
	authRouter.HandleFunc("/api/access/update", handleUpadateAccessRule)
	/* Blacklist */
	authRouter.HandleFunc("/api/blacklist/list", handleListBlacklisted)
	authRouter.HandleFunc("/api/blacklist/country/add", handleCountryBlacklistAdd)
	authRouter.HandleFunc("/api/blacklist/country/remove", handleCountryBlacklistRemove)
	authRouter.HandleFunc("/api/blacklist/ip/add", handleIpBlacklistAdd)
	authRouter.HandleFunc("/api/blacklist/ip/remove", handleIpBlacklistRemove)
	authRouter.HandleFunc("/api/blacklist/enable", handleBlacklistEnable)
	/* Whitelist */
	authRouter.HandleFunc("/api/whitelist/list", handleListWhitelisted)
	authRouter.HandleFunc("/api/whitelist/country/add", handleCountryWhitelistAdd)
	authRouter.HandleFunc("/api/whitelist/country/remove", handleCountryWhitelistRemove)
	authRouter.HandleFunc("/api/whitelist/ip/add", handleIpWhitelistAdd)
	authRouter.HandleFunc("/api/whitelist/ip/remove", handleIpWhitelistRemove)
	authRouter.HandleFunc("/api/whitelist/enable", handleWhitelistEnable)
	authRouter.HandleFunc("/api/whitelist/allowLocal", handleWhitelistAllowLoopback)
	/* Quick Ban List */
	authRouter.HandleFunc("/api/quickban/list", handleListQuickBan)
}

// Register the APIs for path blocking rules management functions, WIP
func RegisterPathRuleAPIs(authRouter *auth.RouterDef) {
	authRouter.HandleFunc("/api/pathrule/add", pathRuleHandler.HandleAddBlockingPath)
	authRouter.HandleFunc("/api/pathrule/list", pathRuleHandler.HandleListBlockingPath)
	authRouter.HandleFunc("/api/pathrule/remove", pathRuleHandler.HandleRemoveBlockingPath)
}

// Register the APIs statistic anlysis and uptime monitoring functions
func RegisterStatisticalAPIs(authRouter *auth.RouterDef) {
	/* Traffic Summary */
	authRouter.HandleFunc("/api/stats/summary", statisticCollector.HandleTodayStatLoad)
	authRouter.HandleFunc("/api/stats/countries", HandleCountryDistrSummary)
	authRouter.HandleFunc("/api/stats/netstat", netstatBuffers.HandleGetNetworkInterfaceStats)
	authRouter.HandleFunc("/api/stats/netstatgraph", netstatBuffers.HandleGetBufferedNetworkInterfaceStats)
	authRouter.HandleFunc("/api/stats/listnic", netstat.HandleListNetworkInterfaces)
	/* Zoraxy Analytic */
	authRouter.HandleFunc("/api/analytic/list", AnalyticLoader.HandleSummaryList)
	authRouter.HandleFunc("/api/analytic/load", AnalyticLoader.HandleLoadTargetDaySummary)
	authRouter.HandleFunc("/api/analytic/loadRange", AnalyticLoader.HandleLoadTargetRangeSummary)
	authRouter.HandleFunc("/api/analytic/exportRange", AnalyticLoader.HandleRangeExport)
	authRouter.HandleFunc("/api/analytic/resetRange", AnalyticLoader.HandleRangeReset)
	/* UpTime Monitor */
	authRouter.HandleFunc("/api/utm/list", HandleUptimeMonitorListing)
}

// Register the APIs for Stream (TCP / UDP) Proxy management functions
func RegisterStreamProxyAPIs(authRouter *auth.RouterDef) {
	authRouter.HandleFunc("/api/streamprox/config/add", streamProxyManager.HandleAddProxyConfig)
	authRouter.HandleFunc("/api/streamprox/config/edit", streamProxyManager.HandleEditProxyConfigs)
	authRouter.HandleFunc("/api/streamprox/config/list", streamProxyManager.HandleListConfigs)
	authRouter.HandleFunc("/api/streamprox/config/start", streamProxyManager.HandleStartProxy)
	authRouter.HandleFunc("/api/streamprox/config/stop", streamProxyManager.HandleStopProxy)
	authRouter.HandleFunc("/api/streamprox/config/delete", streamProxyManager.HandleRemoveProxy)
	authRouter.HandleFunc("/api/streamprox/config/status", streamProxyManager.HandleGetProxyStatus)
}

// Register the APIs for mDNS service management functions
func RegisterMDNSAPIs(authRouter *auth.RouterDef) {
	authRouter.HandleFunc("/api/mdns/list", HandleMdnsListing)
	authRouter.HandleFunc("/api/mdns/discover", HandleMdnsScanning)
}

// Register the APIs for ACME and Auto Renewer management functions
func RegisterACMEAndAutoRenewerAPIs(authRouter *auth.RouterDef) {
	/* ACME Core */
	authRouter.HandleFunc("/api/acme/listExpiredDomains", acmeHandler.HandleGetExpiredDomains)
	authRouter.HandleFunc("/api/acme/obtainCert", AcmeCheckAndHandleRenewCertificate)
	/* Auto Renewer */
	authRouter.HandleFunc("/api/acme/autoRenew/enable", acmeAutoRenewer.HandleAutoRenewEnable)
	authRouter.HandleFunc("/api/acme/autoRenew/ca", HandleACMEPreferredCA)
	authRouter.HandleFunc("/api/acme/autoRenew/email", acmeAutoRenewer.HandleACMEEmail)
	authRouter.HandleFunc("/api/acme/autoRenew/setDomains", acmeAutoRenewer.HandleSetAutoRenewDomains)
	authRouter.HandleFunc("/api/acme/autoRenew/setEAB", acmeAutoRenewer.HanldeSetEAB)
	authRouter.HandleFunc("/api/acme/autoRenew/setDNS", acmeAutoRenewer.HandleSetDNS)
	authRouter.HandleFunc("/api/acme/autoRenew/listDomains", acmeAutoRenewer.HandleLoadAutoRenewDomains)
	authRouter.HandleFunc("/api/acme/autoRenew/renewPolicy", acmeAutoRenewer.HandleRenewPolicy)
	authRouter.HandleFunc("/api/acme/autoRenew/renewNow", acmeAutoRenewer.HandleRenewNow)
	authRouter.HandleFunc("/api/acme/dns/providers", acmedns.HandleServeProvidersJson)
	/* ACME Wizard */
	authRouter.HandleFunc("/api/acme/wizard", acmewizard.HandleGuidedStepCheck)
}

// Register the APIs for Static Web Server management functions
func RegisterStaticWebServerAPIs(authRouter *auth.RouterDef) {
	/* Static Web Server Controls */
	authRouter.HandleFunc("/api/webserv/status", staticWebServer.HandleGetStatus)
	authRouter.HandleFunc("/api/webserv/start", staticWebServer.HandleStartServer)
	authRouter.HandleFunc("/api/webserv/stop", staticWebServer.HandleStopServer)
	authRouter.HandleFunc("/api/webserv/setPort", HandleStaticWebServerPortChange)
	authRouter.HandleFunc("/api/webserv/setDirList", staticWebServer.SetEnableDirectoryListing)
	/* File Manager */
	if *allowWebFileManager {
		authRouter.HandleFunc("/api/fs/list", staticWebServer.FileManager.HandleList)
		authRouter.HandleFunc("/api/fs/upload", staticWebServer.FileManager.HandleUpload)
		authRouter.HandleFunc("/api/fs/download", staticWebServer.FileManager.HandleDownload)
		authRouter.HandleFunc("/api/fs/newFolder", staticWebServer.FileManager.HandleNewFolder)
		authRouter.HandleFunc("/api/fs/copy", staticWebServer.FileManager.HandleFileCopy)
		authRouter.HandleFunc("/api/fs/move", staticWebServer.FileManager.HandleFileMove)
		authRouter.HandleFunc("/api/fs/properties", staticWebServer.FileManager.HandleFileProperties)
		authRouter.HandleFunc("/api/fs/del", staticWebServer.FileManager.HandleFileDelete)
	}
}

// Register the APIs for Network Utilities functions
func RegisterNetworkUtilsAPIs(authRouter *auth.RouterDef) {
	authRouter.HandleFunc("/api/tools/ipscan", ipscan.HandleIpScan)
	authRouter.HandleFunc("/api/tools/portscan", ipscan.HandleScanPort)
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
}

func RegisterPluginAPIs(authRouter *auth.RouterDef) {
	authRouter.HandleFunc("/api/plugins/list", pluginManager.HandleListPlugins)
	authRouter.HandleFunc("/api/plugins/enable", pluginManager.HandleEnablePlugin)
	authRouter.HandleFunc("/api/plugins/disable", pluginManager.HandleDisablePlugin)
	authRouter.HandleFunc("/api/plugins/icon", pluginManager.HandleLoadPluginIcon)
	authRouter.HandleFunc("/api/plugins/info", pluginManager.HandlePluginInfo)

	authRouter.HandleFunc("/api/plugins/groups/list", pluginManager.HandleListPluginGroups)
	authRouter.HandleFunc("/api/plugins/groups/add", pluginManager.HandleAddPluginToGroup)
	authRouter.HandleFunc("/api/plugins/groups/remove", pluginManager.HandleRemovePluginFromGroup)
	authRouter.HandleFunc("/api/plugins/groups/deleteTag", pluginManager.HandleRemovePluginGroup)

	authRouter.HandleFunc("/api/plugins/store/list", pluginManager.HandleListDownloadablePlugins)
	authRouter.HandleFunc("/api/plugins/store/resync", pluginManager.HandleResyncPluginList)
	authRouter.HandleFunc("/api/plugins/store/install", pluginManager.HandleInstallPlugin)
	authRouter.HandleFunc("/api/plugins/store/uninstall", pluginManager.HandleUninstallPlugin)
}

// Register the APIs for Auth functions, due to scoping issue some functions are defined here
func RegisterAuthAPIs(requireAuth bool, targetMux *http.ServeMux) {
	targetMux.HandleFunc("/api/auth/login", authAgent.HandleLogin)
	targetMux.HandleFunc("/api/auth/logout", authAgent.HandleLogout)
	targetMux.HandleFunc("/api/auth/checkLogin", func(w http.ResponseWriter, r *http.Request) {
		if requireAuth {
			authAgent.CheckLogin(w, r)
		} else {
			utils.SendJSONResponse(w, "true")
		}
	})
	targetMux.HandleFunc("/api/auth/username", func(w http.ResponseWriter, r *http.Request) {
		username, err := authAgent.GetUserName(w, r)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}
		js, _ := json.Marshal(username)
		utils.SendJSONResponse(w, string(js))
	})
	targetMux.HandleFunc("/api/auth/userCount", func(w http.ResponseWriter, r *http.Request) {
		js, _ := json.Marshal(authAgent.GetUserCounts())
		utils.SendJSONResponse(w, string(js))
	})
	targetMux.HandleFunc("/api/auth/register", func(w http.ResponseWriter, r *http.Request) {
		if authAgent.GetUserCounts() == 0 {
			//Allow register root admin
			authAgent.HandleRegisterWithoutEmail(w, r, func(username, reserved string) {})
		} else {
			//This function is disabled
			utils.SendErrorResponse(w, "Root management account already exists")
		}
	})
	targetMux.HandleFunc("/api/auth/changePassword", func(w http.ResponseWriter, r *http.Request) {
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

/* Register all the APIs */
func initAPIs(targetMux *http.ServeMux) {
	authRouter := auth.NewManagedHTTPRouter(auth.RouterOption{
		AuthAgent:   authAgent,
		RequireAuth: requireAuth,
		TargetMux:   targetMux,
		DeniedHandler: func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "401 - Unauthorized", http.StatusUnauthorized)
		},
	})

	// Register the standard web services URLs
	var staticWebRes http.Handler
	if *development_build {
		staticWebRes = http.FileServer(http.Dir("web/"))
	} else {
		subFS, err := fs.Sub(webres, "web")
		if err != nil {
			panic("Failed to strip 'web/' from embedded resources: " + err.Error())
		}
		staticWebRes = http.FileServer(http.FS(subFS))
	}

	//Add a layer of middleware for advance control
	advHandler := FSHandler(staticWebRes)
	targetMux.Handle("/", advHandler)

	//Register the APIs
	RegisterAuthAPIs(requireAuth, targetMux)
	RegisterHTTPProxyAPIs(authRouter)
	RegisterTLSAPIs(authRouter)
	RegisterAuthenticationHandlerAPIs(authRouter)
	RegisterRedirectionAPIs(authRouter)
	RegisterAccessRuleAPIs(authRouter)
	RegisterPathRuleAPIs(authRouter)
	RegisterStatisticalAPIs(authRouter)
	RegisterStreamProxyAPIs(authRouter)
	RegisterMDNSAPIs(authRouter)
	RegisterNetworkUtilsAPIs(authRouter)
	RegisterACMEAndAutoRenewerAPIs(authRouter)
	RegisterStaticWebServerAPIs(authRouter)
	RegisterPluginAPIs(authRouter)

	//Account Reset
	targetMux.HandleFunc("/api/account/reset", HandleAdminAccountResetEmail)
	targetMux.HandleFunc("/api/account/new", HandleNewPasswordSetup)

	//Docker UX Optimizations
	authRouter.HandleFunc("/api/docker/available", DockerUXOptimizer.HandleDockerAvailable)
	authRouter.HandleFunc("/api/docker/containers", DockerUXOptimizer.HandleDockerContainersList)

	//Others
	targetMux.HandleFunc("/api/info/x", HandleZoraxyInfo)
	authRouter.HandleFunc("/api/info/geoip", HandleGeoIpLookup)
	authRouter.HandleFunc("/api/conf/export", ExportConfigAsZip)
	authRouter.HandleFunc("/api/conf/import", ImportConfigFromZip)
	authRouter.HandleFunc("/api/log/list", LogViewer.HandleListLog)
	authRouter.HandleFunc("/api/log/read", LogViewer.HandleReadLog)

	//Debug
	authRouter.HandleFunc("/api/info/pprof", pprof.Index)
}
