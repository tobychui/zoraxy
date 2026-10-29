package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	plugin "example.com/zoraxy/request-inspect-example/mod/zoraxy_plugin"
)

func inspect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var payload plugin.RequestInspectPayload
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2*1024*1024)).Decode(&payload); err != nil {
		http.Error(w, "Invalid inspection payload", http.StatusBadRequest)
		return
	}
	u, err := url.ParseRequestURI(payload.RequestURI)
	if err != nil {
		http.Error(w, "Invalid request URI", http.StatusBadRequest)
		return
	}
	result := plugin.RequestInspectResult{Action: "allow"}
	if u.Query().Get("block") == "1" {
		result = plugin.RequestInspectResult{Action: "deny", Status: http.StatusForbidden, Reason: "Blocked by request-inspect-example"}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func main() {
	cfg, err := plugin.ServeAndRecvSpec(&plugin.IntroSpect{
		ID: "org.aroz.zoraxy.request-inspect-example", Name: "Request Inspection Example",
		Author: "Zoraxy contributors", Description: "Opt-in inspection: block query block=1; otherwise continue normal proxy routing.",
		Type: plugin.PluginType_Router, VersionMajor: 1,
		RequestInspect: "/inspect", RequestInspectMaxBody: 64, UIPath: "/ui",
	})
	if err != nil {
		log.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/inspect", inspect)
	mux.HandleFunc("/ui/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Inspection example: ?block=1 is denied.")
	})
	server := &http.Server{Addr: "127.0.0.1:" + strconv.Itoa(cfg.Port), Handler: mux, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second}
	log.Fatal(server.ListenAndServe())
}
