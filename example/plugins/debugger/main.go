package main

import (
	"fmt"
	"net/http"
	"strconv"

	plugin "aroz.org/zoraxy/debugger/mod/zoraxy_plugin"
)

const (
	PLUGIN_ID              = "org.aroz.zoraxy.debugger"
	UI_PATH                = "/debug"
	STATIC_CAPTURE_INGRESS = "/s_capture"
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

		StaticCapturePaths: []plugin.StaticCaptureRule{
			{
				CapturePath: "/test_a",
			},
			{
				CapturePath: "/test_b",
			},
		},
		StaticCaptureIngress: "/s_capture",

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

	//Create a path handler for the capture paths
	pathRouter := plugin.NewPathRouter()
	pathRouter.SetDebugPrintMode(true)
	pathRouter.RegisterPathHandler("/test_a", http.HandlerFunc(HandleCaptureA))
	pathRouter.RegisterPathHandler("/test_b", http.HandlerFunc(HandleCaptureB))
	pathRouter.SetDefaultHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//In theory this should never be called
		//but just in case the request is not captured by the path handlers
		//this will be the fallback handler
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("This request is captured by the default handler!<br>Request URI: " + r.URL.String()))
	}))
	pathRouter.RegisterHandle(STATIC_CAPTURE_INGRESS, http.DefaultServeMux)

	http.HandleFunc(UI_PATH+"/", RenderDebugUI)
	fmt.Println("Debugger started at http://127.0.0.1:" + strconv.Itoa(runtimeCfg.Port))
	http.ListenAndServe("127.0.0.1:"+strconv.Itoa(runtimeCfg.Port), nil)
}

// Handle the captured request
func HandleCaptureA(w http.ResponseWriter, r *http.Request) {
	for key, values := range r.Header {
		for _, value := range values {
			fmt.Printf("%s: %s\n", key, value)
		}
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte("This request is captured by A handler!<br>Request URI: " + r.URL.String()))
}

func HandleCaptureB(w http.ResponseWriter, r *http.Request) {
	for key, values := range r.Header {
		for _, value := range values {
			fmt.Printf("%s: %s\n", key, value)
		}
	}
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte("This request is captured by the B handler!<br>Request URI: " + r.URL.String()))
}
