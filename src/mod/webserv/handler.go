package webserv

import (
	"encoding/json"
	"net/http"
	"strconv"

	"imuslab.com/zoraxy/mod/utils"
)

/*
	Handler.go

	Handler for web server options change
	web server is directly listening to the TCP port
	handlers in this script are for setting change only
*/

type StaticWebServerStatus struct {
	ListeningPort               int
	EnableDirectoryListing      bool
	WebRoot                     string
	Running                     bool
	EnableWebDirManager         bool
	DisableListenToAllInterface bool
}

// Handle getting current static web server status
func (ws *WebServer) HandleGetStatus(w http.ResponseWriter, r *http.Request) {
	listeningPortInt, _ := strconv.Atoi(ws.option.Port)
	currentStatus := StaticWebServerStatus{
		ListeningPort:               listeningPortInt,
		EnableDirectoryListing:      ws.option.EnableDirectoryListing,
		WebRoot:                     ws.option.WebRoot,
		Running:                     ws.isRunning,
		EnableWebDirManager:         ws.option.EnableWebDirManager,
		DisableListenToAllInterface: ws.option.DisableListenToAllInterface,
	}

	js, _ := json.Marshal(currentStatus)
	utils.SendJSONResponse(w, string(js))
}

// Handle request for starting the static web server
func (ws *WebServer) HandleStartServer(w http.ResponseWriter, r *http.Request) {
	err := ws.Start()
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	utils.SendOK(w)
}

// Handle request for stopping the static web server
func (ws *WebServer) HandleStopServer(w http.ResponseWriter, r *http.Request) {
	err := ws.Stop()
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	utils.SendOK(w)
}

// Handle change server listening port request
func (ws *WebServer) HandlePortChange(w http.ResponseWriter, r *http.Request) {
	newPort, err := utils.PostInt(r, "port")
	if err != nil {
		utils.SendErrorResponse(w, "invalid port number given")
		return
	}

	err = ws.ChangePort(strconv.Itoa(newPort))
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

// Change enable directory listing settings
func (ws *WebServer) SetEnableDirectoryListing(w http.ResponseWriter, r *http.Request) {
	enableList, err := utils.PostBool(r, "enable")
	if err != nil {
		utils.SendErrorResponse(w, "invalid setting given")
		return
	}
	err = ws.option.Sysdb.Write("webserv", "dirlist", enableList)
	if err != nil {
		utils.SendErrorResponse(w, "unable to save setting")
		return
	}
	ws.option.EnableDirectoryListing = enableList
	utils.SendOK(w)
}

// Get or set disable listen to all interface settings
func (ws *WebServer) SetDisableListenToAllInterface(w http.ResponseWriter, r *http.Request) {
	disableListen, err := utils.PostBool(r, "disable")
	if err != nil {
		utils.SendErrorResponse(w, "invalid setting given")
		return
	}
	err = ws.option.Sysdb.Write("webserv", "disableListenToAllInterface", disableListen)
	if err != nil {
		utils.SendErrorResponse(w, "unable to save setting")
		return
	}
	ws.option.DisableListenToAllInterface = disableListen
	utils.SendOK(w)
}
