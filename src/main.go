package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"imuslab.com/zoraxy/mod/aroz"
	"imuslab.com/zoraxy/mod/auth"
	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/dynamicproxy/redirection"
	"imuslab.com/zoraxy/mod/geodb"
	"imuslab.com/zoraxy/mod/statistic"
	"imuslab.com/zoraxy/mod/tlscert"
	"imuslab.com/zoraxy/mod/upnp"
	"imuslab.com/zoraxy/mod/uptime"
)

//General flags
var noauth = flag.Bool("noauth", false, "Disable authentication for management interface")
var showver = flag.Bool("version", false, "Show version of this server")

var (
	name    = "Zoraxy"
	version = "2.11"

	handler            *aroz.ArozHandler      //Handle arozos managed permission system
	sysdb              *database.Database     //System database
	authAgent          *auth.AuthAgent        //Authentication agent
	tlsCertManager     *tlscert.Manager       //TLS / SSL management
	redirectTable      *redirection.RuleTable //Handle special redirection rule sets
	geodbStore         *geodb.Store           //GeoIP database
	statisticCollector *statistic.Collector   //Collecting statistic from visitors
	upnpClient         *upnp.UPnPClient       //UPnP Client for poking holes
	uptimeMonitor      *uptime.Monitor        //Uptime monitor service worker
)

// Kill signal handler. Do something before the system the core terminate.
func SetupCloseHandler() {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Println("\r- Shutting down " + name)
		geodbStore.Close()
		statisticCollector.Close()

		//Close database, final
		sysdb.Close()
		os.Exit(0)
	}()
}

func main() {
	//Start the aoModule pipeline (which will parse the flags as well). Pass in the module launch information
	handler = aroz.HandleFlagParse(aroz.ServiceInfo{
		Name:        name,
		Desc:        "Dynamic Reverse Proxy Server",
		Group:       "Network",
		IconPath:    "reverseproxy/img/small_icon.png",
		Version:     version,
		StartDir:    "reverseproxy/index.html",
		SupportFW:   true,
		LaunchFWDir: "reverseproxy/index.html",
		SupportEmb:  false,
		InitFWSize:  []int{1080, 580},
	})

	if *showver {
		fmt.Println(name + " - Version " + version)
		os.Exit(0)
	}

	SetupCloseHandler()

	//Create database
	db, err := database.NewDatabase("sys.db", false)
	if err != nil {
		log.Fatal(err)
	}
	sysdb = db
	//Create tables for the database
	sysdb.NewTable("settings")

	//Create an auth agent
	sessionKey, err := auth.GetSessionKey(sysdb)
	if err != nil {
		log.Fatal(err)
	}
	authAgent = auth.NewAuthenticationAgent(name, []byte(sessionKey), sysdb, true, func(w http.ResponseWriter, r *http.Request) {
		//Not logged in. Redirecting to login page
		http.Redirect(w, r, "/login.html", http.StatusTemporaryRedirect)
	})

	//Create a TLS certificate manager
	tlsCertManager, err = tlscert.NewManager("./certs")
	if err != nil {
		panic(err)
	}

	//Create a redirection rule table
	redirectTable, err = redirection.NewRuleTable("./rules")
	if err != nil {
		panic(err)
	}

	//Create a geodb store
	geodbStore, err = geodb.NewGeoDb(sysdb, "./system/GeoLite2-Country.mmdb")
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

	if err != nil {
		panic(err)
	}

	//Create a upnp client
	err = initUpnp()
	if err != nil {
		panic(err)
	}

	//Initiate management interface APIs
	initAPIs()

	//Start the reverse proxy server in go routine
	go func() {
		ReverseProxtInit()
	}()

	time.Sleep(500 * time.Millisecond)
	//Any log println will be shown in the core system via STDOUT redirection. But not STDIN.
	log.Println("ReverseProxy started. Visit control panel at http://localhost" + handler.Port)
	err = http.ListenAndServe(handler.Port, nil)

	if err != nil {
		log.Fatal(err)
	}

}
