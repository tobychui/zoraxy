package main

import (
	"bytes"
	"fmt"
	"net/http"
	"sync"

	plugin "aroz.org/zoraxy/event-subscriber-example/mod/zoraxy_plugin"
)

const (
	PLUGIN_ID  = "org.aroz.zoraxy.event_subscriber_example"
	UI_PATH    = "/ui"
	EVENT_PATH = "/notifyme/"
)

var (
	EventLog      = make([]plugin.Event, 0) // A slice to store events
	EventLogMutex = &sync.Mutex{}           // Mutex to protect access to the event log
)

func main() {
	// Serve the plugin intro spect
	// This will print the plugin intro spect and exit if the -introspect flag is provided
	runtimeCfg, err := plugin.ServeAndRecvSpec(&plugin.IntroSpect{
		ID:            PLUGIN_ID,
		Name:          "Event Subscriber Example Plugin",
		Author:        "Anthony Rubick",
		AuthorContact: "",
		Description:   "An example plugin for event subscriptions, will display all events in the UI",
		Type:          plugin.PluginType_Utilities,
		VersionMajor:  1,
		VersionMinor:  0,
		VersionPatch:  0,

		UIPath: UI_PATH,

		/* Subscriptions Settings */
		SubscriptionPath: "/notifyme",
		SubscriptionsEvents: map[plugin.EventName]string{
			// for this example, we will subscribe to all events that exist at time of writing
			plugin.EventBlacklistedIPBlocked: "This event is triggered when a blacklisted IP is blocked",
			plugin.EventBlacklistToggled:     "This event is triggered when the blacklist is toggled for an access rule",
			plugin.EventAccessRuleCreated:    "This event is triggered when a new access ruleset is created",
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
	http.HandleFunc(EVENT_PATH, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var event plugin.Event

			// read the request body
			if r.Body == nil || r.ContentLength == 0 {
				http.Error(w, "Request body is empty", http.StatusBadRequest)
				return
			}
			defer r.Body.Close()

			buffer := bytes.NewBuffer(make([]byte, 0, r.ContentLength))
			if _, err := buffer.ReadFrom(r.Body); err != nil {
				http.Error(w, fmt.Sprintf("Failed to read request body: %v", err), http.StatusBadRequest)
				return
			}

			// parse the event from the request body
			if err := plugin.ParseEvent(buffer.Bytes(), &event); err != nil {
				http.Error(w, fmt.Sprintf("Failed to parse event: %v", err), http.StatusBadRequest)
				return
			}

			// Typically, at this point you would use a switch statement on the event.Name
			// to route the event to the appropriate handler.
			//
			// For this example, we will just store the event and return a success message.
			EventLogMutex.Lock()
			defer EventLogMutex.Unlock()
			if len(EventLog) >= 100 { // Limit the log size to 100 events
				EventLog = EventLog[1:] // Remove the oldest event
			}
			EventLog = append(EventLog, event) // Store the event in the log

			w.WriteHeader(http.StatusOK)
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprintf(w, "Event received: %s", event.Name)

		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	serverAddr := fmt.Sprintf("127.0.0.1:%d", runtimeCfg.Port)
	fmt.Printf("Starting API Call Example Plugin on %s\n", serverAddr)
	http.ListenAndServe(serverAddr, nil)
}
