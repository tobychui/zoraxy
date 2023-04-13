package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
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
	"imuslab.com/zoraxy/mod/utils"
)

//General flags
var noauth = flag.Bool("noauth", false, "Disable authentication for management interface")
var showver = flag.Bool("version", false, "Show version of this server")

var (
	name    = "Zoraxy"
	version = "2.1"

	handler            *aroz.ArozHandler
	sysdb              *database.Database
	authAgent          *auth.AuthAgent
	tlsCertManager     *tlscert.Manager
	redirectTable      *redirection.RuleTable
	geodbStore         *geodb.Store
	statisticCollector *statistic.Collector
	upnpClient         *upnp.UPnPClient
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
		IconPath:    "Zoraxy/img/small_icon.png",
		Version:     version,
		StartDir:    "Zoraxy/index.html",
		SupportFW:   true,
		LaunchFWDir: "Zoraxy/index.html",
		SupportEmb:  false,
		InitFWSize:  []int{1080, 580},
	})

	if *showver {
		fmt.Println(name + " - Version " + version)
		os.Exit(0)
	}

	SetupCloseHandler()

	//Check if all required files are here
	ValidateSystemFiles()

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

//Unzip web.tar.gz if file exists
func ValidateSystemFiles() error {
	if !utils.FileExists("./web") || !utils.FileExists("./system") {
		//Check if the web.tar.gz exists
		if utils.FileExists("./web.tar.gz") {
			//Unzip the file
			f, err := os.Open("./web.tar.gz")
			if err != nil {
				return err
			}

			err = utils.ExtractTarGzipByStream(filepath.Clean("./"), f, true)
			if err != nil {
				return err
			}

			err = f.Close()
			if err != nil {
				return err
			}

			//Delete the web.tar.gz
			os.Remove("./web.tar.gz")
		} else {
			return errors.New("system files not found")
		}
	}
	return errors.New("system files not found or corrupted")

}
