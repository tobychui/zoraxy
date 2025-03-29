package main

import (
	"fmt"
	"net/http"
	"strconv"

	"embed"

	"aroz.org/zoraxy/ztnc/mod/database"
	"aroz.org/zoraxy/ztnc/mod/ganserv"
	plugin "aroz.org/zoraxy/ztnc/mod/zoraxy_plugin"
)

const (
	PLUGIN_ID       = "org.aroz.zoraxy.ztnc"
	UI_RELPATH      = "/ui"
	EMBED_FS_ROOT   = "/web"
	DB_FILE_PATH    = "ztnc.db"
	AUTH_TOKEN_PATH = "./authtoken.secret"
)

//go:embed web/*
var content embed.FS

var (
	sysdb      *database.Database
	ganManager *ganserv.NetworkManager
)

func main() {
	// Serve the plugin intro spect
	runtimeCfg, err := plugin.ServeAndRecvSpec(&plugin.IntroSpect{
		ID:            PLUGIN_ID,
		Name:          "ztnc",
		Author:        "aroz.org",
		AuthorContact: "zoraxy.aroz.org",
		Description:   "UI for ZeroTier Network Controller",
		URL:           "https://zoraxy.aroz.org",
		Type:          plugin.PluginType_Utilities,
		VersionMajor:  1,
		VersionMinor:  0,
		VersionPatch:  0,

		// As this is a utility plugin, we don't need to capture any traffic
		// but only serve the UI, so we set the UI (relative to the plugin path) to "/ui/" to match the HTTP Handler
		UIPath: UI_RELPATH,
	})
	if err != nil {
		//Terminate or enter standalone mode here
		panic(err)
	}

	// Create a new PluginEmbedUIRouter that will serve the UI from web folder
	uiRouter := plugin.NewPluginEmbedUIRouter(PLUGIN_ID, &content, EMBED_FS_ROOT, UI_RELPATH)
	uiRouter.EnableDebug = true

	// Register the shutdown handler
	uiRouter.RegisterTerminateHandler(func() {
		// Do cleanup here if needed
		if sysdb != nil {
			sysdb.Close()
		}
		fmt.Println("ztnc Exited")
	}, nil)

	// This will serve the index.html file embedded in the binary
	targetHandler := uiRouter.Handler()
	http.Handle(UI_RELPATH+"/", targetHandler)

	// Start the GAN Network Controller
	err = startGanNetworkController()
	if err != nil {
		panic(err)
	}

	// Initiate the API endpoints
	initApiEndpoints()

	// Start the HTTP server, only listen to loopback interface
	fmt.Println("Plugin UI server started at http://127.0.0.1:" + strconv.Itoa(runtimeCfg.Port) + UI_RELPATH)
	http.ListenAndServe("127.0.0.1:"+strconv.Itoa(runtimeCfg.Port), nil)
}
