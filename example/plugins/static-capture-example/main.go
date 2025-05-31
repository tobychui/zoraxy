package main

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"

	plugin "example.com/zoraxy/static-capture-example/mod/zoraxy_plugin"
)

const (
	PLUGIN_ID              = "org.aroz.zoraxy.static-capture-example"
	UI_PATH                = "/ui"
	STATIC_CAPTURE_INGRESS = "/s_capture"
)

func main() {
	// Serve the plugin intro spect
	// This will print the plugin intro spect and exit if the -introspect flag is provided
	runtimeCfg, err := plugin.ServeAndRecvSpec(&plugin.IntroSpect{
		ID:            "org.aroz.zoraxy.static-capture-example",
		Name:          "Static Capture Example",
		Author:        "aroz.org",
		AuthorContact: "https://aroz.org",
		Description:   "An example for showing how static capture works in Zoraxy.",
		URL:           "https://zoraxy.aroz.org",
		Type:          plugin.PluginType_Router,
		VersionMajor:  1,
		VersionMinor:  0,
		VersionPatch:  0,

		StaticCapturePaths: []plugin.StaticCaptureRule{
			{
				CapturePath: "/test_a", // This is the path that will be captured by the static capture handler
			},
			{
				CapturePath: "/test_b", // This is another path that will be captured by the static capture handler
			},
		},
		StaticCaptureIngress: "/s_capture", // This is the ingress path for static capture requests

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

	// Setup the path router
	pathRouter := plugin.NewPathRouter()
	//pathRouter.SetDebugPrintMode(true)

	/*
		Static Routers
	*/
	pathRouter.RegisterPathHandler("/test_a", http.HandlerFunc(HandleCaptureA))
	pathRouter.RegisterPathHandler("/test_b", http.HandlerFunc(HandleCaptureB))
	pathRouter.SetDefaultHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//	In theory this should never be called
		//	except when there is registered static path in Introspect but you don't create a handler for it (usually a mistake)
		//	but just in case the request is not captured by the path handlers
		//	this will be the fallback handler
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("This request is captured by the default handler!<br>Request URI: " + r.URL.String()))
	}))
	pathRouter.RegisterStaticCaptureHandle(STATIC_CAPTURE_INGRESS, http.DefaultServeMux)

	// To simplify the example, we will use the default HTTP ServeMux
	http.HandleFunc(UI_PATH+"/", RenderDebugUI)
	fmt.Println("Static path capture example started at http://127.0.0.1:" + strconv.Itoa(runtimeCfg.Port))
	http.ListenAndServe("127.0.0.1:"+strconv.Itoa(runtimeCfg.Port), nil)
}

// Handle the captured request
func HandleCaptureA(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte("This request is captured by A handler!<br>Request URI: " + r.URL.String()))
}

func HandleCaptureB(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte("This request is captured by the B handler!<br>Request URI: " + r.URL.String()))
}

// Render the debug UI
func RenderDebugUI(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "**Plugin UI Debug Interface**\n\n[Recv Headers] \n")

	headerKeys := make([]string, 0, len(r.Header))
	for name := range r.Header {
		headerKeys = append(headerKeys, name)
	}
	sort.Strings(headerKeys)
	for _, name := range headerKeys {
		values := r.Header[name]
		for _, value := range values {
			fmt.Fprintf(w, "%s: %s\n", name, value)
		}
	}
	w.Header().Set("Content-Type", "text/html")
}
