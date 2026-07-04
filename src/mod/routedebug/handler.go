package routedebug

import (
	"encoding/json"
	"net/http"
	"strconv"

	"imuslab.com/zoraxy/mod/utils"
)

/*
	handler.go

	HTTP handlers for the Route Debugger management API.
	These are wired into the Zoraxy auth-gated router in api.go.
*/

// RouteDebuggerStatus is the JSON payload returned by HandleGetStatus.
type RouteDebuggerStatus struct {
	Running       bool
	ListeningPort int
	PrettyPrint   bool
}

// HandleGetStatus returns the current state of the route debugger as JSON.
func (rd *RouteDebugger) HandleGetStatus(w http.ResponseWriter, r *http.Request) {
	port, _ := strconv.Atoi(rd.option.Port)
	status := RouteDebuggerStatus{
		Running:       rd.isRunning,
		ListeningPort: port,
		PrettyPrint:   rd.option.PrettyPrint,
	}
	js, _ := json.Marshal(status)
	utils.SendJSONResponse(w, string(js))
}

// HandleStart starts the route debugger server.
func (rd *RouteDebugger) HandleStart(w http.ResponseWriter, r *http.Request) {
	if err := rd.Start(); err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	utils.SendOK(w)
}

// HandleStop stops the route debugger server.
func (rd *RouteDebugger) HandleStop(w http.ResponseWriter, r *http.Request) {
	if err := rd.Stop(); err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	utils.SendOK(w)
}

// HandlePortChange changes the listening port (POST param: "port").
func (rd *RouteDebugger) HandlePortChange(w http.ResponseWriter, r *http.Request) {
	newPort, err := utils.PostInt(r, "port")
	if err != nil || newPort < 1 || newPort > 65535 {
		utils.SendErrorResponse(w, "invalid port number")
		return
	}
	if err := rd.ChangePort(strconv.Itoa(newPort)); err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	utils.SendOK(w)
}

// HandleSetPrettyPrint toggles HTML vs. plain-text output (POST param: "enable").
func (rd *RouteDebugger) HandleSetPrettyPrint(w http.ResponseWriter, r *http.Request) {
	enable, err := utils.PostBool(r, "enable")
	if err != nil {
		utils.SendErrorResponse(w, "invalid value")
		return
	}
	rd.SetPrettyPrint(enable)
	utils.SendOK(w)
}
