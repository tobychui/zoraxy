package node

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/uptime"
	"imuslab.com/zoraxy/mod/utils"
)

type Node struct {
	ID             string    `json:"id"`
	Name           string    `json:"name,omitempty"`
	Host           string    `json:"host"`
	LastIP         string    `json:"last_ip,omitempty"`
	ManagementPort string    `json:"management_port,omitempty"`
	Enabled        bool      `json:"enabled"`
	LocalOverride  bool      `json:"local_override,omitempty"`
	Token          string    `json:"token"`
	RegisteredAt   time.Time `json:"registered_at" `
	LastSeen       time.Time `json:"last_seen"`
	ZoraxyVersion  string    `json:"zoraxy_version"`
	ConfigVersion  string    `json:"config_version"`
}

type Options struct {
	Logger         *logger.Logger //Logger for the stream proxy
	ConfigStore    string         //Folder to store the config files, will be created if not exists
	StatusFile     string
	UpdateInterval time.Duration
	RequestTimeout time.Duration
	Mode           string
	NodeServer     string
	NodeToken      string
	SystemVersion  string
	ManagementPort string

	HostnameProvider          func() (string, error)
	NodeIPProvider            func() string
	NodeNameProvider          func() (string, error)
	LocalOverrideProvider     func() bool
	GetConfigVersion          func() (string, error)
	GetStreamProxyRuntime     func() (map[string]*StreamProxyRuntime, error)
	GetTelemetrySnapshot      func() (*TelemetrySnapshot, error)
	ExportProxyConfigs        func() (*ProxyConfigSnapshot, error)
	ExportProxyConfigsForNode func(*Node) (*ProxyConfigSnapshot, error)
	ImportProxyConfigs        func(*ProxyConfigSnapshot) error
	ExportAccessRules         func() (*AccessSnapshot, error)
	ExportAccessRulesForNode  func(*Node) (*AccessSnapshot, error)
	ImportAccessRules         func(*AccessSnapshot) error
	ExportCertificates        func() (*CertificateSnapshot, error)
	ExportCertificatesForNode func(*Node) (*CertificateSnapshot, error)
	ImportCertificates        func(*CertificateSnapshot) error
	ExportSystemData          func() (*SystemSnapshot, error)
	ExportSystemDataForNode   func(*Node) (*SystemSnapshot, error)
	ImportSystemData          func(*SystemSnapshot) error
}

type Manager struct {
	Options              *Options
	Nodes                []*Node //A list of registered Node nodes
	sysdb                *database.Database
	updateTicket         *time.Ticker
	updateTicketStopChan chan bool
	client               *NodeClient
	syncMutex            sync.Mutex
	tickerMutex          sync.Mutex
	statusFile           string
	syncStatus           *SyncStatus
}

func NewManager(sysdb *database.Database, options *Options) (*Manager, error) {
	if !utils.FileExists(options.ConfigStore) {
		err := os.MkdirAll(options.ConfigStore, 0775)
		if err != nil {
			return nil, err
		}
	}

	//Load relay configs from db
	var nodes []*Node
	nodeConfigFiles, err := filepath.Glob(options.ConfigStore + "/*.config")
	if err != nil {
		return nil, err
	}

	for _, configFile := range nodeConfigFiles {
		//Read file into bytes
		configBytes, err := os.ReadFile(configFile)
		if err != nil {
			options.Logger.PrintAndLog("nodes", "Read node config failed", err)
			continue
		}
		thisRelayConfig := &Node{}
		err = json.Unmarshal(configBytes, thisRelayConfig)
		if err != nil {
			options.Logger.PrintAndLog("nodes", "Unmarshal node config failed", err)
			continue
		}
		if !bytes.Contains(configBytes, []byte(`"enabled"`)) {
			thisRelayConfig.Enabled = true
		}

		//Append the config to the list
		nodes = append(nodes, thisRelayConfig)
	}

	thisManager := &Manager{
		sysdb:   sysdb,
		Nodes:   nodes,
		Options: options,
	}

	thisManager.statusFile = options.StatusFile
	if thisManager.statusFile == "" {
		thisManager.statusFile = filepath.Join(options.ConfigStore, "sync.status.json")
	}

	syncStatus, err := LoadSyncStatus(thisManager.statusFile)
	if err != nil {
		return nil, err
	}
	syncStatus.Mode = options.Mode
	syncStatus.PrimaryServer = options.NodeServer
	thisManager.syncStatus = syncStatus

	if options.NodeServer != "" && options.NodeToken != "" {
		thisManager.client = NewNodeClient(options.NodeServer, options.NodeToken, options.RequestTimeout)
	}

	thisManager.SetUpdateInterval(options.UpdateInterval)

	return thisManager, nil
}

func (m *Manager) updateNodeConfig() {
	_ = m.syncNodeConfig()
}

func (m *Manager) MatchesLocalNode(assignedNodeID string) bool {
	return strings.TrimSpace(assignedNodeID) == ""
}

func (m *Manager) collectStreamProxyRuntime() map[string]*StreamProxyRuntime {
	if m == nil || m.Options == nil || m.Options.GetStreamProxyRuntime == nil {
		return map[string]*StreamProxyRuntime{}
	}

	runtimeSnapshot, err := m.Options.GetStreamProxyRuntime()
	if err != nil {
		m.logf("Failed to collect stream proxy runtime snapshot", err)
		return map[string]*StreamProxyRuntime{}
	}
	if runtimeSnapshot == nil {
		return map[string]*StreamProxyRuntime{}
	}

	return runtimeSnapshot
}

func (m *Manager) buildNodeStateUpdatePayload() (string, string, string, string, bool, map[string]*StreamProxyRuntime) {
	currentConfigVersion := ""
	if m.Options != nil && m.Options.GetConfigVersion != nil {
		configVersion, err := m.Options.GetConfigVersion()
		if err != nil {
			m.logf("Failed to calculate current config version", err)
		} else {
			currentConfigVersion = configVersion
		}
	}

	hostName, err := m.getHostname()
	if err != nil {
		m.logf("Failed to detect hostname for node update", err)
		hostName = ""
	}

	nodeName := hostName
	if m.Options != nil && m.Options.NodeNameProvider != nil {
		reportedNodeName, nameErr := m.Options.NodeNameProvider()
		if nameErr != nil {
			m.logf("Failed to detect node display name for node update", nameErr)
		} else if strings.TrimSpace(reportedNodeName) != "" {
			nodeName = strings.TrimSpace(reportedNodeName)
		}
	}

	return hostName, nodeName, m.getNodeIP(), currentConfigVersion, m.isLocalOverrideEnabled(), m.collectStreamProxyRuntime()
}

func (m *Manager) sendNodeStateUpdate(configVersion string) error {
	if m == nil || m.Options == nil || !strings.EqualFold(m.Options.Mode, "node") {
		return nil
	}

	if m.client == nil {
		if m.Options.NodeServer == "" || m.Options.NodeToken == "" {
			return nil
		}
		m.client = NewNodeClient(m.Options.NodeServer, m.Options.NodeToken, m.Options.RequestTimeout)
	}

	hostName, nodeName, nodeIP, currentConfigVersion, localOverride, streamRuntime := m.buildNodeStateUpdatePayload()
	if strings.TrimSpace(configVersion) != "" {
		currentConfigVersion = strings.TrimSpace(configVersion)
	}

	return m.client.UpdateNodeInfo("/node/api/update", hostName, nodeName, nodeIP, m.Options.ManagementPort, m.Options.SystemVersion, currentConfigVersion, localOverride, streamRuntime)
}

func (m *Manager) syncNodeConfig() error {
	if m == nil || m.Options == nil || !strings.EqualFold(m.Options.Mode, "node") {
		return nil
	}

	m.syncMutex.Lock()
	defer m.syncMutex.Unlock()

	if m.client == nil {
		if m.Options.NodeServer == "" || m.Options.NodeToken == "" {
			return nil
		}
		m.client = NewNodeClient(m.Options.NodeServer, m.Options.NodeToken, m.Options.RequestTimeout)
	}

	m.markSyncAttempt()
	_, _, _, currentConfigVersion, localOverride, _ := m.buildNodeStateUpdatePayload()
	if err := m.sendNodeStateUpdate(currentConfigVersion); err != nil {
		m.markSyncFailure(currentConfigVersion, err)
		return err
	}

	if localOverride {
		m.syncTelemetry()
		m.markSyncLocalOverride(currentConfigVersion)
		return nil
	}

	proxyConfigs, err := m.client.FetchProxyConfigs("/node/api/config")
	if err != nil {
		m.markSyncFailure(currentConfigVersion, err)
		return err
	}

	if proxyConfigs.RequireVersionMatch {
		primaryVersion := normalizeVersion(proxyConfigs.PrimaryVersion)
		nodeVersion := normalizeVersion(m.Options.SystemVersion)
		if primaryVersion != "" && nodeVersion != "" && primaryVersion != nodeVersion {
			err := fmt.Errorf("primary version %s does not match node version %s", strings.TrimSpace(proxyConfigs.PrimaryVersion), strings.TrimSpace(m.Options.SystemVersion))
			m.syncTelemetry()
			m.markSyncFailure(currentConfigVersion, err)
			return err
		}
	}

	if proxyConfigs.ConfigVersion == "" || proxyConfigs.ConfigVersion == currentConfigVersion {
		m.syncTelemetry()
		m.markSyncSuccess(currentConfigVersion)
		return nil
	}

	systemData, err := m.client.FetchSystemData("/node/api/system")
	if err != nil {
		m.markSyncFailure(currentConfigVersion, err)
		return err
	}

	accessRules, err := m.client.FetchAccessRules("/node/api/access")
	if err != nil {
		m.markSyncFailure(currentConfigVersion, err)
		return err
	}

	certificates, err := m.client.FetchCertificates("/node/api/certs")
	if err != nil {
		m.markSyncFailure(currentConfigVersion, err)
		return err
	}

	if systemData.ConfigVersion != proxyConfigs.ConfigVersion || accessRules.ConfigVersion != proxyConfigs.ConfigVersion || certificates.ConfigVersion != proxyConfigs.ConfigVersion {
		err := fmt.Errorf("proxy=%s system=%s access=%s certs=%s", proxyConfigs.ConfigVersion, systemData.ConfigVersion, accessRules.ConfigVersion, certificates.ConfigVersion)
		m.markSyncFailure(currentConfigVersion, err)
		return err
	}

	if m.Options.ImportSystemData != nil {
		if err := m.Options.ImportSystemData(systemData); err != nil {
			m.markSyncFailure(currentConfigVersion, err)
			return err
		}
	}

	if m.Options.ImportCertificates != nil {
		if err := m.Options.ImportCertificates(certificates); err != nil {
			m.markSyncFailure(currentConfigVersion, err)
			return err
		}
	}

	if m.Options.ImportAccessRules != nil {
		if err := m.Options.ImportAccessRules(accessRules); err != nil {
			m.markSyncFailure(currentConfigVersion, err)
			return err
		}
	}

	if m.Options.ImportProxyConfigs != nil {
		if err := m.Options.ImportProxyConfigs(proxyConfigs); err != nil {
			m.markSyncFailure(currentConfigVersion, err)
			return err
		}
	}

	syncedConfigVersion := proxyConfigs.ConfigVersion
	if m.Options.GetConfigVersion != nil {
		configVersion, err := m.Options.GetConfigVersion()
		if err != nil {
			m.logf("Failed to refresh config version after sync", err)
		} else if configVersion != "" {
			syncedConfigVersion = configVersion
		}
	}

	if err := m.sendNodeStateUpdate(syncedConfigVersion); err != nil {
		m.logf("Failed to report synced node information", err)
	}
	m.syncTelemetry()

	m.markSyncSuccess(syncedConfigVersion)
	m.logf("Configuration synced from primary node", nil)
	return nil
}

func (m *Manager) SyncNow() error {
	if m == nil {
		return fmt.Errorf("node manager is not available")
	}
	if m.Options == nil || !strings.EqualFold(m.Options.Mode, "node") {
		return fmt.Errorf("manual sync is only available in node mode")
	}

	return m.syncNodeConfig()
}

func (m *Manager) syncTelemetry() {
	if m == nil || m.Options == nil || m.Options.GetTelemetrySnapshot == nil || m.client == nil {
		return
	}

	fullTelemetrySnapshot, err := m.Options.GetTelemetrySnapshot()
	if err != nil {
		m.logf("Failed to collect node telemetry snapshot", err)
		return
	}
	if fullTelemetrySnapshot == nil {
		return
	}

	telemetrySnapshot := m.prepareTelemetrySnapshotForUpload(fullTelemetrySnapshot)
	if err := m.client.SendTelemetry("/node/api/telemetry", telemetrySnapshot); err != nil {
		m.logf("Failed to upload node telemetry snapshot", err)
		return
	}

	m.updateTelemetryCursor(fullTelemetrySnapshot)
}

func (m *Manager) SyncTelemetryNow() {
	if m == nil {
		return
	}

	m.syncTelemetry()
}

func (m *Manager) ReportNodeStateNow() {
	if m == nil {
		return
	}

	if err := m.sendNodeStateUpdate(""); err != nil {
		m.logf("Failed to report node state update", err)
	}
}

func (m *Manager) prepareTelemetrySnapshotForUpload(snapshot *TelemetrySnapshot) *TelemetrySnapshot {
	if snapshot == nil {
		return nil
	}

	prepared := &TelemetrySnapshot{
		GeneratedAt: snapshot.GeneratedAt,
		Today:       cloneTelemetrySummary(snapshot.Today),
		Analytics:   []*AnalyticsRecord{},
		UptimeLogs:  map[string][]*uptime.Record{},
		StreamProxy: map[string]*StreamProxyRuntime{},
	}

	for uuid, runtime := range snapshot.StreamProxy {
		prepared.StreamProxy[uuid] = cloneStreamProxyRuntime(runtime)
	}

	if m.syncStatus == nil || !m.syncStatus.TelemetrySeeded {
		prepared.Analytics = mergeAnalyticsRecords(nil, snapshot.Analytics)
		for targetID, records := range snapshot.UptimeLogs {
			prepared.UptimeLogs[targetID] = mergeUptimeRecords(nil, records)
		}
		return prepared
	}

	lastAnalyticsDate := strings.TrimSpace(m.syncStatus.LastTelemetryAnalyticsDate)
	currentAnalyticsDate := time.Now().Format("2006_01_02")
	for _, record := range snapshot.Analytics {
		if record == nil || strings.TrimSpace(record.Date) == "" {
			continue
		}
		if record.Date == currentAnalyticsDate || (lastAnalyticsDate != "" && record.Date > lastAnalyticsDate) {
			prepared.Analytics = append(prepared.Analytics, cloneAnalyticsRecord(record))
		}
	}

	for targetID, records := range snapshot.UptimeLogs {
		lastTimestamp := int64(0)
		if m.syncStatus.LastTelemetryUptimeTimestamp != nil {
			lastTimestamp = m.syncStatus.LastTelemetryUptimeTimestamp[targetID]
		}

		newRecords := make([]*uptime.Record, 0)
		for _, record := range records {
			if record == nil || record.Timestamp <= lastTimestamp {
				continue
			}
			newRecords = append(newRecords, cloneUptimeRecord(record))
		}
		if len(newRecords) > 0 {
			prepared.UptimeLogs[targetID] = newRecords
		}
	}

	return prepared
}

func (m *Manager) updateTelemetryCursor(snapshot *TelemetrySnapshot) {
	if m == nil || snapshot == nil {
		return
	}

	if m.syncStatus == nil {
		m.syncStatus = &SyncStatus{}
	}
	if m.syncStatus.LastTelemetryUptimeTimestamp == nil {
		m.syncStatus.LastTelemetryUptimeTimestamp = map[string]int64{}
	}

	m.syncStatus.TelemetrySeeded = true

	lastAnalyticsDate := strings.TrimSpace(m.syncStatus.LastTelemetryAnalyticsDate)
	for _, record := range snapshot.Analytics {
		if record == nil || strings.TrimSpace(record.Date) == "" {
			continue
		}
		if record.Date > lastAnalyticsDate {
			lastAnalyticsDate = record.Date
		}
	}
	m.syncStatus.LastTelemetryAnalyticsDate = lastAnalyticsDate

	for targetID, records := range snapshot.UptimeLogs {
		lastTimestamp := m.syncStatus.LastTelemetryUptimeTimestamp[targetID]
		for _, record := range records {
			if record == nil {
				continue
			}
			if record.Timestamp > lastTimestamp {
				lastTimestamp = record.Timestamp
			}
		}
		if lastTimestamp > 0 {
			m.syncStatus.LastTelemetryUptimeTimestamp[targetID] = lastTimestamp
		}
	}

	m.saveSyncStatus()
}

func (m *Manager) EnsureInitialSync() error {
	if m == nil || m.Options == nil || !strings.EqualFold(m.Options.Mode, "node") {
		return nil
	}

	if err := m.syncNodeConfig(); err != nil {
		if m.syncStatus != nil && m.syncStatus.HasPreviousSync {
			m.logf("Primary node unavailable, continuing with previous synced state", err)
			return nil
		}
		return fmt.Errorf("initial node sync failed: %w", err)
	}

	return nil
}

func (m *Manager) GetSyncStatus() *SyncStatus {
	if m == nil || m.syncStatus == nil {
		return &SyncStatus{}
	}

	clonedStatus := *m.syncStatus
	if m.syncStatus.LastTelemetryUptimeTimestamp != nil {
		clonedStatus.LastTelemetryUptimeTimestamp = make(map[string]int64, len(m.syncStatus.LastTelemetryUptimeTimestamp))
		for key, value := range m.syncStatus.LastTelemetryUptimeTimestamp {
			clonedStatus.LastTelemetryUptimeTimestamp[key] = value
		}
	}
	return &clonedStatus
}

// Wrapper function to log error
func (m *Manager) logf(message string, originalError error) {
	if m.Options.Logger == nil {
		//Print to fmt
		if originalError != nil {
			message += ": " + originalError.Error()
		}
		println(message)
		return
	}
	m.Options.Logger.PrintAndLog("nodes", message, originalError)
}

func (m *Manager) ListRegisteredNodes() ([]*Node, error) {
	return m.Nodes, nil
}

func (m *Manager) RegisterNode(Node *Node) error {
	m.Nodes = append(m.Nodes, Node)
	m.SaveConfigToDatabase()
	return nil
}

func (m *Manager) UnregisterNode(Node *Node) error {
	err := os.Remove(filepath.Join(m.Options.ConfigStore, Node.ID+".config"))
	if err != nil {
		return err
	}
	_ = os.Remove(m.getTelemetryFilePath(Node.ID))
	for i, a := range m.Nodes {
		if a.ID == Node.ID {
			m.Nodes = append(m.Nodes[:i], m.Nodes[i+1:]...)
			m.SaveConfigToDatabase()
			return nil
		}
	}
	return fmt.Errorf("node %s not found", Node.ID)
}

func (m *Manager) UpdateNodeLastSeen(Node *Node) error {
	if Node == nil {
		return fmt.Errorf("Node cannot be nil")
	}
	Node.LastSeen = time.Now()
	m.SaveConfigToDatabase()
	return nil
}

func (m *Manager) GetNodeByID(NodeID string) (*Node, error) {
	for _, Node := range m.Nodes {
		if Node.ID == NodeID {
			return Node, nil
		}
	}
	return nil, fmt.Errorf("node %s not found", NodeID)
}

func (m *Manager) GetNodeByToken(token string) (*Node, error) {
	for _, Node := range m.Nodes {
		if Node.Token == token {
			return Node, nil
		}
	}
	return nil, fmt.Errorf("node %s not found", token)
}

func (m *Manager) GetNodeByHost(host string) (*Node, error) {
	for _, Node := range m.Nodes {
		if Node.Host == host {
			return Node, nil
		}
	}
	return nil, fmt.Errorf("host %s not found", host)
}

func (m *Manager) GenerateJoinRequest(name string) (*Node, error) {
	node := &Node{
		ID:           uuid.New().String(),
		Name:         strings.TrimSpace(name),
		Enabled:      true,
		Token:        generateNodeToken(),
		RegisteredAt: time.Now(),
	}
	if err := m.RegisterNode(node); err != nil {
		return nil, err
	}
	return node, nil
}

func (m *Manager) CleanupJoinRequests() {
	for _, node := range m.Nodes {
		if node.RegisteredAt.Add(time.Hour * 24).Before(time.Now()) {
			err := m.UnregisterNode(node)
			if err != nil {
				m.logf("Failed to cleanup join request", err)
				return
			}
		}
	}
}

// Save all configs to ConfigStore folder
func (m *Manager) SaveConfigToDatabase() {
	for _, node := range m.Nodes {
		configBytes, err := json.Marshal(node)
		if err != nil {
			m.logf("Failed to marshal stream proxy config", err)
			continue
		}
		err = os.WriteFile(m.Options.ConfigStore+"/"+node.ID+".config", configBytes, 0775)
		if err != nil {
			m.logf("Failed to save stream proxy config", err)
		}
	}
}

func (m *Manager) getHostname() (string, error) {
	if m.Options != nil && m.Options.HostnameProvider != nil {
		return m.Options.HostnameProvider()
	}

	return os.Hostname()
}

func (m *Manager) getNodeIP() string {
	if m.Options != nil && m.Options.NodeIPProvider != nil {
		return strings.TrimSpace(m.Options.NodeIPProvider())
	}

	return ""
}

func (m *Manager) isLocalOverrideEnabled() bool {
	if m == nil || m.Options == nil || m.Options.LocalOverrideProvider == nil {
		return false
	}

	return m.Options.LocalOverrideProvider()
}

func (m *Manager) RotateNodeToken(targetNode *Node) error {
	if targetNode == nil {
		return fmt.Errorf("node cannot be nil")
	}

	targetNode.Token = generateNodeToken()
	m.SaveConfigToDatabase()
	return nil
}

func (m *Manager) SetNodeEnabled(targetNode *Node, enabled bool) error {
	if targetNode == nil {
		return fmt.Errorf("node cannot be nil")
	}

	targetNode.Enabled = enabled
	m.SaveConfigToDatabase()
	return nil
}

func (m *Manager) SetUpdateInterval(interval time.Duration) {
	if m == nil {
		return
	}

	m.tickerMutex.Lock()
	defer m.tickerMutex.Unlock()

	if m.updateTicket != nil {
		m.updateTicket.Stop()
		m.updateTicket = nil
	}
	if m.updateTicketStopChan != nil {
		close(m.updateTicketStopChan)
		m.updateTicketStopChan = nil
	}

	if m.Options != nil {
		m.Options.UpdateInterval = interval
	}

	if interval <= 0 {
		return
	}

	m.updateTicketStopChan = make(chan bool)
	m.updateTicket = time.NewTicker(interval)
	go func(m *Manager, ticker *time.Ticker, stopChan chan bool) {
		for {
			select {
			case <-stopChan:
				return
			case <-ticker.C:
				m.updateNodeConfig()
			}
		}
	}(m, m.updateTicket, m.updateTicketStopChan)
}

func (m *Manager) GetUpdateInterval() time.Duration {
	if m == nil || m.Options == nil {
		return 0
	}

	return m.Options.UpdateInterval
}

func (m *Manager) GetOnlineThreshold() time.Duration {
	if m != nil && m.sysdb != nil && m.sysdb.KeyExists("settings", "node_offline_threshold_seconds") {
		offlineThresholdSeconds := 0
		if err := m.sysdb.Read("settings", "node_offline_threshold_seconds", &offlineThresholdSeconds); err == nil {
			if offlineThresholdSeconds < 30 {
				offlineThresholdSeconds = 30
			}
			return time.Duration(offlineThresholdSeconds) * time.Second
		}
	}

	intervalSeconds := 60
	if m != nil && m.sysdb != nil && m.sysdb.KeyExists("settings", "node_sync_interval_seconds") {
		if err := m.sysdb.Read("settings", "node_sync_interval_seconds", &intervalSeconds); err != nil {
			intervalSeconds = 60
		}
	}
	if intervalSeconds < 10 {
		intervalSeconds = 10
	}
	return (time.Duration(intervalSeconds) * time.Second * 2) + (30 * time.Second)
}

func (m *Manager) IsNodeOnline(targetNode *Node) bool {
	if targetNode == nil || targetNode.LastSeen.IsZero() {
		return false
	}

	return time.Since(targetNode.LastSeen) <= m.GetOnlineThreshold()
}

func normalizeVersion(version string) string {
	version = strings.TrimSpace(version)
	version = strings.TrimPrefix(version, "v")
	version = strings.TrimPrefix(version, "V")
	return version
}

func versionsMatch(primaryVersion string, nodeVersion string) bool {
	primaryVersion = normalizeVersion(primaryVersion)
	nodeVersion = normalizeVersion(nodeVersion)
	if primaryVersion == "" || nodeVersion == "" {
		return false
	}

	return primaryVersion == nodeVersion
}

func (m *Manager) GetPrimaryVersion() string {
	if m == nil || m.Options == nil {
		return ""
	}

	return strings.TrimSpace(m.Options.SystemVersion)
}

func (m *Manager) IsVersionMatchRequired() bool {
	if m == nil || m.sysdb == nil || !m.sysdb.KeyExists("settings", "node_require_version_match") {
		return true
	}

	required := true
	if err := m.sysdb.Read("settings", "node_require_version_match", &required); err != nil {
		return true
	}

	return required
}

func (m *Manager) GetNodeVersionMismatch(targetNode *Node) (bool, string) {
	if targetNode == nil || !m.IsVersionMatchRequired() {
		return false, ""
	}

	primaryVersion := strings.TrimSpace(m.GetPrimaryVersion())
	nodeVersion := strings.TrimSpace(targetNode.ZoraxyVersion)
	if !versionsMatch(primaryVersion, nodeVersion) {
		if normalizeVersion(primaryVersion) == "" || normalizeVersion(nodeVersion) == "" {
			return false, ""
		}

		message := fmt.Sprintf("Primary version %s does not match node version %s. Sync is blocked until versions match or version enforcement is disabled.", primaryVersion, nodeVersion)
		return true, message
	}

	return false, ""
}

func (m *Manager) markSyncAttempt() {
	if m.syncStatus == nil {
		m.syncStatus = &SyncStatus{}
	}
	m.syncStatus.Mode = m.Options.Mode
	m.syncStatus.PrimaryServer = m.Options.NodeServer
	m.syncStatus.LastAttemptAt = time.Now()
	m.saveSyncStatus()
}

func (m *Manager) markSyncSuccess(configVersion string) {
	if m.syncStatus == nil {
		m.syncStatus = &SyncStatus{}
	}
	m.syncStatus.Mode = m.Options.Mode
	m.syncStatus.PrimaryServer = m.Options.NodeServer
	m.syncStatus.HasPreviousSync = true
	m.syncStatus.LastSuccessAt = time.Now()
	m.syncStatus.LastConfigVersion = configVersion
	m.syncStatus.LastError = ""
	m.syncStatus.LocalOverride = false
	m.saveSyncStatus()
}

func (m *Manager) markSyncFailure(configVersion string, err error) {
	if m.syncStatus == nil {
		m.syncStatus = &SyncStatus{}
	}
	m.syncStatus.Mode = m.Options.Mode
	m.syncStatus.PrimaryServer = m.Options.NodeServer
	m.syncStatus.LastConfigVersion = configVersion
	m.syncStatus.LocalOverride = false
	if err != nil {
		m.syncStatus.LastError = err.Error()
		m.logf("Node synchronization with primary failed", err)
	}
	m.saveSyncStatus()
}

func (m *Manager) markSyncLocalOverride(configVersion string) {
	if m.syncStatus == nil {
		m.syncStatus = &SyncStatus{}
	}
	m.syncStatus.Mode = m.Options.Mode
	m.syncStatus.PrimaryServer = m.Options.NodeServer
	m.syncStatus.LastAttemptAt = time.Now()
	m.syncStatus.LastConfigVersion = configVersion
	m.syncStatus.LastError = ""
	m.syncStatus.LocalOverride = true
	m.saveSyncStatus()
}

func (m *Manager) saveSyncStatus() {
	if m.statusFile == "" || m.syncStatus == nil {
		return
	}

	if err := SaveSyncStatus(m.statusFile, m.syncStatus); err != nil {
		m.logf("Failed to save node sync status", err)
	}
}

func generateNodeToken() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return uuid.New().String()
	}

	hash := sha256.Sum256(bytes)
	return hex.EncodeToString(hash[:])
}
