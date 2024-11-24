package main

import (
	"log"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"imuslab.com/zoraxy/mod/access"
	"imuslab.com/zoraxy/mod/acme"
	"imuslab.com/zoraxy/mod/auth"
	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/dockerux"
	"imuslab.com/zoraxy/mod/dynamicproxy/loadbalance"
	"imuslab.com/zoraxy/mod/dynamicproxy/redirection"
	"imuslab.com/zoraxy/mod/forwardproxy"
	"imuslab.com/zoraxy/mod/ganserv"
	"imuslab.com/zoraxy/mod/geodb"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/info/logviewer"
	"imuslab.com/zoraxy/mod/mdns"
	"imuslab.com/zoraxy/mod/netstat"
	"imuslab.com/zoraxy/mod/pathrule"
	"imuslab.com/zoraxy/mod/sshprox"
	"imuslab.com/zoraxy/mod/statistic"
	"imuslab.com/zoraxy/mod/statistic/analytic"
	"imuslab.com/zoraxy/mod/streamproxy"
	"imuslab.com/zoraxy/mod/tlscert"
	"imuslab.com/zoraxy/mod/webserv"
)

/*
	Startup Sequence

	This function starts the startup sequence of all
	required modules. Their startup sequences are inter-dependent
	and must be started in a specific order.

	Don't touch this function unless you know what you are doing
*/

var (
	/*
		MDNS related
	*/
	previousmdnsScanResults = []*mdns.NetworkHost{}
	mdnsTickerStop          chan bool
)

func startupSequence() {
	//Start a system wide logger and log viewer
	l, err := logger.NewLogger(LOG_PREFIX, LOG_FOLDER)
	if err == nil {
		SystemWideLogger = l
	} else {
		panic(err)
	}
	LogViewer = logviewer.NewLogViewer(&logviewer.ViewerOption{
		RootFolder: LOG_FOLDER,
		Extension:  LOG_EXTENSION,
	})

	//Create database
	db, err := database.NewDatabase(DATABASE_PATH, false)
	if err != nil {
		log.Fatal(err)
	}
	sysdb = db
	//Create tables for the database
	sysdb.NewTable("settings")

	//Create tmp folder and conf folder
	os.MkdirAll(TMP_FOLDER, 0775)
	os.MkdirAll(CONF_HTTP_PROXY, 0775)

	//Create an auth agent
	sessionKey, err := auth.GetSessionKey(sysdb, SystemWideLogger)
	if err != nil {
		log.Fatal(err)
	}
	authAgent = auth.NewAuthenticationAgent(SYSTEM_NAME, []byte(sessionKey), sysdb, true, SystemWideLogger, func(w http.ResponseWriter, r *http.Request) {
		//Not logged in. Redirecting to login page
		http.Redirect(w, r, ppf("/login.html"), http.StatusTemporaryRedirect)
	})

	//Create a TLS certificate manager
	tlsCertManager, err = tlscert.NewManager(CONF_CERT_STORE, DEVELOPMENT_BUILD, SystemWideLogger)
	if err != nil {
		panic(err)
	}

	//Create a redirection rule table
	db.NewTable("redirect")
	redirectAllowRegexp := false
	db.Read("redirect", "regex", &redirectAllowRegexp)
	redirectTable, err = redirection.NewRuleTable(CONF_REDIRECTION, redirectAllowRegexp, SystemWideLogger)
	if err != nil {
		panic(err)
	}

	//Create a geodb store
	geodbStore, err = geodb.NewGeoDb(sysdb, &geodb.StoreOptions{
		AllowSlowIpv4LookUp:          !*enableHighSpeedGeoIPLookup,
		AllowSlowIpv6Lookup:          !*enableHighSpeedGeoIPLookup,
		SlowLookupCacheClearInterval: GEODB_CACHE_CLEAR_INTERVAL * time.Minute,
	})
	if err != nil {
		panic(err)
	}

	//Create a load balancer
	loadBalancer = loadbalance.NewLoadBalancer(&loadbalance.Options{
		SystemUUID: nodeUUID,
		Geodb:      geodbStore,
		Logger:     SystemWideLogger,
	})

	//Create the access controller
	accessController, err = access.NewAccessController(&access.Options{
		Database:     sysdb,
		GeoDB:        geodbStore,
		ConfigFolder: CONF_ACCESS_RULE,
	})
	if err != nil {
		panic(err)
	}

	/*
		//Create an SSO handler
		ssoHandler, err = sso.NewSSOHandler(&sso.SSOConfig{
			SystemUUID:       nodeUUID,
			PortalServerPort: 5488,
			AuthURL:          "http://auth.localhost",
			Database:         sysdb,
			Logger:           SystemWideLogger,
		})
		if err != nil {
			log.Fatal(err)
		}
		//Restore the SSO handler to previous state before shutdown
		ssoHandler.RestorePreviousRunningState()
	*/

	//Create a statistic collector
	statisticCollector, err = statistic.NewStatisticCollector(statistic.CollectorOption{
		Database: sysdb,
	})
	if err != nil {
		panic(err)
	}

	//Start the static web server
	staticWebServer = webserv.NewWebServer(&webserv.WebServerOptions{
		Sysdb:                  sysdb,
		Port:                   strconv.Itoa(WEBSERV_DEFAULT_PORT), //Default Port
		WebRoot:                *staticWebServerRoot,
		EnableDirectoryListing: true,
		EnableWebDirManager:    *allowWebFileManager,
		Logger:                 SystemWideLogger,
	})
	//Restore the web server to previous shutdown state
	staticWebServer.RestorePreviousState()

	//Create a netstat buffer
	netstatBuffers, err = netstat.NewNetStatBuffer(300, SystemWideLogger)
	if err != nil {
		SystemWideLogger.PrintAndLog("Network", "Failed to load network statistic info", err)
		panic(err)
	}

	/*
		Path Rules

		This section of starutp script start the path rules where
		user can define their own routing logics
	*/

	pathRuleHandler = pathrule.NewPathRuleHandler(&pathrule.Options{
		Enabled:      false,
		ConfigFolder: CONF_PATH_RULE,
	})

	/*
		MDNS Discovery Service

		This discover nearby ArozOS Nodes or other services
		that provide mDNS discovery with domain (e.g. Synology NAS)
	*/

	if *allowMdnsScanning {
		portInt, err := strconv.Atoi(strings.Split(*webUIPort, ":")[1])
		if err != nil {
			portInt = 8000
		}

		hostName := *mdnsName
		if hostName == "" {
			hostName = MDNS_HOSTNAME_PREFIX + nodeUUID
		} else {
			//Trim off the suffix
			hostName = strings.TrimSuffix(hostName, ".local")
		}

		mdnsScanner, err = mdns.NewMDNS(mdns.NetworkHost{
			HostName:     hostName,
			Port:         portInt,
			Domain:       MDNS_IDENTIFY_DOMAIN,
			Model:        MDNS_IDENTIFY_DEVICE_TYPE,
			UUID:         nodeUUID,
			Vendor:       MDNS_IDENTIFY_VENDOR,
			BuildVersion: SYSTEM_VERSION,
		}, "")
		if err != nil {
			SystemWideLogger.Println("Unable to startup mDNS service. Disabling mDNS services")
		} else {
			//Start initial scanning
			go func() {
				hosts := mdnsScanner.Scan(MDNS_SCAN_TIMEOUT, "")
				previousmdnsScanResults = hosts
				SystemWideLogger.Println("mDNS Startup scan completed")
			}()

			//Create a ticker to update mDNS results every 5 minutes
			ticker := time.NewTicker(MDNS_SCAN_UPDATE_INTERVAL * time.Minute)
			stopChan := make(chan bool)
			go func() {
				for {
					select {
					case <-stopChan:
						ticker.Stop()
					case <-ticker.C:
						hosts := mdnsScanner.Scan(MDNS_SCAN_TIMEOUT, "")
						previousmdnsScanResults = hosts
						SystemWideLogger.Println("mDNS scan result updated")
					}
				}
			}()
			mdnsTickerStop = stopChan
		}
	}

	/*
		Global Area Network

		Require zerotier token to work
	*/
	usingZtAuthToken := *ztAuthToken
	if usingZtAuthToken == "" {
		usingZtAuthToken, err = ganserv.TryLoadorAskUserForAuthkey()
		if err != nil {
			SystemWideLogger.Println("Failed to load ZeroTier controller API authtoken")
		}
	}
	ganManager = ganserv.NewNetworkManager(&ganserv.NetworkManagerOptions{
		AuthToken: usingZtAuthToken,
		ApiPort:   *ztAPIPort,
		Database:  sysdb,
	})

	//Create WebSSH Manager
	webSshManager = sshprox.NewSSHProxyManager()

	//Create TCP Proxy Manager
	streamProxyManager, err = streamproxy.NewStreamProxy(&streamproxy.Options{
		AccessControlHandler: accessController.DefaultAccessRule.AllowConnectionAccess,
		ConfigStore:          CONF_STREAM_PROXY,
		Logger:               SystemWideLogger,
	})
	if err != nil {
		panic(err)
	}

	//Create WoL MAC storage table
	sysdb.NewTable("wolmac")

	//Create an email sender if SMTP config exists
	sysdb.NewTable("smtp")
	EmailSender = loadSMTPConfig()

	//Create an analytic loader
	AnalyticLoader = analytic.NewDataLoader(sysdb, statisticCollector)

	//Create basic forward proxy
	sysdb.NewTable("fwdproxy")
	fwdProxyEnabled := false
	fwdProxyPort := 5587
	sysdb.Read("fwdproxy", "port", &fwdProxyPort)
	sysdb.Read("fwdproxy", "enabled", &fwdProxyEnabled)
	forwardProxy = forwardproxy.NewForwardProxy(sysdb, fwdProxyPort, SystemWideLogger)
	if fwdProxyEnabled {
		SystemWideLogger.PrintAndLog("Forward Proxy", "HTTP Forward Proxy Listening on :"+strconv.Itoa(forwardProxy.Port), nil)
		forwardProxy.Start()
	}

	/*
		ACME API

		Obtaining certificates from ACME Server
	*/
	//Create a table just to store acme related preferences
	sysdb.NewTable("acmepref")
	acmeHandler = initACME()
	acmeAutoRenewer, err = acme.NewAutoRenewer(
		ACME_AUTORENEW_CONFIG_PATH,
		CONF_CERT_STORE,
		int64(*acmeAutoRenewInterval),
		*acmeCertAutoRenewDays,
		acmeHandler,
		SystemWideLogger,
	)
	if err != nil {
		log.Fatal(err)
	}

	/* Docker UX Optimizer */
	if runtime.GOOS == "windows" && *runningInDocker {
		SystemWideLogger.PrintAndLog("warning", "Invalid start flag combination: docker=true && runtime.GOOS == windows. Running in docker UX development mode.", nil)
	}
	DockerUXOptimizer = dockerux.NewDockerOptimizer(*runningInDocker, SystemWideLogger)

}

// This sequence start after everything is initialized
func finalSequence() {
	//Start ACME renew agent
	acmeRegisterSpecialRoutingRule()

	//Inject routing rules
	registerBuildInRoutingRules()
}
