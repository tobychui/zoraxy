package main

import (
	_ "embed"
	"fmt"
	"net/http"
	"strconv"

	plugin "example.com/zoraxy/helloworld/zoraxy_plugin"
)

//go:embed index.html
var indexHTML string

func helloWorldHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, indexHTML)
}

func main() {
	// Serve the plugin intro spect
	// This will print the plugin intro spect and exit if the -introspect flag is provided
	runtimeCfg, err := plugin.ServeAndRecvSpec(&plugin.IntroSpect{
		ID:            "com.example.helloworld",
		Name:          "Hello World Plugin",
		Author:        "foobar",
		AuthorContact: "admin@example.com",
		Description:   "A simple hello world plugin",
		URL:           "https://example.com",
		Type:          plugin.PluginType_Utilities,
		VersionMajor:  1,
		VersionMinor:  0,
		VersionPatch:  0,

		// As this is a utility plugin, we don't need to capture any traffic
		// but only serve the UI, so we set the UI (relative to the plugin path) to "/"
		UIPath: "/",
	})

	if err != nil {
		//Terminate or enter standalone mode here
		panic(err)
	}

	// Serve the hello world page
	// This will serve the index.html file embedded in the binary
	http.HandleFunc("/", helloWorldHandler)
	fmt.Println("Server started at http://localhost:" + strconv.Itoa(runtimeCfg.Port))
	http.ListenAndServe(":"+strconv.Itoa(runtimeCfg.Port), nil)
}
