package streamproxy

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

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

	listenAddr, err := utils.PostPara(r, "listenAddr")
	if err != nil {
		utils.SendErrorResponse(w, "first address cannot be empty")
		return
	}

	proxyAddr, err := utils.PostPara(r, "proxyAddr")
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

	useTCP, _ := utils.PostBool(r, "useTCP")
	useUDP, _ := utils.PostBool(r, "useUDP")
	ProxyProtocolVersion, _ := utils.PostInt(r, "proxyProtocolVersion")
	enableLogging, _ := utils.PostBool(r, "enableLogging")

	//Create the target config
	newConfigUUID := m.NewConfig(&ProxyRelayOptions{
		Name:                 name,
		ListeningAddr:        strings.TrimSpace(listenAddr),
		ProxyAddr:            strings.TrimSpace(proxyAddr),
		Timeout:              timeout,
		UseTCP:               useTCP,
		UseUDP:               useUDP,
		ProxyProtocolVersion: convertIntToProxyProtocolVersion(ProxyProtocolVersion),
		EnableLogging:        enableLogging,
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
	listenAddr, _ := utils.PostPara(r, "listenAddr")
	proxyAddr, _ := utils.PostPara(r, "proxyAddr")
	useTCP, _ := utils.PostBool(r, "useTCP")
	useUDP, _ := utils.PostBool(r, "useUDP")
	proxyProtocolVersion, _ := utils.PostInt(r, "proxyProtocolVersion")
	enableLogging, _ := utils.PostBool(r, "enableLogging")

	newTimeoutStr, _ := utils.PostPara(r, "timeout")
	newTimeout := -1
	if newTimeoutStr != "" {
		newTimeout, err = strconv.Atoi(newTimeoutStr)
		if err != nil {
			utils.SendErrorResponse(w, "invalid newTimeout value: "+newTimeoutStr)
			return
		}
	}

	// Create a new ProxyRuleUpdateConfig with the extracted parameters
	newConfig := &ProxyRuleUpdateConfig{
		InstanceUUID:         configUUID,
		NewName:              newName,
		NewListeningAddr:     listenAddr,
		NewProxyAddr:         proxyAddr,
		UseTCP:               useTCP,
		UseUDP:               useUDP,
		ProxyProtocolVersion: proxyProtocolVersion,
		EnableLogging:        enableLogging,
		NewTimeout:           newTimeout,
	}

	// Call the EditConfig method to modify the configuration
	err = m.EditConfig(newConfig)
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
		targetProxyConfig.Running = false
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
		targetProxyConfig.Running = false
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
