package tcpprox

import (
	"encoding/json"
	"net/http"
	"strconv"

	"imuslab.com/zoraxy/mod/utils"
)

/*
	Handler.go
	Handlers for the tcprox. Remove this file
	if your application do not need any http
	handler.
*/

func (m *Manager) HandleAddProxyConfig(w http.ResponseWriter, r *http.Request) {
	name, err := utils.PostPara(r, "name")
	if err != nil {
		utils.SendErrorResponse(w, "name cannot be empty")
		return
	}

	portA, err := utils.PostPara(r, "porta")
	if err != nil {
		utils.SendErrorResponse(w, "first address cannot be empty")
		return
	}

	portB, err := utils.PostPara(r, "portb")
	if err != nil {
		utils.SendErrorResponse(w, "second address cannot be empty")
		return
	}

	timeoutStr, _ := utils.PostPara(r, "timeout")
	timeout := m.Options.DefaultTimeout
	if timeoutStr != "" {
		timeout, err = strconv.Atoi(timeoutStr)
		if err != nil {
			utils.SendErrorResponse(w, "invalid timeout value: "+timeoutStr)
			return
		}
	}

	modeValue := ProxyMode_Transport
	mode, err := utils.PostPara(r, "mode")
	if err != nil || mode == "" {
		utils.SendErrorResponse(w, "no mode given")
	} else if mode == "listen" {
		modeValue = ProxyMode_Listen
	} else if mode == "transport" {
		modeValue = ProxyMode_Transport
	} else if mode == "starter" {
		modeValue = ProxyMode_Starter
	} else {
		utils.SendErrorResponse(w, "invalid mode given. Only support listen / transport / starter")
	}

	//Create the target config
	newConfigUUID := m.NewConfig(&ProxyRelayOptions{
		Name:    name,
		PortA:   portA,
		PortB:   portB,
		Timeout: timeout,
		Mode:    modeValue,
	})

	js, _ := json.Marshal(newConfigUUID)
	utils.SendJSONResponse(w, string(js))
}

func (m *Manager) HandleEditProxyConfigs(w http.ResponseWriter, r *http.Request) {
	// Extract POST parameters using utils.PostPara
	configUUID, err := utils.PostPara(r, "uuid")
	if err != nil {
		utils.SendErrorResponse(w, "config UUID cannot be empty")
		return
	}

	newName, _ := utils.PostPara(r, "name")
	newPortA, _ := utils.PostPara(r, "porta")
	newPortB, _ := utils.PostPara(r, "portb")
	newModeStr, _ := utils.PostPara(r, "mode")
	newMode := -1
	if newModeStr != "" {
		if newModeStr == "listen" {
			newMode = 0
		} else if newModeStr == "transport" {
			newMode = 1
		} else if newModeStr == "starter" {
			newMode = 2
		} else {
			utils.SendErrorResponse(w, "invalid new mode value")
			return
		}
	}

	newTimeoutStr, _ := utils.PostPara(r, "timeout")
	newTimeout := -1
	if newTimeoutStr != "" {
		newTimeout, err = strconv.Atoi(newTimeoutStr)
		if err != nil {
			utils.SendErrorResponse(w, "invalid newTimeout value: "+newTimeoutStr)
			return
		}
	}

	// Call the EditConfig method to modify the configuration
	err = m.EditConfig(configUUID, newName, newPortA, newPortB, newMode, newTimeout)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

func (m *Manager) HandleListConfigs(w http.ResponseWriter, r *http.Request) {
	js, _ := json.Marshal(m.Configs)
	utils.SendJSONResponse(w, string(js))
}

func (m *Manager) HandleStartProxy(w http.ResponseWriter, r *http.Request) {
	uuid, err := utils.PostPara(r, "uuid")
	if err != nil {
		utils.SendErrorResponse(w, "invalid uuid given")
		return
	}

	targetProxyConfig, err := m.GetConfigByUUID(uuid)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	err = targetProxyConfig.Start()
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

func (m *Manager) HandleStopProxy(w http.ResponseWriter, r *http.Request) {
	uuid, err := utils.PostPara(r, "uuid")
	if err != nil {
		utils.SendErrorResponse(w, "invalid uuid given")
		return
	}

	targetProxyConfig, err := m.GetConfigByUUID(uuid)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	if !targetProxyConfig.IsRunning() {
		utils.SendErrorResponse(w, "target proxy service is not running")
		return
	}

	targetProxyConfig.Stop()
	utils.SendOK(w)
}

func (m *Manager) HandleRemoveProxy(w http.ResponseWriter, r *http.Request) {
	uuid, err := utils.PostPara(r, "uuid")
	if err != nil {
		utils.SendErrorResponse(w, "invalid uuid given")
		return
	}

	targetProxyConfig, err := m.GetConfigByUUID(uuid)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	if targetProxyConfig.IsRunning() {
		utils.SendErrorResponse(w, "Service is running")
		return
	}

	err = m.RemoveConfig(targetProxyConfig.UUID)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

func (m *Manager) HandleGetProxyStatus(w http.ResponseWriter, r *http.Request) {
	uuid, err := utils.GetPara(r, "uuid")
	if err != nil {
		utils.SendErrorResponse(w, "invalid uuid given")
		return
	}

	targetConfig, err := m.GetConfigByUUID(uuid)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	js, _ := json.Marshal(targetConfig)
	utils.SendJSONResponse(w, string(js))
}

func (m *Manager) HandleConfigValidate(w http.ResponseWriter, r *http.Request) {
	uuid, err := utils.GetPara(r, "uuid")
	if err != nil {
		utils.SendErrorResponse(w, "invalid uuid given")
		return
	}

	targetConfig, err := m.GetConfigByUUID(uuid)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	err = targetConfig.ValidateConfigs()
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}
