package main

import (
	"encoding/json"
	"net/http"
	"time"

	plugin "aroz.org/zoraxy/event-subscriber-example/mod/zoraxy_plugin"
)

func RenderUI(config *plugin.ConfigureSpec, w http.ResponseWriter, r *http.Request) {
	// Render the UI for the plugin
	var eventLogHTML string
	if len(EventLog) == 0 {
		eventLogHTML = "<p>No events received yet<br>Try toggling a blacklist or something like that</p>"
	} else {
		EventLogMutex.Lock()
		defer EventLogMutex.Unlock()
		for _, event := range EventLog {
			rawEventData, _ := json.Marshal(event)

			eventLogHTML += "<div class='response-block'>"
			eventLogHTML += "<h3>" + string(event.Name) + " at " + time.Unix(event.Timestamp, 0).Local().Format(time.RFC3339) + "</h3>"
			eventLogHTML += "<div class='response-content'>"
			eventLogHTML += "<p class='ui meta'>Event Data:</p>"
			eventLogHTML += "<pre>" + string(rawEventData) + "</pre>"
			eventLogHTML += "</div></div>"
		}

	}

	w.Header().Set("Content-Type", "text/html")
	w.WriteHeader(http.StatusOK)
	html := `
	<!DOCTYPE html>
	<html>
	<head>
		<title>Event Log</title>
		<meta charset="UTF-8">
		<link rel="stylesheet" href="/script/semantic/semantic.min.css">
		<script src="/script/jquery-3.6.0.min.js"></script>
		<script src="/script/semantic/semantic.min.js"></script>
    	<meta name="viewport" content="width=device-width, initial-scale=1.0">
		<link rel="stylesheet" href="/main.css">
		<style>
			body {
				background: none;
			}

			.response-block {
				background-color: var(--theme_bg_primary);
				border: 1px solid var(--theme_divider);
				border-radius: 8px;
				padding: 20px;
				margin: 5px 0;
				box-shadow: 0 2px 4px rgba(0,0,0,0.1);
				transition: box-shadow 0.3s ease;
			}
			.response-block:hover {
				box-shadow: 0 4px 8px rgba(0,0,0,0.15);
			}
			.response-block h3 {
				margin-top: 0;
				color: var(--text_color);
				border-bottom: 2px solid #007bff;
				padding-bottom: 8px;
			}
			
			.response-content {
				margin-top: 10px;
			}
			.response-content pre {
				background-color: var(--theme_highlight);
				border: 1px solid var(--theme_divider);
				border-radius: 4px;
				padding: 10px;
				overflow: auto;
				font-size: 12px;
				line-height: 1.4;
				height: fit-content;
				max-height: 80vh;
				resize: vertical;
				box-sizing: border-box;
			}
		</style>
	</head>
	<body>
	<!-- Dark theme script must be included after body tag-->
	<link rel="stylesheet" href="/darktheme.css">
	<script src="/script/darktheme.js"></script>
	<div class="ui container">

		<h1>Event Log</h1>
		<div id="event-log" class="ui basic segment">` + eventLogHTML + `</div>
	</body>
	</html>
	`
	w.Write([]byte(html))
}
