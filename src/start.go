package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"imuslab.com/zoraxy/mod/acme"
	"imuslab.com/zoraxy/mod/auth"
	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/dynamicproxy/redirection"
	"imuslab.com/zoraxy/mod/ganserv"
	"imuslab.com/zoraxy/mod/geodb"
	"imuslab.com/zoraxy/mod/mdns"
	"imuslab.com/zoraxy/mod/netstat"
	"imuslab.com/zoraxy/mod/pathrule"
	"imuslab.com/zoraxy/mod/sshprox"
	"imuslab.com/zoraxy/mod/statistic"
	"imuslab.com/zoraxy/mod/statistic/analytic"
	"imuslab.com/zoraxy/mod/tcpprox"
	"imuslab.com/zoraxy/mod/tlscert"
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
	sessionKey, err := auth.GetSessionKey(sysdb)
	if err != nil {
		log.Fatal(err)
	}
	authAgent = auth.NewAuthenticationAgent(name, []byte(sessionKey), sysdb, true, func(w http.ResponseWriter, r *http.Request) {
		//Not logged in. Redirecting to login page
		http.Redirect(w, r, ppf("/login.html"), http.StatusTemporaryRedirect)
	})

	//Create a TLS certificate manager
	tlsCertManager, err = tlscert.NewManager("./conf/certs", development)
	if err != nil {
		panic(err)
	}

	//Create a redirection rule table
	redirectTable, err = redirection.NewRuleTable("./conf/redirect")
	if err != nil {
		panic(err)
	}

	//Create a geodb store
	geodbStore, err = geodb.NewGeoDb(sysdb)
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

	//Create a netstat buffer
	netstatBuffers, err = netstat.NewNetStatBuffer(300)
	if err != nil {
		log.Println("Failed to load network statistic info")
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
		portInt, err := strconv.Atoi(strings.Split(handler.Port, ":")[1])
		if err != nil {
			portInt = 8000
		}
		mdnsScanner, err = mdns.NewMDNS(mdns.NetworkHost{
			HostName:     "zoraxy_" + nodeUUID,
			Port:         portInt,
			Domain:       "zoraxy.arozos.com",
			Model:        "Network Gateway",
			UUID:         nodeUUID,
			Vendor:       "imuslab.com",
			BuildVersion: version,
		}, "")
		if err != nil {
			log.Println("Unable to startup mDNS service. Disabling mDNS services")
		} else {
			//Start initial scanning
			go func() {
				hosts := mdnsScanner.Scan(30, "")
				previousmdnsScanResults = hosts
				log.Println("mDNS Startup scan completed")
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
						log.Println("mDNS scan result updated")
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
			log.Println("Failed to load ZeroTier controller API authtoken")
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
	tcpProxyManager = tcpprox.NewTCProxy(&tcpprox.Options{
		Database:             sysdb,
		AccessControlHandler: geodbStore.AllowConnectionAccess,
	})

	//Create WoL MAC storage table
	sysdb.NewTable("wolmac")

	//Create an email sender if SMTP config exists
	sysdb.NewTable("smtp")
	EmailSender = loadSMTPConfig()

	//Create an analytic loader
	AnalyticLoader = analytic.NewDataLoader(sysdb, statisticCollector)

	/*
		ACME API

		Obtaining certificates from ACME Server
	*/
	acmeHandler = initACME()
	acmeAutoRenewer, err = acme.NewAutoRenewer("./conf/acme_conf.json", "./conf/certs/", int64(*acmeAutoRenewInterval), acmeHandler)
	if err != nil {
		log.Fatal(err)
	}
}

// This sequence start after everything is initialized
func finalSequence() {
	//Start ACME renew agent
	acmeRegisterSpecialRoutingRule()
}
