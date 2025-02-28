package main

import (
	"embed"
	_ "embed"
	"fmt"
	"net/http"
	"strconv"

	plugin "example.com/zoraxy/helloworld/zoraxy_plugin"
)

const (
	PLUGIN_ID = "com.example.helloworld"
	UI_PATH   = "/"
	WEB_ROOT  = "/www"
)

//go:embed www/*
var content embed.FS

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
		UIPath: UI_PATH,
	})
	if err != nil {
		//Terminate or enter standalone mode here
		panic(err)
	}

	// Register the shutdown handler
	plugin.RegisterShutdownHandler(func() {
		// Do cleanup here if needed
		fmt.Println("Hello World Plugin Exited")
	})

	embedWebRouter := plugin.NewPluginEmbedUIRouter(PLUGIN_ID, &content, WEB_ROOT, UI_PATH)

	// Serve the hello world page in the www folder
	http.Handle(UI_PATH, embedWebRouter.Handler())
	fmt.Println("Hello World started at http://127.0.0.1:" + strconv.Itoa(runtimeCfg.Port))
	http.ListenAndServe("127.0.0.1:"+strconv.Itoa(runtimeCfg.Port), nil)
}
