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
	required modules
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
	l, err := logger.NewLogger("zr", "./log")
	if err == nil {
		SystemWideLogger = l
	} else {
		panic(err)
	}
	LogViewer = logviewer.NewLogViewer(&logviewer.ViewerOption{
		RootFolder: "./log",
		Extension:  ".log",
	})

	//Create database
	db, err := database.NewDatabase("sys.db", false)
	if err != nil {
		log.Fatal(err)
	}
	sysdb = db
	//Create tables for the database
	sysdb.NewTable("settings")

	//Create tmp folder and conf folder
	os.MkdirAll("./tmp", 0775)
	os.MkdirAll("./conf/proxy/", 0775)

	//Create an auth agent
	sessionKey, err := auth.GetSessionKey(sysdb, SystemWideLogger)
	if err != nil {
		log.Fatal(err)
	}
	authAgent = auth.NewAuthenticationAgent(name, []byte(sessionKey), sysdb, true, SystemWideLogger, func(w http.ResponseWriter, r *http.Request) {
		//Not logged in. Redirecting to login page
		http.Redirect(w, r, ppf("/login.html"), http.StatusTemporaryRedirect)
	})

	//Create a TLS certificate manager
	tlsCertManager, err = tlscert.NewManager("./conf/certs", development)
	if err != nil {
		panic(err)
	}

	//Create a redirection rule table
	db.NewTable("redirect")
	redirectAllowRegexp := false
	db.Read("redirect", "regex", &redirectAllowRegexp)
	redirectTable, err = redirection.NewRuleTable("./conf/redirect", redirectAllowRegexp, SystemWideLogger)
	if err != nil {
		panic(err)
	}

	//Create a geodb store
	geodbStore, err = geodb.NewGeoDb(sysdb, &geodb.StoreOptions{
		AllowSlowIpv4LookUp: !*enableHighSpeedGeoIPLookup,
		AllowSloeIpv6Lookup: !*enableHighSpeedGeoIPLookup,
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
		ConfigFolder: "./conf/access",
	})
	if err != nil {
		panic(err)
	}

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
		Port:                   "5487", //Default Port
		WebRoot:                *staticWebServerRoot,
		EnableDirectoryListing: true,
		EnableWebDirManager:    *allowWebFileManager,
		Logger:                 SystemWideLogger,
	})
	//Restore the web server to previous shutdown state
	staticWebServer.RestorePreviousState()

	//Create a netstat buffer
	netstatBuffers, err = netstat.NewNetStatBuffer(300)
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
		ConfigFolder: "./conf/rules/pathrules",
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
			hostName = "zoraxy_" + nodeUUID
		} else {
			//Trim off the suffix
			hostName = strings.TrimSuffix(hostName, ".local")
		}

		mdnsScanner, err = mdns.NewMDNS(mdns.NetworkHost{
			HostName:     hostName,
			Port:         portInt,
			Domain:       "zoraxy.arozos.com",
			Model:        "Network Gateway",
			UUID:         nodeUUID,
			Vendor:       "imuslab.com",
			BuildVersion: version,
		}, "")
		if err != nil {
			SystemWideLogger.Println("Unable to startup mDNS service. Disabling mDNS services")
		} else {
			//Start initial scanning
			go func() {
				hosts := mdnsScanner.Scan(30, "")
				previousmdnsScanResults = hosts
				SystemWideLogger.Println("mDNS Startup scan completed")
			}()

			//Create a ticker to update mDNS results every 5 minutes
			ticker := time.NewTicker(15 * time.Minute)
			stopChan := make(chan bool)
			go func() {
				for {
					select {
					case <-stopChan:
						ticker.Stop()
					case <-ticker.C:
						hosts := mdnsScanner.Scan(30, "")
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
	streamProxyManager = streamproxy.NewStreamProxy(&streamproxy.Options{
		Database:             sysdb,
		AccessControlHandler: accessController.DefaultAccessRule.AllowConnectionAccess,
	})

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
	acmeAutoRenewer, err = acme.NewAutoRenewer("./conf/acme_conf.json", "./conf/certs/", int64(*acmeAutoRenewInterval), acmeHandler)
	if err != nil {
		log.Fatal(err)
	}

	/* Docker UX Optimizer */
	if runtime.GOOS == "windows" && *runningInDocker {
		SystemWideLogger.PrintAndLog("WARNING", "Invalid start flag combination: docker=true && runtime.GOOS == windows. Running in docker UX development mode.", nil)
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
