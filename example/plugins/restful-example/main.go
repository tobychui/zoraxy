package main

import (
	"embed"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	plugin "example.com/zoraxy/restful-example/mod/zoraxy_plugin"
)

const (
	PLUGIN_ID = "com.example.restful-example"
	UI_PATH   = "/"
	WEB_ROOT  = "/www"
)

//go:embed www/*
var content embed.FS

func main() {
	// Serve the plugin intro spect
	// This will print the plugin intro spect and exit if the -introspect flag is provided
	runtimeCfg, err := plugin.ServeAndRecvSpec(&plugin.IntroSpect{
		ID:            "com.example.restful-example",
		Name:          "Restful Example",
		Author:        "foobar",
		AuthorContact: "admin@example.com",
		Description:   "A simple demo for making RESTful API calls in plugin",
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

	// Create a new PluginEmbedUIRouter that will serve the UI from web folder
	// The router will also help to handle the termination of the plugin when
	// a user wants to stop the plugin via Zoraxy Web UI
	embedWebRouter := plugin.NewPluginEmbedUIRouter(PLUGIN_ID, &content, WEB_ROOT, UI_PATH)
	embedWebRouter.RegisterTerminateHandler(func() {
		// Do cleanup here if needed
		fmt.Println("Restful-example Exited")
	}, nil)

	//Register a simple API endpoint that will echo the request body
	// Since we are using the default http.ServeMux, we can register the handler directly with the last
	// parameter as nil
	embedWebRouter.HandleFunc("/api/echo", func(w http.ResponseWriter, r *http.Request) {
		// This is a simple echo API that will return the request body as response
		name := r.URL.Query().Get("name")
		if name == "" {
			http.Error(w, "Missing 'name' query parameter", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		response := map[string]string{"message": fmt.Sprintf("Hello %s", name)}
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		}
	}, nil)

	// Here is another example of a POST API endpoint that will echo the form data
	// This will handle POST requests to /api/post and return the form data as response
	embedWebRouter.HandleFunc("/api/post", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "Failed to parse form data", http.StatusBadRequest)
			return
		}

		for key, values := range r.PostForm {
			for _, value := range values {
				// Generate a simple HTML response
				w.Header().Set("Content-Type", "text/html")
				fmt.Fprintf(w, "%s: %s<br>", key, value)
			}
		}
	}, nil)

	// Serve the restful-example page in the www folder
	http.Handle(UI_PATH, embedWebRouter.Handler())
	fmt.Println("Restful-example started at http://127.0.0.1:" + strconv.Itoa(runtimeCfg.Port))
	err = http.ListenAndServe("127.0.0.1:"+strconv.Itoa(runtimeCfg.Port), nil)
	if err != nil {
		panic(err)
	}

}
