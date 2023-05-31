package main

import (
	"embed"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"imuslab.com/zoraxy/mod/aroz"
	"imuslab.com/zoraxy/mod/auth"
	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/dynamicproxy/redirection"
	"imuslab.com/zoraxy/mod/email"
	"imuslab.com/zoraxy/mod/ganserv"
	"imuslab.com/zoraxy/mod/geodb"
	"imuslab.com/zoraxy/mod/mdns"
	"imuslab.com/zoraxy/mod/netstat"
	"imuslab.com/zoraxy/mod/sshprox"
	"imuslab.com/zoraxy/mod/statistic"
	"imuslab.com/zoraxy/mod/statistic/analytic"
	"imuslab.com/zoraxy/mod/tcpprox"
	"imuslab.com/zoraxy/mod/tlscert"
	"imuslab.com/zoraxy/mod/uptime"
	"imuslab.com/zoraxy/mod/utils"
)

// General flags
var noauth = flag.Bool("noauth", false, "Disable authentication for management interface")
var showver = flag.Bool("version", false, "Show version of this server")
var allowSshLoopback = flag.Bool("sshlb", false, "Allow loopback web ssh connection (DANGER)")
var ztAuthToken = flag.String("ztauth", "", "ZeroTier authtoken for the local node")
var ztAPIPort = flag.Int("ztport", 9993, "ZeroTier controller API port")
var (
	name        = "Zoraxy"
	version     = "2.6.1"
	nodeUUID    = "generic"
	development = true //Set this to false to use embedded web fs
	bootTime    = time.Now().Unix()

	/*
		Binary Embedding File System
	*/
	//go:embed web/*
	webres embed.FS

	/*
		Handler Modules
	*/
	handler            *aroz.ArozHandler       //Handle arozos managed permission system
	sysdb              *database.Database      //System database
	authAgent          *auth.AuthAgent         //Authentication agent
	tlsCertManager     *tlscert.Manager        //TLS / SSL management
	redirectTable      *redirection.RuleTable  //Handle special redirection rule sets
	geodbStore         *geodb.Store            //GeoIP database, also handle black list and whitelist features
	netstatBuffers     *netstat.NetStatBuffers //Realtime graph buffers
	statisticCollector *statistic.Collector    //Collecting statistic from visitors
	uptimeMonitor      *uptime.Monitor         //Uptime monitor service worker
	mdnsScanner        *mdns.MDNSHost          //mDNS discovery services
	ganManager         *ganserv.NetworkManager //Global Area Network Manager
	webSshManager      *sshprox.Manager        //Web SSH connection service
	tcpProxyManager    *tcpprox.Manager        //TCP Proxy Manager

	//Helper modules
	EmailSender    *email.Sender        //Email sender that handle email sending
	AnalyticLoader *analytic.DataLoader //Data loader for Zoraxy Analytic
)

// Kill signal handler. Do something before the system the core terminate.
func SetupCloseHandler() {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("- Shutting down " + name)
		fmt.Println("- Closing GeoDB ")
		geodbStore.Close()
		fmt.Println("- Closing Netstats Listener")
		netstatBuffers.Close()
		fmt.Println("- Closing Statistic Collector")
		statisticCollector.Close()
		fmt.Println("- Stopping mDNS Discoverer")
		//Stop the mdns service
		mdnsTickerStop <- true
		mdnsScanner.Close()

		//Remove the tmp folder
		fmt.Println("- Cleaning up tmp files")
		os.RemoveAll("./tmp")

		//Close database, final
		fmt.Println("- Stopping system database")
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
		IconPath:    "zoraxy/img/small_icon.png",
		Version:     version,
		StartDir:    "zoraxy/index.html",
		SupportFW:   true,
		LaunchFWDir: "zoraxy/index.html",
		SupportEmb:  false,
		InitFWSize:  []int{1080, 580},
	})

	if *showver {
		fmt.Println(name + " - Version " + version)
		os.Exit(0)
	}

	SetupCloseHandler()

	//Read or create the system uuid
	uuidRecord := "./sys.uuid"
	if !utils.FileExists(uuidRecord) {
		newSystemUUID := uuid.New().String()
		os.WriteFile(uuidRecord, []byte(newSystemUUID), 0775)
	}
	uuidBytes, err := os.ReadFile(uuidRecord)
	if err != nil {
		log.Println("Unable to read system uuid from file system")
		panic(err)
	}
	nodeUUID = string(uuidBytes)

	//Startup all modules
	startupSequence()

	//Initiate management interface APIs
	requireAuth = !(*noauth || handler.IsUsingExternalPermissionManager())
	initAPIs()

	//Start the reverse proxy server in go routine
	go func() {
		ReverseProxtInit()
	}()

	time.Sleep(500 * time.Millisecond)

	log.Println("Zoraxy started. Visit control panel at http://localhost" + handler.Port)
	err = http.ListenAndServe(handler.Port, nil)

	if err != nil {
		log.Fatal(err)
	}

}
