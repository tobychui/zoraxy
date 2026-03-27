package node

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"imuslab.com/zoraxy/mod/utils"
)

func getNodeDisplayName(targetNode *Node) string {
	if targetNode == nil {
		return ""
	}

	if strings.TrimSpace(targetNode.Name) != "" {
		return strings.TrimSpace(targetNode.Name)
	}
	if strings.TrimSpace(targetNode.Host) != "" {
		return strings.TrimSpace(targetNode.Host)
	}

	return targetNode.ID
}

func getNodeAdminURL(targetNode *Node) string {
	if targetNode == nil {
		return ""
	}

	host := strings.TrimSpace(targetNode.LastIP)
	if host == "" {
		return ""
	}

	port := strings.TrimSpace(targetNode.ManagementPort)
	if port == "" {
		port = "8000"
	}

	return "http://" + net.JoinHostPort(host, port)
}

func getRemoteNodeIP(r *http.Request) string {
	if r == nil {
		return ""
	}

	remoteIP := strings.TrimSpace(r.RemoteAddr)
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		remoteIP = strings.TrimSpace(strings.Split(forwarded, ",")[0])
	}

	if host, _, err := net.SplitHostPort(remoteIP); err == nil {
		return host
	}

	return remoteIP
}

// Return a list of domains where the certificates covers
func (m *Manager) HandleListNodes(w http.ResponseWriter, r *http.Request) {
	registeredNodes, err := m.ListRegisteredNodes()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type NodeInfo struct {
		ID                  string `json:"id"`
		Name                string `json:"name"`
		DisplayName         string `json:"display_name"`
		Host                string `json:"host"`
		LastIP              string `json:"last_ip,omitempty"`
		ManagementPort      string `json:"management_port,omitempty"`
		AdminURL            string `json:"admin_url,omitempty"`
		Enabled             bool   `json:"enabled"`
		LocalOverride       bool   `json:"local_override"`
		Online              bool   `json:"online"`
		Status              string `json:"status"`
		Token               string `json:"token"`
		RegisteredAt        string `json:"registered_at" `
		LastSeen            string `json:"last_seen"`
		ZoraxyVersion       string `json:"zoraxy_version"`
		ConfigVersion       string `json:"config_version"`
		PrimaryVersion      string `json:"primary_version"`
		RequireVersionMatch bool   `json:"require_version_match"`
		VersionMismatch     bool   `json:"version_mismatch"`
		VersionMessage      string `json:"version_mismatch_message,omitempty"`
	}

	results := make([]*NodeInfo, 0)
	for _, Node := range registeredNodes {
		lastSeen := ""
		if !Node.LastSeen.IsZero() {
			lastSeen = Node.LastSeen.Format("2006-01-02 15:04:05")
		}
		online := m.IsNodeOnline(Node)
		status := "offline"
		if online && Node.LocalOverride {
			status = "local_override"
		} else if online {
			status = "online"
		}
		versionMismatch, versionMessage := m.GetNodeVersionMismatch(Node)
		results = append(results, &NodeInfo{
			ID:                  Node.ID,
			Name:                Node.Name,
			DisplayName:         getNodeDisplayName(Node),
			Host:                Node.Host,
			LastIP:              Node.LastIP,
			ManagementPort:      Node.ManagementPort,
			AdminURL:            getNodeAdminURL(Node),
			Enabled:             Node.Enabled,
			LocalOverride:       Node.LocalOverride,
			Online:              online,
			Status:              status,
			Token:               Node.Token,
			RegisteredAt:        Node.RegisteredAt.Format("2006-01-02 15:04:05"),
			LastSeen:            lastSeen,
			ZoraxyVersion:       Node.ZoraxyVersion,
			ConfigVersion:       Node.ConfigVersion,
			PrimaryVersion:      m.GetPrimaryVersion(),
			RequireVersionMatch: m.IsVersionMatchRequired(),
			VersionMismatch:     versionMismatch,
			VersionMessage:      versionMessage,
		})
	}
	response, err := json.Marshal(results)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(response)
}

func (m *Manager) HandleRegisterNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	registeredNode, err := m.GenerateJoinRequest(strings.TrimSpace(r.PostForm.Get("name")))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response, err := json.Marshal(registeredNode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	utils.SendJSONResponse(w, string(response))
}

func (m *Manager) HandleGetNodeInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, err := utils.GetPara(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	targetNode, err := m.GetNodeByID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	telemetry, err := m.LoadNodeTelemetry(targetNode.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	online := m.IsNodeOnline(targetNode)
	status := "offline"
	if online && targetNode.LocalOverride {
		status = "local_override"
	} else if online {
		status = "online"
	}
	versionMismatch, versionMessage := m.GetNodeVersionMismatch(targetNode)

	response, err := json.Marshal(struct {
		*Node
		DisplayName          string             `json:"display_name"`
		AdminURL             string             `json:"admin_url,omitempty"`
		Online               bool               `json:"online"`
		Status               string             `json:"status"`
		LocalOverride        bool               `json:"local_override"`
		PrimaryVersion       string             `json:"primary_version"`
		RequireVersionMatch  bool               `json:"require_version_match"`
		VersionMismatch      bool               `json:"version_mismatch"`
		VersionMismatchError string             `json:"version_mismatch_message,omitempty"`
		Telemetry            *TelemetrySnapshot `json:"telemetry,omitempty"`
		TelemetryOverview    *TelemetryOverview `json:"telemetry_overview,omitempty"`
	}{
		Node:                 targetNode,
		DisplayName:          getNodeDisplayName(targetNode),
		AdminURL:             getNodeAdminURL(targetNode),
		Online:               online,
		Status:               status,
		LocalOverride:        targetNode.LocalOverride,
		PrimaryVersion:       m.GetPrimaryVersion(),
		RequireVersionMatch:  m.IsVersionMatchRequired(),
		VersionMismatch:      versionMismatch,
		VersionMismatchError: versionMessage,
		Telemetry:            telemetry,
		TelemetryOverview:    BuildTelemetryOverview(telemetry),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	utils.SendJSONResponse(w, string(response))
}

func (m *Manager) HandleUnregisterNode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr, err := utils.PostPara(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, fmt.Errorf("failed to parse token: %v", err).Error(), http.StatusBadRequest)
		return
	}

	Node, err := m.GetNodeByID(id.String())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := m.UnregisterNode(Node); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	utils.SendOK(w)
}

func (m *Manager) HandleRotateNodeToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr, err := utils.PostPara(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, fmt.Errorf("failed to parse token: %v", err).Error(), http.StatusBadRequest)
		return
	}

	targetNode, err := m.GetNodeByID(id.String())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := m.RotateNodeToken(targetNode); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response, err := json.Marshal(targetNode)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	utils.SendJSONResponse(w, string(response))
}

func (m *Manager) HandleSetNodeEnabled(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr, err := utils.PostPara(r, "id")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	enabled, err := utils.PostBool(r, "enabled")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	id, err := uuid.Parse(idStr)
	if err != nil {
		http.Error(w, fmt.Errorf("failed to parse token: %v", err).Error(), http.StatusBadRequest)
		return
	}

	targetNode, err := m.GetNodeByID(id.String())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := m.SetNodeEnabled(targetNode, enabled); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response, err := json.Marshal(struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}{
		ID:      targetNode.ID,
		Enabled: targetNode.Enabled,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	utils.SendJSONResponse(w, string(response))
}

func (m *Manager) HandleNodeUpdate(node *Node, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	hostname, err := utils.PostPara(r, "hostname")
	if err == nil {
		node.Host = hostname
	}

	nodeName, err := utils.PostPara(r, "node_name")
	if err == nil {
		node.Name = strings.TrimSpace(nodeName)
	}

	managementPort, err := utils.PostPara(r, "management_port")
	if err == nil {
		node.ManagementPort = strings.TrimSpace(managementPort)
	}

	nodeIP, err := utils.PostPara(r, "node_ip")
	if err == nil && strings.TrimSpace(nodeIP) != "" {
		node.LastIP = strings.TrimSpace(nodeIP)
	} else {
		node.LastIP = getRemoteNodeIP(r)
	}

	zoraxyVersion, err := utils.PostPara(r, "zoraxy_version")
	if err == nil {
		node.ZoraxyVersion = zoraxyVersion
	}

	configVersion, err := utils.PostPara(r, "config_version")
	if err == nil {
		node.ConfigVersion = configVersion
	}

	localOverride, err := utils.PostBool(r, "local_override")
	if err == nil {
		node.LocalOverride = localOverride
	}

	streamProxyRuntimeRaw, streamRuntimeErr := utils.PostPara(r, "stream_proxy_runtime")
	if streamRuntimeErr == nil {
		streamRuntime := map[string]*StreamProxyRuntime{}
		if strings.TrimSpace(streamProxyRuntimeRaw) != "" {
			if err := json.Unmarshal([]byte(streamProxyRuntimeRaw), &streamRuntime); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		if err := m.UpdateNodeStreamProxyRuntime(node.ID, streamRuntime); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if err = m.UpdateNodeLastSeen(node); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	utils.SendOK(w)
}

func (m *Manager) HandleGetTelemetrySummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	summary, err := m.BuildTelemetrySummary()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if summary == nil {
		summary = &TelemetrySummary{
			GeneratedAt: time.Now(),
		}
	}

	response, err := json.Marshal(summary)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	utils.SendJSONResponse(w, string(response))
}

func (m *Manager) HandleGetProxyConfigs(node *Node, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if m.Options == nil || (m.Options.ExportProxyConfigs == nil && m.Options.ExportProxyConfigsForNode == nil) {
		http.Error(w, "Proxy configuration export is not available", http.StatusServiceUnavailable)
		return
	}

	var snapshot *ProxyConfigSnapshot
	var err error
	if m.Options.ExportProxyConfigsForNode != nil {
		snapshot, err = m.Options.ExportProxyConfigsForNode(node)
	} else {
		snapshot, err = m.Options.ExportProxyConfigs()
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response, err := json.Marshal(snapshot)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	utils.SendJSONResponse(w, string(response))
}

func (m *Manager) HandleGetAccessRules(node *Node, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if m.Options == nil || (m.Options.ExportAccessRules == nil && m.Options.ExportAccessRulesForNode == nil) {
		http.Error(w, "Access rule export is not available", http.StatusServiceUnavailable)
		return
	}

	var snapshot *AccessSnapshot
	var err error
	if m.Options.ExportAccessRulesForNode != nil {
		snapshot, err = m.Options.ExportAccessRulesForNode(node)
	} else {
		snapshot, err = m.Options.ExportAccessRules()
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response, err := json.Marshal(snapshot)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	utils.SendJSONResponse(w, string(response))
}

func (m *Manager) HandleGetCertificates(node *Node, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if m.Options == nil || (m.Options.ExportCertificates == nil && m.Options.ExportCertificatesForNode == nil) {
		http.Error(w, "Certificate export is not available", http.StatusServiceUnavailable)
		return
	}

	var snapshot *CertificateSnapshot
	var err error
	if m.Options.ExportCertificatesForNode != nil {
		snapshot, err = m.Options.ExportCertificatesForNode(node)
	} else {
		snapshot, err = m.Options.ExportCertificates()
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response, err := json.Marshal(snapshot)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	utils.SendJSONResponse(w, string(response))
}

func (m *Manager) HandleGetSystemData(node *Node, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if m.Options == nil || (m.Options.ExportSystemData == nil && m.Options.ExportSystemDataForNode == nil) {
		http.Error(w, "System data export is not available", http.StatusServiceUnavailable)
		return
	}

	var snapshot *SystemSnapshot
	var err error
	if m.Options.ExportSystemDataForNode != nil {
		snapshot, err = m.Options.ExportSystemDataForNode(node)
	} else {
		snapshot, err = m.Options.ExportSystemData()
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response, err := json.Marshal(snapshot)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	utils.SendJSONResponse(w, string(response))
}

func (m *Manager) HandleNodeTelemetry(node *Node, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	telemetry := &TelemetrySnapshot{}
	if err := json.NewDecoder(r.Body).Decode(telemetry); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := m.MergeNodeTelemetry(node.ID, telemetry); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	utils.SendOK(w)
}
