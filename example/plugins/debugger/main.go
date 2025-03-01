package main

import (
	"fmt"
	"net/http"
	"strconv"

	plugin "aroz.org/zoraxy/debugger/mod/zoraxy_plugin"
)

const (
	PLUGIN_ID = "org.aroz.zoraxy.debugger"
	UI_PATH   = "/debug"
)

func main() {
	// Serve the plugin intro spect
	// This will print the plugin intro spect and exit if the -introspect flag is provided
	runtimeCfg, err := plugin.ServeAndRecvSpec(&plugin.IntroSpect{
		ID:            "org.aroz.zoraxy.debugger",
		Name:          "Plugin Debugger",
		Author:        "aroz.org",
		AuthorContact: "https://aroz.org",
		Description:   "A debugger for Zoraxy <-> plugin communication pipeline",
		URL:           "https://zoraxy.aroz.org",
		Type:          plugin.PluginType_Router,
		VersionMajor:  1,
		VersionMinor:  0,
		VersionPatch:  0,

		GlobalCapturePaths: []plugin.CaptureRule{
			{
				CapturePath:     "/debug_test", //Capture all traffic of all HTTP proxy rule
				IncludeSubPaths: true,
			},
		},
		GlobalCaptureIngress: "",
		AlwaysCapturePaths:   []plugin.CaptureRule{},
		AlwaysCaptureIngress: "",

		UIPath: UI_PATH,

		/*
			SubscriptionPath: "/subept",
			SubscriptionsEvents: []plugin.SubscriptionEvent{
		*/
	})
	if err != nil {
		//Terminate or enter standalone mode here
		panic(err)
	}

	// Register the shutdown handler
	plugin.RegisterShutdownHandler(func() {
		// Do cleanup here if needed
		fmt.Println("Debugger Terminated")
	})

	http.HandleFunc(UI_PATH+"/", RenderDebugUI)
	http.HandleFunc("/gcapture", HandleIngressCapture)
	fmt.Println("Debugger started at http://127.0.0.1:" + strconv.Itoa(runtimeCfg.Port))
	http.ListenAndServe("127.0.0.1:"+strconv.Itoa(runtimeCfg.Port), nil)
}

// Handle the captured request
func HandleIngressCapture(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "Capture request received")
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte("This request is captured by the debugger"))
}
