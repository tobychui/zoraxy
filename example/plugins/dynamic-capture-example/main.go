package main

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	plugin "example.com/zoraxy/dynamic-capture-example/mod/zoraxy_plugin"
)

const (
	PLUGIN_ID              = "org.aroz.zoraxy.dynamic-capture-example"
	UI_PATH                = "/debug"
	STATIC_CAPTURE_INGRESS = "/s_capture"
)

func main() {
	// Serve the plugin intro spect
	// This will print the plugin intro spect and exit if the -introspect flag is provided
	runtimeCfg, err := plugin.ServeAndRecvSpec(&plugin.IntroSpect{
		ID:            "org.aroz.zoraxy.dynamic-capture-example",
		Name:          "Dynamic Capture Example",
		Author:        "aroz.org",
		AuthorContact: "https://aroz.org",
		Description:   "This is an example plugin for Zoraxy that demonstrates how to use dynamic captures.",
		URL:           "https://zoraxy.aroz.org",
		Type:          plugin.PluginType_Router,
		VersionMajor:  1,
		VersionMinor:  0,
		VersionPatch:  0,

		DynamicCaptureSniff:   "/d_sniff",
		DynamicCaptureIngress: "/d_capture",

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
	pathRouter.SetDebugPrintMode(true)

	/*
		Dynamic Captures
	*/
	pathRouter.RegisterDynamicSniffHandler("/d_sniff", http.DefaultServeMux, func(dsfr *plugin.DynamicSniffForwardRequest) plugin.SniffResult {
		//In this example, we want to capture all URI
		//that start with /foobar and forward it to the dynamic capture handler
		if strings.HasPrefix(dsfr.RequestURI, "/foobar") {
			reqUUID := dsfr.GetRequestUUID()
			fmt.Println("Accepting request with UUID: " + reqUUID)

			// Print all the values of the request
			fmt.Println("Method:", dsfr.Method)
			fmt.Println("Hostname:", dsfr.Hostname)
			fmt.Println("URL:", dsfr.URL)
			fmt.Println("Header:")
			for key, values := range dsfr.Header {
				for _, value := range values {
					fmt.Printf("  %s: %s\n", key, value)
				}
			}
			fmt.Println("RemoteAddr:", dsfr.RemoteAddr)
			fmt.Println("Host:", dsfr.Host)
			fmt.Println("RequestURI:", dsfr.RequestURI)
			fmt.Println("Proto:", dsfr.Proto)
			fmt.Println("ProtoMajor:", dsfr.ProtoMajor)
			fmt.Println("ProtoMinor:", dsfr.ProtoMinor)

			// We want to handle this request, reply with aSniffResultAccept
			return plugin.SniffResultAccpet
		}

		// If the request URI does not match, we skip this request
		fmt.Println("Skipping request with UUID: " + dsfr.GetRequestUUID())
		return plugin.SniffResultSkip
	})
	pathRouter.RegisterDynamicCaptureHandle("/d_capture", http.DefaultServeMux, func(w http.ResponseWriter, r *http.Request) {
		// This is the dynamic capture handler where it actually captures and handle the request
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Welcome to the dynamic capture handler!"))

		// Print all the request info to the response writer
		w.Write([]byte("\n\nRequest Info:\n"))
		w.Write([]byte("Request URI: " + r.RequestURI + "\n"))
		w.Write([]byte("Request Method: " + r.Method + "\n"))
		w.Write([]byte("Request Headers:\n"))
		headers := make([]string, 0, len(r.Header))
		for key := range r.Header {
			headers = append(headers, key)
		}
		sort.Strings(headers)
		for _, key := range headers {
			for _, value := range r.Header[key] {
				w.Write([]byte(fmt.Sprintf("%s: %s\n", key, value)))
			}
		}
	})

	http.HandleFunc(UI_PATH+"/", RenderDebugUI)
	fmt.Println("Dynamic capture example started at http://127.0.0.1:" + strconv.Itoa(runtimeCfg.Port))
	http.ListenAndServe("127.0.0.1:"+strconv.Itoa(runtimeCfg.Port), nil)
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
