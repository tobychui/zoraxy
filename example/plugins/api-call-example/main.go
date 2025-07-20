package main

import (
	"fmt"
	"net/http"

	plugin "aroz.org/zoraxy/api-call-example/mod/zoraxy_plugin"
)

const (
	PLUGIN_ID = "org.aroz.zoraxy.api_call_example"
	UI_PATH   = "/ui"
)

func main() {
	// Serve the plugin intro spect
	// This will print the plugin intro spect and exit if the -introspect flag is provided
	runtimeCfg, err := plugin.ServeAndRecvSpec(&plugin.IntroSpect{
		ID:            PLUGIN_ID,
		Name:          "API Call Example Plugin",
		Author:        "Anthony Rubick",
		AuthorContact: "",
		Description:   "An example plugin for making API calls",
		Type:          plugin.PluginType_Utilities,
		VersionMajor:  1,
		VersionMinor:  0,
		VersionPatch:  0,

		UIPath: UI_PATH,

		/* API Access Control */
		PermittedAPIEndpoints: []plugin.PermittedAPIEndpoint{
			{
				Method:   http.MethodGet,
				Endpoint: "/plugin/api/access/list",
				Reason:   "Used to display all configured Access Rules",
			},
		},
	})

	if err != nil {
		fmt.Printf("Error serving introspect: %v\n", err)
		return
	}

	// Start the HTTP server
	http.HandleFunc(UI_PATH+"/", func(w http.ResponseWriter, r *http.Request) {
		RenderUI(runtimeCfg, w, r)
	})

	serverAddr := fmt.Sprintf("127.0.0.1:%d", runtimeCfg.Port)
	fmt.Printf("Starting API Call Example Plugin on %s\n", serverAddr)
	http.ListenAndServe(serverAddr, nil)
}
