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
	authRouter.HandleFunc("/api/proxy/enable", ReverseProxyHandleOnOff, false)
	authRouter.HandleFunc("/api/proxy/add", ReverseProxyHandleAddEndpoint, false)
	authRouter.HandleFunc("/api/proxy/status", ReverseProxyStatus, true)
	authRouter.HandleFunc("/api/proxy/toggle", ReverseProxyToggleRuleSet, false)
	authRouter.HandleFunc("/api/proxy/list", ReverseProxyList, true)
	authRouter.HandleFunc("/api/proxy/listTags", ReverseProxyListTags, true)
	authRouter.HandleFunc("/api/proxy/detail", ReverseProxyListDetail, true)
	authRouter.HandleFunc("/api/proxy/edit", ReverseProxyHandleEditEndpoint, false)
	authRouter.HandleFunc("/api/proxy/setAlias", ReverseProxyHandleAlias, false)
	authRouter.HandleFunc("/api/proxy/setTlsConfig", ReverseProxyHandleSetTlsConfig, false)
	authRouter.HandleFunc("/api/proxy/setHostname", ReverseProxyHandleSetHostname, false)
	authRouter.HandleFunc("/api/proxy/del", DeleteProxyEndpoint, false)
	authRouter.HandleFunc("/api/proxy/updateCredentials", UpdateProxyBasicAuthCredentials, false)
	authRouter.HandleFunc("/api/proxy/tlscheck", domainsniff.HandleCheckSiteSupportTLS, false)
	authRouter.HandleFunc("/api/proxy/setIncoming", HandleIncomingPortSet, false)
	authRouter.HandleFunc("/api/proxy/useHttpsRedirect", HandleUpdateHttpsRedirect, false)
	authRouter.HandleFunc("/api/proxy/listenPort80", HandleUpdatePort80Listener, false)
	authRouter.HandleFunc("/api/proxy/requestIsProxied", HandleManagementProxyCheck, false)
	authRouter.HandleFunc("/api/proxy/developmentMode", HandleDevelopmentModeChange, false)
	/* Reverse proxy upstream (load balance) */
	authRouter.HandleFunc("/api/proxy/upstream/list", ReverseProxyUpstreamList, false)
	authRouter.HandleFunc("/api/proxy/upstream/add", ReverseProxyUpstreamAdd, false)
	authRouter.HandleFunc("/api/proxy/upstream/setPriority", ReverseProxyUpstreamSetPriority, false)
	authRouter.HandleFunc("/api/proxy/upstream/update", ReverseProxyUpstreamUpdate, false)
	authRouter.HandleFunc("/api/proxy/upstream/remove", ReverseProxyUpstreamDelete, false)
	/* Reverse proxy virtual directory */
	authRouter.HandleFunc("/api/proxy/vdir/list", ReverseProxyListVdir, false)
	authRouter.HandleFunc("/api/proxy/vdir/add", ReverseProxyAddVdir, false)
	authRouter.HandleFunc("/api/proxy/vdir/del", ReverseProxyDeleteVdir, false)
	authRouter.HandleFunc("/api/proxy/vdir/edit", ReverseProxyEditVdir, false)
	/* Reverse proxy user-defined header */
	authRouter.HandleFunc("/api/proxy/header/list", HandleCustomHeaderList, true)
	authRouter.HandleFunc("/api/proxy/header/add", HandleCustomHeaderAdd, false)
	authRouter.HandleFunc("/api/proxy/header/remove", HandleCustomHeaderRemove, false)
	authRouter.HandleFunc("/api/proxy/header/handleHSTS", HandleHSTSState, false)
	authRouter.HandleFunc("/api/proxy/header/handleHopByHop", HandleHopByHop, false)
	authRouter.HandleFunc("/api/proxy/header/handleHostOverwrite", HandleHostOverwrite, false)
	authRouter.HandleFunc("/api/proxy/header/handlePermissionPolicy", HandlePermissionPolicy, false)
	authRouter.HandleFunc("/api/proxy/header/handleWsHeaderBehavior", HandleWsHeaderBehavior, false)
	/* Reverse proxy auth related */
	authRouter.HandleFunc("/api/proxy/auth/exceptions/list", ListProxyBasicAuthExceptionPaths, true)
	authRouter.HandleFunc("/api/proxy/auth/exceptions/add", AddProxyBasicAuthExceptionPaths, false)
	authRouter.HandleFunc("/api/proxy/auth/exceptions/delete", RemoveProxyBasicAuthExceptionPaths, false)
}

// Register the APIs for TLS / SSL certificate management functions
func RegisterTLSAPIs(authRouter *auth.RouterDef) {
	//Global certificate settings
	authRouter.HandleFunc("/api/cert/tls", handleToggleTLSProxy, false)
	authRouter.HandleFunc("/api/cert/tlsRequireLatest", handleSetTlsRequireLatest, false)
	authRouter.HandleFunc("/api/cert/resolve", handleCertTryResolve, false)
	authRouter.HandleFunc("/api/cert/setPreferredCertificate", handleSetDomainPreferredCertificate, false)

	//Certificate store functions
	authRouter.HandleFunc("/api/cert/upload", tlsCertManager.HandleCertUpload, false)
	authRouter.HandleFunc("/api/cert/download", tlsCertManager.HandleCertDownload, false)
	authRouter.HandleFunc("/api/cert/list", tlsCertManager.HandleListCertificate, false)
	authRouter.HandleFunc("/api/cert/listdomains", tlsCertManager.HandleListDomains, false)
	authRouter.HandleFunc("/api/cert/checkDefault", tlsCertManager.HandleDefaultCertCheck, false)
	authRouter.HandleFunc("/api/cert/delete", tlsCertManager.HandleCertRemove, false)
	authRouter.HandleFunc("/api/cert/selfsign", tlsCertManager.HandleSelfSignCertGenerate, false)
}

// Register the APIs for Authentication handlers like Forward Auth and OAUTH2
func RegisterAuthenticationHandlerAPIs(authRouter *auth.RouterDef) {
	authRouter.HandleFunc("/api/sso/forward-auth", forwardAuthRouter.HandleAPIOptions, false)
	authRouter.HandleFunc("/api/sso/OAuth2", oauth2Router.HandleSetOAuth2Settings, false)
}

// Register the APIs for redirection rules management functions
func RegisterRedirectionAPIs(authRouter *auth.RouterDef) {
	authRouter.HandleFunc("/api/redirect/list", handleListRedirectionRules, true)
	authRouter.HandleFunc("/api/redirect/add", handleAddRedirectionRule, false)
	authRouter.HandleFunc("/api/redirect/delete", handleDeleteRedirectionRule, false)
	authRouter.HandleFunc("/api/redirect/edit", handleEditRedirectionRule, false)
	authRouter.HandleFunc("/api/redirect/regex", handleToggleRedirectRegexpSupport, false)
}

// Register the APIs for access rules management functions
func RegisterAccessRuleAPIs(authRouter *auth.RouterDef) {
	/* Access Rules Settings & Status */
	authRouter.HandleFunc("/api/access/list", handleListAccessRules, true)
	authRouter.HandleFunc("/api/access/attach", handleAttachRuleToHost, false)
	authRouter.HandleFunc("/api/access/create", handleCreateAccessRule, false)
	authRouter.HandleFunc("/api/access/remove", handleRemoveAccessRule, false)
	authRouter.HandleFunc("/api/access/update", handleUpadateAccessRule, false)
	/* Blacklist */
	authRouter.HandleFunc("/api/blacklist/list", handleListBlacklisted, true)
	authRouter.HandleFunc("/api/blacklist/country/add", handleCountryBlacklistAdd, false)
	authRouter.HandleFunc("/api/blacklist/country/remove", handleCountryBlacklistRemove, false)
	authRouter.HandleFunc("/api/blacklist/ip/add", handleIpBlacklistAdd, true)
	authRouter.HandleFunc("/api/blacklist/ip/remove", handleIpBlacklistRemove, true)
	authRouter.HandleFunc("/api/blacklist/enable", handleBlacklistEnable, true)
	/* Whitelist */
	authRouter.HandleFunc("/api/whitelist/list", handleListWhitelisted, true)
	authRouter.HandleFunc("/api/whitelist/country/add", handleCountryWhitelistAdd, false)
	authRouter.HandleFunc("/api/whitelist/country/remove", handleCountryWhitelistRemove, false)
	authRouter.HandleFunc("/api/whitelist/ip/add", handleIpWhitelistAdd, false)
	authRouter.HandleFunc("/api/whitelist/ip/remove", handleIpWhitelistRemove, false)
	authRouter.HandleFunc("/api/whitelist/enable", handleWhitelistEnable, false)
	authRouter.HandleFunc("/api/whitelist/allowLocal", handleWhitelistAllowLoopback, false)
	/* Quick Ban List */
	authRouter.HandleFunc("/api/quickban/list", handleListQuickBan, false)
}

// Register the APIs for path blocking rules management functions, WIP
func RegisterPathRuleAPIs(authRouter *auth.RouterDef) {
	authRouter.HandleFunc("/api/pathrule/add", pathRuleHandler.HandleAddBlockingPath, false)
	authRouter.HandleFunc("/api/pathrule/list", pathRuleHandler.HandleListBlockingPath, false)
	authRouter.HandleFunc("/api/pathrule/remove", pathRuleHandler.HandleRemoveBlockingPath, false)
}

// Register the APIs statistic anlysis and uptime monitoring functions
func RegisterStatisticalAPIs(authRouter *auth.RouterDef) {
	/* Traffic Summary */
	authRouter.HandleFunc("/api/stats/summary", statisticCollector.HandleTodayStatLoad, false)
	authRouter.HandleFunc("/api/stats/countries", HandleCountryDistrSummary, false)
	authRouter.HandleFunc("/api/stats/netstat", netstatBuffers.HandleGetNetworkInterfaceStats, false)
	authRouter.HandleFunc("/api/stats/netstatgraph", netstatBuffers.HandleGetBufferedNetworkInterfaceStats, false)
	authRouter.HandleFunc("/api/stats/listnic", netstat.HandleListNetworkInterfaces, false)
	/* Zoraxy Analytic */
	authRouter.HandleFunc("/api/analytic/list", AnalyticLoader.HandleSummaryList, false)
	authRouter.HandleFunc("/api/analytic/load", AnalyticLoader.HandleLoadTargetDaySummary, false)
	authRouter.HandleFunc("/api/analytic/loadRange", AnalyticLoader.HandleLoadTargetRangeSummary, false)
	authRouter.HandleFunc("/api/analytic/exportRange", AnalyticLoader.HandleRangeExport, false)
	authRouter.HandleFunc("/api/analytic/resetRange", AnalyticLoader.HandleRangeReset, false)
	/* UpTime Monitor */
	authRouter.HandleFunc("/api/utm/list", HandleUptimeMonitorListing, false)
}

// Register the APIs for Stream (TCP / UDP) Proxy management functions
func RegisterStreamProxyAPIs(authRouter *auth.RouterDef) {
	authRouter.HandleFunc("/api/streamprox/config/add", streamProxyManager.HandleAddProxyConfig, false)
	authRouter.HandleFunc("/api/streamprox/config/edit", streamProxyManager.HandleEditProxyConfigs, false)
	authRouter.HandleFunc("/api/streamprox/config/list", streamProxyManager.HandleListConfigs, false)
	authRouter.HandleFunc("/api/streamprox/config/start", streamProxyManager.HandleStartProxy, false)
	authRouter.HandleFunc("/api/streamprox/config/stop", streamProxyManager.HandleStopProxy, false)
	authRouter.HandleFunc("/api/streamprox/config/delete", streamProxyManager.HandleRemoveProxy, false)
	authRouter.HandleFunc("/api/streamprox/config/status", streamProxyManager.HandleGetProxyStatus, false)
}

// Register the APIs for mDNS service management functions
func RegisterMDNSAPIs(authRouter *auth.RouterDef) {
	authRouter.HandleFunc("/api/mdns/list", HandleMdnsListing, false)
	authRouter.HandleFunc("/api/mdns/discover", HandleMdnsScanning, false)
}

// Register the APIs for ACME and Auto Renewer management functions
func RegisterACMEAndAutoRenewerAPIs(authRouter *auth.RouterDef) {
	/* ACME Core */
	authRouter.HandleFunc("/api/acme/listExpiredDomains", acmeHandler.HandleGetExpiredDomains, false)
	authRouter.HandleFunc("/api/acme/obtainCert", AcmeCheckAndHandleRenewCertificate, false)
	/* Auto Renewer */
	authRouter.HandleFunc("/api/acme/autoRenew/enable", acmeAutoRenewer.HandleAutoRenewEnable, false)
	authRouter.HandleFunc("/api/acme/autoRenew/ca", HandleACMEPreferredCA, false)
	authRouter.HandleFunc("/api/acme/autoRenew/email", acmeAutoRenewer.HandleACMEEmail, false)
	authRouter.HandleFunc("/api/acme/autoRenew/setDomains", acmeAutoRenewer.HandleSetAutoRenewDomains, false)
	authRouter.HandleFunc("/api/acme/autoRenew/setEAB", acmeAutoRenewer.HanldeSetEAB, false)
	authRouter.HandleFunc("/api/acme/autoRenew/setDNS", acmeAutoRenewer.HandleSetDNS, false)
	authRouter.HandleFunc("/api/acme/autoRenew/listDomains", acmeAutoRenewer.HandleLoadAutoRenewDomains, false)
	authRouter.HandleFunc("/api/acme/autoRenew/renewPolicy", acmeAutoRenewer.HandleRenewPolicy, false)
	authRouter.HandleFunc("/api/acme/autoRenew/renewNow", acmeAutoRenewer.HandleRenewNow, false)
	authRouter.HandleFunc("/api/acme/dns/providers", acmedns.HandleServeProvidersJson, false)
	/* ACME Wizard */
	authRouter.HandleFunc("/api/acme/wizard", acmewizard.HandleGuidedStepCheck, false)
}

// Register the APIs for Static Web Server management functions
func RegisterStaticWebServerAPIs(authRouter *auth.RouterDef) {
	/* Static Web Server Controls */
	authRouter.HandleFunc("/api/webserv/status", staticWebServer.HandleGetStatus, false)
	authRouter.HandleFunc("/api/webserv/start", staticWebServer.HandleStartServer, false)
	authRouter.HandleFunc("/api/webserv/stop", staticWebServer.HandleStopServer, false)
	authRouter.HandleFunc("/api/webserv/setPort", HandleStaticWebServerPortChange, false)
	authRouter.HandleFunc("/api/webserv/setDirList", staticWebServer.SetEnableDirectoryListing, false)
	authRouter.HandleFunc("/api/webserv/disableListenAllInterface", staticWebServer.SetDisableListenToAllInterface, false)
	/* File Manager */
	if *allowWebFileManager {
		authRouter.HandleFunc("/api/fs/list", staticWebServer.FileManager.HandleList, false)
		authRouter.HandleFunc("/api/fs/upload", staticWebServer.FileManager.HandleUpload, false)
		authRouter.HandleFunc("/api/fs/download", staticWebServer.FileManager.HandleDownload, false)
		authRouter.HandleFunc("/api/fs/newFolder", staticWebServer.FileManager.HandleNewFolder, false)
		authRouter.HandleFunc("/api/fs/copy", staticWebServer.FileManager.HandleFileCopy, false)
		authRouter.HandleFunc("/api/fs/move", staticWebServer.FileManager.HandleFileMove, false)
		authRouter.HandleFunc("/api/fs/properties", staticWebServer.FileManager.HandleFileProperties, false)
		authRouter.HandleFunc("/api/fs/del", staticWebServer.FileManager.HandleFileDelete, false)
	}
}

// Register the APIs for Network Utilities functions
func RegisterNetworkUtilsAPIs(authRouter *auth.RouterDef) {
	authRouter.HandleFunc("/api/tools/ipscan", ipscan.HandleIpScan, false)
	authRouter.HandleFunc("/api/tools/portscan", ipscan.HandleScanPort, false)
	authRouter.HandleFunc("/api/tools/traceroute", netutils.HandleTraceRoute, false)
	authRouter.HandleFunc("/api/tools/ping", netutils.HandlePing, false)
	authRouter.HandleFunc("/api/tools/whois", netutils.HandleWhois, false)
	authRouter.HandleFunc("/api/tools/webssh", HandleCreateProxySession, false)
	authRouter.HandleFunc("/api/tools/websshSupported", HandleWebSshSupportCheck, false)
	authRouter.HandleFunc("/api/tools/wol", HandleWakeOnLan, false)
	authRouter.HandleFunc("/api/tools/smtp/get", HandleSMTPGet, false)
	authRouter.HandleFunc("/api/tools/smtp/set", HandleSMTPSet, false)
	authRouter.HandleFunc("/api/tools/smtp/admin", HandleAdminEmailGet, false)
	authRouter.HandleFunc("/api/tools/smtp/test", HandleTestEmailSend, false)
	authRouter.HandleFunc("/api/tools/fwdproxy/enable", forwardProxy.HandleToogle, false)
	authRouter.HandleFunc("/api/tools/fwdproxy/port", forwardProxy.HandlePort, false)
}

func RegisterPluginAPIs(authRouter *auth.RouterDef) {
	authRouter.HandleFunc("/api/plugins/list", pluginManager.HandleListPlugins, false)
	authRouter.HandleFunc("/api/plugins/enable", pluginManager.HandleEnablePlugin, false)
	authRouter.HandleFunc("/api/plugins/disable", pluginManager.HandleDisablePlugin, false)
	authRouter.HandleFunc("/api/plugins/icon", pluginManager.HandleLoadPluginIcon, false)
	authRouter.HandleFunc("/api/plugins/info", pluginManager.HandlePluginInfo, false)

	authRouter.HandleFunc("/api/plugins/groups/list", pluginManager.HandleListPluginGroups, false)
	authRouter.HandleFunc("/api/plugins/groups/add", pluginManager.HandleAddPluginToGroup, false)
	authRouter.HandleFunc("/api/plugins/groups/remove", pluginManager.HandleRemovePluginFromGroup, false)
	authRouter.HandleFunc("/api/plugins/groups/deleteTag", pluginManager.HandleRemovePluginGroup, false)

	authRouter.HandleFunc("/api/plugins/store/list", pluginManager.HandleListDownloadablePlugins, false)
	authRouter.HandleFunc("/api/plugins/store/resync", pluginManager.HandleResyncPluginList, false)
	authRouter.HandleFunc("/api/plugins/store/install", pluginManager.HandleInstallPlugin, false)
	authRouter.HandleFunc("/api/plugins/store/uninstall", pluginManager.HandleUninstallPlugin, false)

	// Developer options
	authRouter.HandleFunc("/api/plugins/developer/enableAutoReload", pluginManager.HandleEnableHotReload, false)
	authRouter.HandleFunc("/api/plugins/developer/setAutoReloadInterval", pluginManager.HandleSetHotReloadInterval, false)
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
	authRouter.HandleFunc("/api/docker/available", DockerUXOptimizer.HandleDockerAvailable, false)
	authRouter.HandleFunc("/api/docker/containers", DockerUXOptimizer.HandleDockerContainersList, false)

	//Others
	targetMux.HandleFunc("/api/info/x", HandleZoraxyInfo)
	authRouter.HandleFunc("/api/info/geoip", HandleGeoIpLookup, false)
	authRouter.HandleFunc("/api/conf/export", ExportConfigAsZip, false)
	authRouter.HandleFunc("/api/conf/import", ImportConfigFromZip, false)
	authRouter.HandleFunc("/api/log/list", LogViewer.HandleListLog, false)
	authRouter.HandleFunc("/api/log/read", LogViewer.HandleReadLog, false)

	//Debug
	authRouter.HandleFunc("/api/info/pprof", pprof.Index, false)
}
