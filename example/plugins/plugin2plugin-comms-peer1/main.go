package main

import (
	"embed"
	"fmt"
	"net/http"
	"strconv"

	plugin "aroz.org/zoraxy/plugins/plugin2plugin-comms-peer1/mod/zoraxy_plugin"
)

// Notes:
// This plugin handles updating the UI with new messages received from the peer plugin via SSE, other option you
// could use are WebSockets or polling the server at intervals

const (
	PLUGIN_ID         = "org.aroz.zoraxy.plugin2plugin_comms_peer1"
	PEER_ID           = "org.aroz.zoraxy.plugin2plugin_comms_peer2"
	UI_PATH           = "/ui"
	SUBSCRIPTION_PATH = "/notifyme"
	WEB_ROOT          = "/www"
)

//go:embed www/*
var content embed.FS

func main() {
	// Serve the plugin intro spect
	// This will print the plugin intro spect and exit if the -introspect flag is provided
	runtimeCfg, err := plugin.ServeAndRecvSpec(&plugin.IntroSpect{
		ID:            PLUGIN_ID,
		Name:          "Plugin2Plugin Comms Peer 1",
		Author:        "Anthony Rubick",
		AuthorContact: "",
		Description:   "An example plugin for demonstrating plugin to plugin communications - Peer 1",
		Type:          plugin.PluginType_Utilities,
		VersionMajor:  1,
		VersionMinor:  0,
		VersionPatch:  0,

		UIPath: UI_PATH,

		/* API Access Control */
		PermittedAPIEndpoints: []plugin.PermittedAPIEndpoint{
			{
				Method:   http.MethodPost,
				Endpoint: "/plugin/event/emit",
				Reason:   "Used to send events to the peer plugin",
			},
		},

		/* Subscriptions Settings */
		SubscriptionPath: SUBSCRIPTION_PATH,
		SubscriptionsEvents: map[string]string{
			"dummy": "A dummy event to satisfy the requirement of having at least one event",
		},
	})

	if err != nil {
		fmt.Printf("Error serving introspect: %v\n", err)
		return
	}

	// Start the HTTP server
	embedWebRouter := plugin.NewPluginEmbedUIRouter(PLUGIN_ID, &content, WEB_ROOT, UI_PATH)
	// for debugging, use the following line instead
	// embedWebRouter := plugin.NewPluginEmbedUIRouter(PLUGIN_ID, "."+ WEB_ROOT, UI_PATH)

	embedWebRouter.RegisterTerminateHandler(func() {
		// Do cleanup here if needed
		fmt.Println("Plugin Exited")
	}, nil)

	// Serve the API
	RegisterAPIs(runtimeCfg)

	// Serve the web page in the www folder
	http.Handle(UI_PATH+"/", embedWebRouter.Handler())
	http.HandleFunc(SUBSCRIPTION_PATH+"/", handleReceivedEvent)
	fmt.Println("Plugin2Plugin Comms Peer 1 started at http://127.0.0.1:" + strconv.Itoa(runtimeCfg.Port))
	err = http.ListenAndServe("127.0.0.1:"+strconv.Itoa(runtimeCfg.Port), nil)
	if err != nil {
		panic(err)
	}
}

func RegisterAPIs(cfg *plugin.ConfigureSpec) {
	// Add API handlers here
	http.HandleFunc(UI_PATH+"/api/send_message", func(w http.ResponseWriter, r *http.Request) {
		handleSendMessage(cfg, w, r)
	})
	http.HandleFunc(UI_PATH+"/api/events", handleSSE)
	http.HandleFunc(UI_PATH+"/api/message_history", handleFetchMessageHistory)
}
