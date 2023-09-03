package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"imuslab.com/zoraxy/mod/sshprox"
	"imuslab.com/zoraxy/mod/utils"
)

/*
	webssh.go

	This script handle the establish of a new ssh proxy object
*/

func HandleCreateProxySession(w http.ResponseWriter, r *http.Request) {
	//Get what ip address and port to connect to
	ipaddr, err := utils.PostPara(r, "ipaddr")
	if err != nil {
		http.Error(w, "Invalid Usage", http.StatusInternalServerError)
		return
	}

	portString, err := utils.PostPara(r, "port")
	if err != nil {
		portString = "22"
	}

	username, err := utils.PostPara(r, "username")
	if err != nil {
		username = ""
	}

	port, err := strconv.Atoi(portString)
	if err != nil {
		utils.SendErrorResponse(w, "invalid port number given")
		return
	}

	if !*allowSshLoopback {
		//Not allow loopback connections
		if strings.EqualFold(strings.TrimSpace(ipaddr), "localhost") || strings.TrimSpace(ipaddr) == "127.0.0.1" {
			//Request target is loopback
			utils.SendErrorResponse(w, "loopback web ssh connection is not enabled on this host")
			return
		}
	}

	//Check if the target is a valid ssh endpoint
	if !sshprox.IsSSHConnectable(ipaddr, port) {
		utils.SendErrorResponse(w, ipaddr+":"+strconv.Itoa(port)+" is not a valid SSH server")
		return
	}

	//Create a new proxy instance
	instance, err := webSshManager.NewSSHProxy("./tmp/gotty")
	if err != nil {
		utils.SendErrorResponse(w, strings.ReplaceAll(err.Error(), "\\", "/"))
		return
	}

	//Create an ssh process to the target address
	err = instance.CreateNewConnection(webSshManager.GetNextPort(), username, ipaddr, port)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Return the instance uuid
	js, _ := json.Marshal(instance.UUID)
	utils.SendJSONResponse(w, string(js))
}

// Check if the host support ssh, or if the target domain (and port, optional) support ssh
func HandleWebSshSupportCheck(w http.ResponseWriter, r *http.Request) {
	domain, err := utils.PostPara(r, "domain")
	if err != nil {
		//Check if ssh supported on this host
		isSupport := sshprox.IsWebSSHSupported()
		js, _ := json.Marshal(isSupport)
		utils.SendJSONResponse(w, string(js))
		return
	}
	//Domain is given. Check if port is given
	portString, err := utils.PostPara(r, "port")
	if err != nil {
		portString = "22"
	}

	port, err := strconv.Atoi(portString)
	if err != nil {
		utils.SendErrorResponse(w, "invalid port number given")
		return
	}

	if port < 1 || port > 65534 {
		utils.SendErrorResponse(w, "invalid port number given")
		return
	}

	looksLikeSSHServer := sshprox.IsSSHConnectable(domain, port)
	js, _ := json.Marshal(looksLikeSSHServer)
	utils.SendJSONResponse(w, string(js))
}
