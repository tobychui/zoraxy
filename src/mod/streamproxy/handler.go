package streamproxy

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"imuslab.com/zoraxy/mod/utils"
)

type proxyConfigResponse struct {
	*ProxyRelayInstance
	EffectiveRunning bool                `json:"effective_running"`
	ManagedByNode    bool                `json:"managed_by_node"`
	CanStartLocally  bool                `json:"can_start_locally"`
	RemoteStatus     *RemoteRuntimeState `json:"remote_status,omitempty"`
}

func (m *Manager) buildProxyConfigResponse(config *ProxyRelayInstance) *proxyConfigResponse {
	if config == nil {
		return nil
	}

	clonedConfig, err := CloneConfig(config)
	if err != nil || clonedConfig == nil {
		cloned := *config
		cloned.tcpStopChan = nil
		cloned.udpStopChan = nil
		cloned.parent = nil
		clonedConfig = &cloned
	}

	managedByNode := !m.isLocallyAssigned(config.AssignedNodeID)
	effectiveRunning := config.IsRunning() || config.Running
	response := &proxyConfigResponse{
		ProxyRelayInstance: clonedConfig,
		EffectiveRunning:   effectiveRunning,
		ManagedByNode:      managedByNode,
		CanStartLocally:    !managedByNode,
	}

	if managedByNode {
		response.ProxyRelayInstance.Running = false
		response.EffectiveRunning = false
		response.RemoteStatus = m.getRemoteRuntimeState(config)
		if response.RemoteStatus != nil && response.RemoteStatus.Online {
			response.ProxyRelayInstance.Running = response.RemoteStatus.Running
			response.EffectiveRunning = response.RemoteStatus.Running
		}
	}

	return response
}

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
	assignedNodeID, _ := utils.PostPara(r, "assignedNodeId")

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
		AssignedNodeID:       assignedNodeID,
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
	assignedNodeID, _ := utils.PostPara(r, "assignedNodeId")

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
		AssignedNodeID:       assignedNodeID,
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
	results := make([]*proxyConfigResponse, 0, len(m.Configs))
	for _, config := range m.Configs {
		results = append(results, m.buildProxyConfigResponse(config))
	}

	js, _ := json.Marshal(results)
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
	if !m.isLocallyAssigned(targetProxyConfig.AssignedNodeID) {
		utils.SendErrorResponse(w, "target proxy is assigned to another node")
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
	if !m.isLocallyAssigned(targetProxyConfig.AssignedNodeID) {
		utils.SendErrorResponse(w, "target proxy is assigned to another node")
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

	js, _ := json.Marshal(m.buildProxyConfigResponse(targetConfig))
	utils.SendJSONResponse(w, string(js))
}
