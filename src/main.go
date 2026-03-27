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
	"path/filepath"
	"strings"
	"syscall"

	"github.com/google/uuid"
	"github.com/gorilla/csrf"
	"imuslab.com/zoraxy/mod/auth"
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

func loadOrCreateSystemUUID(uuidRecord string) (string, error) {
	uuidRecord = strings.TrimSpace(uuidRecord)
	if uuidRecord == "" {
		return "", fmt.Errorf("UUID file path is empty")
	}

	if err := os.MkdirAll(filepath.Dir(uuidRecord), 0775); err != nil {
		return "", fmt.Errorf("unable to create UUID directory: %w", err)
	}

	if !utils.FileExists(uuidRecord) {
		newSystemUUID := uuid.New().String()
		if err := os.WriteFile(uuidRecord, []byte(newSystemUUID), 0775); err != nil {
			return "", fmt.Errorf("unable to create node UUID file %s: %w", uuidRecord, err)
		}
	}

	uuidBytes, err := os.ReadFile(uuidRecord)
	if err != nil {
		return "", fmt.Errorf("unable to read node UUID from %s: %w", uuidRecord, err)
	}

	systemUUID := strings.TrimSpace(string(uuidBytes))
	if systemUUID == "" {
		return "", fmt.Errorf("node UUID file %s is empty", uuidRecord)
	}

	if _, err := uuid.Parse(systemUUID); err != nil {
		return "", fmt.Errorf("invalid node UUID in %s: %w", uuidRecord, err)
	}

	return systemUUID, nil
}

func applyNodeEnvironmentOverrides() {
	if strings.TrimSpace(*nodeServer) == "" {
		if envNodeServer := strings.TrimSpace(os.Getenv("ZORAXY_NODE_SERVER")); envNodeServer != "" {
			*nodeServer = envNodeServer
		}
	}

	if strings.TrimSpace(*nodeToken) == "" {
		if envNodeToken := strings.TrimSpace(os.Getenv("ZORAXY_NODE_TOKEN")); envNodeToken != "" {
			*nodeToken = envNodeToken
		}
	}

	if strings.TrimSpace(*nodeIP) == "" {
		if envNodeIP := strings.TrimSpace(os.Getenv("ZORAXY_NODE_IP")); envNodeIP != "" {
			*nodeIP = envNodeIP
		}
	}
}

func resolveRuntimeMode() error {
	hasNodeServer := strings.TrimSpace(*nodeServer) != ""
	hasNodeToken := strings.TrimSpace(*nodeToken) != ""

	if hasNodeServer != hasNodeToken {
		return fmt.Errorf("node mode requires both server and token; provide both via flags or ZORAXY_NODE_SERVER/ZORAXY_NODE_TOKEN")
	}

	if hasNodeServer && hasNodeToken {
		*mode = "node"
	} else {
		*mode = "primary"
	}

	return nil
}

func main() {
	//Parse startup flags
	flag.Parse()
	applyNodeEnvironmentOverrides()
	if err := resolveRuntimeMode(); err != nil {
		log.Fatal(err)
	}

	//Initialize path variables from flags
	TMP_FOLDER = *path_tmp
	CONF_FOLDER = *path_conf
	CONF_HTTP_PROXY = CONF_FOLDER + "/proxy"
	CONF_STREAM_PROXY = CONF_FOLDER + "/streamproxy"
	CONF_NODES = CONF_FOLDER + "/nodes"
	CONF_CERT_STORE = CONF_FOLDER + "/certs"
	CONF_REDIRECTION = CONF_FOLDER + "/redirect"
	CONF_ACCESS_RULE = CONF_FOLDER + "/access"
	CONF_PATH_RULE = CONF_FOLDER + "/rules/pathrules"
	CONF_PLUGIN_GROUPS = CONF_FOLDER + "/plugin_groups.json"
	CONF_GEODB_PATH = CONF_FOLDER + "/geodb"
	CONF_LOG_CONFIG = CONF_FOLDER + "/log_conf.json"
	ACME_AUTORENEW_CONFIG_PATH = CONF_FOLDER + "/acme_conf.json"
	CONF_TRUSTED_PROXIES = CONF_FOLDER + "/trusted_proxies.json"

	/* Maintaince Function Modes */
	if *showver {
		fmt.Println(SYSTEM_NAME + " - Version " + SYSTEM_VERSION)
		os.Exit(0)
	}
	if *geoDbUpdate {
		geodb.DownloadGeoDBUpdate(CONF_GEODB_PATH)
		os.Exit(0)
	}
	if *reset_account {
		//Reset admin account by removing all users from the system
		auth.ResetAccount(databaseBackend, path_database)
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

	if *mode == "node" {
		loadedUUID, err := loadOrCreateSystemUUID(*path_uuid)
		if err != nil {
			log.Fatal(err)
		}
		nodeUUID = loadedUUID

		if *nodeToken == "" {
			log.Fatal("Node token cannot be empty")
		}

		if *nodeServer == "" {
			log.Fatal("Node server cannot be empty")
		}
	} else {
		loadedUUID, err := loadOrCreateSystemUUID(*path_uuid)
		if err != nil {
			log.Println("Unable to read system uuid from file system:", err)
			panic(err)
		}
		nodeUUID = loadedUUID
	}

	//Create a new webmin mux, plugin mux and csrf middleware layer
	webminPanelMux = http.NewServeMux()
	pluginAPIMux := http.NewServeMux()
	nodeAPIMux := http.NewServeMux()

	//Startup all modules, see start.go
	startupSequence()

	csrfMiddleware = csrf.Protect(
		[]byte(nodeUUID),
		csrf.CookieName(CSRF_COOKIENAME),
		csrf.Secure(false),
		csrf.Path("/"),
		csrf.SameSite(csrf.SameSiteLaxMode),
	)

	//Initiate APIs
	requireAuth = !(*noauth)
	initAPIs(webminPanelMux)
	initRestAPI(pluginAPIMux)
	initNodeAPI(nodeAPIMux, webminPanelMux)

	// Create a entry mux to accept all management interface requests
	entryMux := http.NewServeMux()
	entryMux.Handle("/plugin/", pluginAPIMux)            //For plugins API access
	entryMux.Handle("/node/", nodeAPIMux)                //For nodes API access
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

	if *mode == "node" {
		if err := applyDesiredLocalNodeServiceState(); err != nil {
			SystemWideLogger.PrintAndLog("nodes", "Failed to apply desired node service state", err)
		}
		go func() {
			_ = nodeManager.SyncNow()
		}()
	}

	if strings.HasPrefix(*webUIPort, ":") {
		//Bind to all interfaces, issue #672
		SystemWideLogger.Println(SYSTEM_NAME + " started. Visit control panel at http://localhost" + *webUIPort)
	} else {
		SystemWideLogger.Println(SYSTEM_NAME + " started. Visit control panel at http://" + *webUIPort)
	}

	err := http.ListenAndServe(*webUIPort, entryMux)

	if err != nil {
		log.Fatal(err)
	}
}
