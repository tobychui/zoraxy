package main

/*
  ______
 |___  /
    / / ___  _ __ __ ___  ___   _
   / / / _ \| '__/ _` \ \/ / | | |
  / /_| (_) | | | (_| |>  <| |_| |
 /_____\___/|_|  \__,_/_/\_\\__, |
                             __/ |
                            |___/

Zoraxy - A general purpose HTTP reverse proxy and forwarding tool
Author: tobychui
License: AGPLv3

--------------------------------------------

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, version 3 of the License or any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

*/

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/csrf"
	"imuslab.com/zoraxy/mod/update"
	"imuslab.com/zoraxy/mod/utils"
)

/* SIGTERM handler, do shutdown sequences before closing */
func SetupCloseHandler() {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		ShutdownSeq()
		os.Exit(0)
	}()
}

func ShutdownSeq() {
	SystemWideLogger.Println("Shutting down " + SYSTEM_NAME)
	SystemWideLogger.Println("Closing Netstats Listener")
	if netstatBuffers != nil {
		netstatBuffers.Close()
	}

	SystemWideLogger.Println("Closing Statistic Collector")
	if statisticCollector != nil {
		statisticCollector.Close()
	}

	if mdnsTickerStop != nil {
		SystemWideLogger.Println("Stopping mDNS Discoverer (might take a few minutes)")
		// Stop the mdns service
		mdnsTickerStop <- true
	}
	if mdnsScanner != nil {
		mdnsScanner.Close()
	}
	SystemWideLogger.Println("Shutting down load balancer")
	if loadBalancer != nil {
		loadBalancer.Close()
	}
	SystemWideLogger.Println("Closing Certificates Auto Renewer")
	if acmeAutoRenewer != nil {
		acmeAutoRenewer.Close()
	}
	//Remove the tmp folder
	SystemWideLogger.Println("Cleaning up tmp files")
	os.RemoveAll("./tmp")

	//Close database
	SystemWideLogger.Println("Stopping system database")
	sysdb.Close()

	//Close logger
	SystemWideLogger.Println("Closing system wide logger")
	SystemWideLogger.Close()
}

func main() {
	//Parse startup flags
	flag.Parse()
	if *showver {
		fmt.Println(SYSTEM_NAME + " - Version " + SYSTEM_VERSION)
		os.Exit(0)
	}

	if !utils.ValidateListeningAddress(*webUIPort) {
		fmt.Println("Malformed -port (listening address) paramter. Do you mean -port=:" + *webUIPort + "?")
		os.Exit(0)
	}

	if *enableAutoUpdate {
		fmt.Println("Checking required config update")
		update.RunConfigUpdate(0, update.GetVersionIntFromVersionNumber(SYSTEM_VERSION))
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
		SystemWideLogger.PrintAndLog("ZeroTier", "Unable to read system uuid from file system", nil)
		panic(err)
	}
	nodeUUID = string(uuidBytes)

	//Create a new webmin mux and csrf middleware layer
	webminPanelMux = http.NewServeMux()
	csrfMiddleware = csrf.Protect(
		[]byte(nodeUUID),
		csrf.CookieName(CSRF_COOKIENAME),
		csrf.Secure(false),
		csrf.Path("/"),
		csrf.SameSite(csrf.SameSiteLaxMode),
	)

	//Startup all modules
	startupSequence()

	//Initiate management interface APIs
	requireAuth = !(*noauth)
	initAPIs(webminPanelMux)

	//Start the reverse proxy server in go routine
	go func() {
		ReverseProxtInit()
	}()

	time.Sleep(500 * time.Millisecond)

	//Start the finalize sequences
	finalSequence()

	SystemWideLogger.Println(SYSTEM_NAME + " started. Visit control panel at http://localhost" + *webUIPort)
	err = http.ListenAndServe(*webUIPort, csrfMiddleware(webminPanelMux))

	if err != nil {
		log.Fatal(err)
	}
}
