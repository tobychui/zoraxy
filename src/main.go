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
	"strings"
	"syscall"

	"github.com/google/uuid"
	"github.com/gorilla/csrf"
	"imuslab.com/zoraxy/mod/geodb"
	"imuslab.com/zoraxy/mod/update"
	"imuslab.com/zoraxy/mod/utils"
)

/* SIGTERM handler, do shutdown sequences before closing */
func SetupCloseHandler() {
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-c
		ShutdownSeq()
		os.Exit(0)
	}()
}

func main() {
	//Parse startup flags
	flag.Parse()

	/* Maintaince Function Modes */
	if *showver {
		fmt.Println(SYSTEM_NAME + " - Version " + SYSTEM_VERSION)
		os.Exit(0)
	}
	if *geoDbUpdate {
		geodb.DownloadGeoDBUpdate(CONF_GEODB_PATH)
		os.Exit(0)
	}

	/* Main Zoraxy Routines */
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
	uuidRecord := *path_uuid
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

	//Create a new webmin mux, plugin mux and csrf middleware layer
	webminPanelMux = http.NewServeMux()
	pluginAPIMux := http.NewServeMux()
	csrfMiddleware = csrf.Protect(
		[]byte(nodeUUID),
		csrf.CookieName(CSRF_COOKIENAME),
		csrf.Secure(false),
		csrf.Path("/"),
		csrf.SameSite(csrf.SameSiteLaxMode),
	)

	//Startup all modules, see start.go
	startupSequence()

	//Initiate APIs
	requireAuth = !(*noauth)
	initAPIs(webminPanelMux)
	initRestAPI(pluginAPIMux)

	// Create a entry mux to accept all management interface requests
	entryMux := http.NewServeMux()
	entryMux.Handle("/plugin/", pluginAPIMux)            //For plugins API access
	entryMux.Handle("/", csrfMiddleware(webminPanelMux)) //For webmin UI access, require csrf token

	// Start the reverse proxy server in go routine
	go func() {
		ReverseProxyInit()
	}()

	// Wait for dynamicProxyRouter to be initialized before proceeding
	// See ReverseProxyInit() in reverseproxy.go
	<-dynamicProxyRouterReady

	//Start the finalize sequences
	finalSequence()

	if strings.HasPrefix(*webUIPort, ":") {
		//Bind to all interfaces, issue #672
		SystemWideLogger.Println(SYSTEM_NAME + " started. Visit control panel at http://localhost" + *webUIPort)
	} else {
		SystemWideLogger.Println(SYSTEM_NAME + " started. Visit control panel at http://" + *webUIPort)
	}

	err = http.ListenAndServe(*webUIPort, entryMux)

	if err != nil {
		log.Fatal(err)
	}
}
