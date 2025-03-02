package main

import (
	"fmt"
	"net/http"
	"strconv"

	plugin "aroz.org/zoraxy/example/static_capture/mod/zoraxy_plugin"
)

const (
	PLUGIN_ID              = "org.aroz.zoraxy.static_capture"
	UI_PATH                = "/ui"
	STATIC_CAPTURE_INGRESS = "/static_capture"
)

func main() {
	// Serve the plugin intro spect
	// This will print the plugin intro spect and exit if the -introspect flag is provided
	runtimeCfg, err := plugin.ServeAndRecvSpec(&plugin.IntroSpect{
		ID:            PLUGIN_ID,
		Name:          "Static Capture Example",
		Author:        "aroz.org",
		AuthorContact: "https://aroz.org",
		Description:   "An example plugin implementing static capture",
		URL:           "https://zoraxy.aroz.org",
		Type:          plugin.PluginType_Router,
		VersionMajor:  1,
		VersionMinor:  0,
		VersionPatch:  0,

		/*
			Static Capture Settings

			Once plugin is enabled these rules always applies to the enabled HTTP Proxy rule
			This is faster than dynamic capture, but less flexible

			In this example, we will capture two paths
			/test_a and /test_b. Once this plugin is enabled on a HTTP proxy rule, let say
			https://example.com the plugin will capture all requests to
			https://example.com/test_a and https://example.com/test_b
			and reverse proxy it to the StaticCaptureIngress path of your plugin like
			/static_capture/test_a and /static_capture/test_b
		*/
		StaticCapturePaths: []plugin.StaticCaptureRule{
			{
				CapturePath: "/test_a/",
			},
			{
				CapturePath: "/test_b/",
			},
		},
		StaticCaptureIngress: STATIC_CAPTURE_INGRESS,

		UIPath: UI_PATH,
	})
	if err != nil {
		//Terminate or enter standalone mode here
		panic(err)
	}

	/*
		Static Capture Router

		The plugin library already provided a path router to handle the static capture
		paths. The path router will capture the requests to the specified paths and
		restore the original request path before forwarding it to the handler.

		In this example, we will create a path router to handle the two capture paths
		/test_a and /test_b. The path router will capture the requests to these paths
		and print the request headers to the console.
	*/
	pathRouter := plugin.NewPathRouter()
	pathRouter.SetDebugPrintMode(true)
	pathRouter.RegisterPathHandler("/test_a/", http.HandlerFunc(HandleCaptureA))
	pathRouter.RegisterPathHandler("/test_b/", http.HandlerFunc(HandleCaptureB))
	pathRouter.SetDefaultHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//In theory this should never be called
		//but just in case the request is not captured by the path handlers
		//like if you have forgotten to register the handler for the capture path
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("This request is captured by the default handler!<br>Request URI: " + r.URL.String()))
	}))
	//Lastly, register the path routing to the default http mux (or the mux you are using)
	pathRouter.RegisterHandle(STATIC_CAPTURE_INGRESS, http.DefaultServeMux)

	//Create a path handler for the UI
	http.HandleFunc(UI_PATH+"/", RenderDebugUI)
	fmt.Println("Debugger started at http://127.0.0.1:" + strconv.Itoa(runtimeCfg.Port))
	http.ListenAndServe("127.0.0.1:"+strconv.Itoa(runtimeCfg.Port), nil)
}

// Handle the captured request
func HandleCaptureA(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Request captured by A handler")
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte("This request is captured by A handler!<br>Request URI: " + r.URL.String()))
}

func HandleCaptureB(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Request captured by B handler")
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte("This request is captured by the B handler!<br>Request URI: " + r.URL.String()))
}
