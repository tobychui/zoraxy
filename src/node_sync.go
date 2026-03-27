package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"imuslab.com/zoraxy/mod/access"
	"imuslab.com/zoraxy/mod/auth/sso/forward"
	"imuslab.com/zoraxy/mod/auth/sso/zorxauth"
	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/dynamicproxy"
	"imuslab.com/zoraxy/mod/dynamicproxy/loadbalance"
	"imuslab.com/zoraxy/mod/dynamicproxy/redirection"
	"imuslab.com/zoraxy/mod/node"
	"imuslab.com/zoraxy/mod/streamproxy"
	"imuslab.com/zoraxy/mod/tlscert"
	"imuslab.com/zoraxy/mod/utils"
)

type nodeSyncPayload struct {
	ServiceEnabled bool                        `json:"service_enabled"`
	RootConfig     json.RawMessage             `json:"root_config,omitempty"`
	ProxyConfigs   []json.RawMessage           `json:"proxy_configs"`
	AccessRules    []*access.AccessRule        `json:"access_rules"`
	TrustedProxies []access.TrustedProxy       `json:"trusted_proxies"`
	Certificates   []tlscert.StoredCertificate `json:"certificates"`
	DatabaseBackup json.RawMessage             `json:"database_backup"`
	StreamConfigs  []json.RawMessage           `json:"stream_configs,omitempty"`
	RedirectRules  []json.RawMessage           `json:"redirect_rules,omitempty"`
}

type preservedLocalNodeSettings struct {
	NodeName           string
	ConfigUnlocked     bool
	InboundPort        int
	UseTLS             bool
	MinTLSVersion      string
	DevelopmentMode    bool
	ListenOnPort80     bool
	ForceHTTPSRedirect bool
	UseProxyProtocol   bool
}

func getNodeLocalPreservedTables() []string {
	return []string{
		"stats",
		forward.DatabaseTable,
		zorxauth.DB_NAME,
		zorxauth.DB_USERS_TABLE,
		zorxauth.DB_BROWSER_SESSIONS_TABLE,
		zorxauth.DB_GATEWAY_SESSIONS_TABLE,
	}
}

func isNodeLocalBackupSettingKey(key string) bool {
	switch strings.TrimSpace(key) {
	case localNodeDisplayNameSettingKey,
		localNodeConfigWriteUnlockedKey,
		"inbound",
		"usetls",
		"minTLSVersion",
		"devMode",
		"listenP80",
		"redirect",
		"useProxyProtocol":
		return true
	default:
		return false
	}
}

func capturePreservedLocalNodeSettings() preservedLocalNodeSettings {
	settings := preservedLocalNodeSettings{
		NodeName:           getStoredLocalNodeName(),
		ConfigUnlocked:     getLocalNodeConfigWriteUnlocked(),
		InboundPort:        *defaultInboundPort,
		UseTLS:             true,
		MinTLSVersion:      "1.2",
		DevelopmentMode:    false,
		ListenOnPort80:     true,
		ForceHTTPSRedirect: true,
		UseProxyProtocol:   false,
	}

	if sysdb == nil {
		return settings
	}

	_ = sysdb.Read("settings", "inbound", &settings.InboundPort)
	_ = sysdb.Read("settings", "usetls", &settings.UseTLS)
	_ = sysdb.Read("settings", "minTLSVersion", &settings.MinTLSVersion)
	_ = sysdb.Read("settings", "devMode", &settings.DevelopmentMode)
	_ = sysdb.Read("settings", "listenP80", &settings.ListenOnPort80)
	_ = sysdb.Read("settings", "redirect", &settings.ForceHTTPSRedirect)
	_ = sysdb.Read("settings", "useProxyProtocol", &settings.UseProxyProtocol)

	return settings
}

func restorePreservedLocalNodeSettings(settings preservedLocalNodeSettings) error {
	if err := setStoredLocalNodeName(settings.NodeName); err != nil {
		return err
	}
	if err := setLocalNodeConfigWriteUnlocked(settings.ConfigUnlocked); err != nil {
		return err
	}
	if sysdb == nil {
		return nil
	}

	if err := sysdb.Write("settings", "inbound", settings.InboundPort); err != nil {
		return err
	}
	if err := sysdb.Write("settings", "usetls", settings.UseTLS); err != nil {
		return err
	}
	if err := sysdb.Write("settings", "minTLSVersion", settings.MinTLSVersion); err != nil {
		return err
	}
	if err := sysdb.Write("settings", "devMode", settings.DevelopmentMode); err != nil {
		return err
	}
	if err := sysdb.Write("settings", "listenP80", settings.ListenOnPort80); err != nil {
		return err
	}
	if err := sysdb.Write("settings", "redirect", settings.ForceHTTPSRedirect); err != nil {
		return err
	}
	if err := sysdb.Write("settings", "useProxyProtocol", settings.UseProxyProtocol); err != nil {
		return err
	}

	return nil
}

func stripNodeLocalSettingsFromBackup(backupJSON []byte) ([]byte, error) {
	if len(backupJSON) == 0 {
		return backupJSON, nil
	}

	backup := database.BackupData{}
	if err := json.Unmarshal(backupJSON, &backup); err != nil {
		return nil, err
	}

	if len(backup.Tables) == 0 {
		return backupJSON, nil
	}

	settingsEntries, ok := backup.Tables["settings"]
	if !ok {
		return backupJSON, nil
	}

	filteredEntries := make([]database.BackupEntry, 0, len(settingsEntries))
	for _, entry := range settingsEntries {
		if isNodeLocalBackupSettingKey(entry.Key) {
			continue
		}
		filteredEntries = append(filteredEntries, entry)
	}
	backup.Tables["settings"] = filteredEntries

	return json.MarshalIndent(backup, "", "  ")
}

func shouldExportForNode(assignedNodeID string, targetNode *node.Node) bool {
	assignedNodeID = strings.TrimSpace(assignedNodeID)
	if targetNode == nil {
		return assignedNodeID == ""
	}
	return assignedNodeID == targetNode.ID
}

func normalizeNodeVirtualDirectories(endpoint *dynamicproxy.ProxyEndpoint, targetNode *node.Node) {
	filteredVdirs := make([]*dynamicproxy.VirtualDirectoryEndpoint, 0, len(endpoint.VirtualDirectories))
	for _, vdir := range endpoint.VirtualDirectories {
		if vdir == nil || !shouldExportForNode(vdir.AssignedNodeID, targetNode) {
			continue
		}

		vdir.AssignedNodeID = ""
		filteredVdirs = append(filteredVdirs, vdir)
	}
	endpoint.VirtualDirectories = filteredVdirs
}

func applyNodeRootOverride(rootConfig *dynamicproxy.ProxyEndpoint, targetNode *node.Node) {
	if rootConfig == nil {
		return
	}

	if targetNode != nil && rootConfig.NodeDefaultSites != nil {
		if override, ok := rootConfig.NodeDefaultSites[targetNode.ID]; ok && override != nil {
			rootConfig.DefaultSiteOption = override.DefaultSiteOption
			rootConfig.DefaultSiteValue = override.DefaultSiteValue
			rootConfig.ActiveOrigins = []*loadbalance.Upstream{
				{
					OriginIpOrDomain:         override.OriginIpOrDomain,
					RequireTLS:               override.RequireTLS,
					SkipCertValidations:      override.SkipCertValidations,
					SkipWebSocketOriginCheck: true,
					Weight:                   1,
				},
			}
		}
	}

	rootConfig.NodeDefaultSites = map[string]*dynamicproxy.NodeDefaultSiteConfig{}
}

func buildNodeSyncPayload(targetNode *node.Node) (*nodeSyncPayload, error) {
	if dynamicProxyRouter == nil {
		return nil, errors.New("reverse proxy router is not ready")
	}
	if accessController == nil {
		return nil, errors.New("access controller is not ready")
	}
	if tlsCertManager == nil {
		return nil, errors.New("tls certificate manager is not ready")
	}
	if sysdb == nil {
		return nil, errors.New("system database is not ready")
	}

	payload := &nodeSyncPayload{
		ServiceEnabled: true,
		ProxyConfigs:   []json.RawMessage{},
		AccessRules:    []*access.AccessRule{},
		TrustedProxies: []access.TrustedProxy{},
		Certificates:   []tlscert.StoredCertificate{},
		StreamConfigs:  []json.RawMessage{},
		RedirectRules:  []json.RawMessage{},
	}

	if targetNode != nil {
		payload.ServiceEnabled = targetNode.Enabled
	} else if *mode == "node" {
		payload.ServiceEnabled = getDesiredLocalNodeServiceState()
	}

	if dynamicProxyRouter.Root != nil {
		rootConfig, err := normalizeNodeProxyConfig(dynamicProxyRouter.Root)
		if err != nil {
			return nil, err
		}
		normalizeNodeVirtualDirectories(rootConfig, targetNode)
		applyNodeRootOverride(rootConfig, targetNode)

		rootJSON, err := json.Marshal(rootConfig)
		if err != nil {
			return nil, err
		}
		payload.RootConfig = rootJSON
	}

	hostConfigs := make([]*dynamicproxy.ProxyEndpoint, 0)
	for _, endpoint := range dynamicProxyRouter.GetProxyEndpointsAsMap() {
		if !shouldExportForNode(endpoint.AssignedNodeID, targetNode) {
			continue
		}

		normalizedEndpoint, err := normalizeNodeProxyConfig(endpoint)
		if err != nil {
			return nil, err
		}
		normalizedEndpoint.AssignedNodeID = ""
		normalizeNodeVirtualDirectories(normalizedEndpoint, targetNode)
		normalizedEndpoint.NodeDefaultSites = map[string]*dynamicproxy.NodeDefaultSiteConfig{}
		hostConfigs = append(hostConfigs, normalizedEndpoint)
	}

	sort.Slice(hostConfigs, func(i, j int) bool {
		return hostConfigs[i].RootOrMatchingDomain < hostConfigs[j].RootOrMatchingDomain
	})

	for _, endpoint := range hostConfigs {
		endpointJSON, err := json.Marshal(endpoint)
		if err != nil {
			return nil, err
		}
		payload.ProxyConfigs = append(payload.ProxyConfigs, endpointJSON)
	}

	accessRules := accessController.ListAllAccessRules()
	clonedRules := make([]*access.AccessRule, 0, len(accessRules))
	for _, rule := range accessRules {
		clonedRule, err := access.CloneAccessRule(rule)
		if err != nil {
			return nil, err
		}
		clonedRules = append(clonedRules, clonedRule)
	}

	sort.Slice(clonedRules, func(i, j int) bool {
		return clonedRules[i].ID < clonedRules[j].ID
	})
	payload.AccessRules = clonedRules

	trustedProxies := accessController.ListTrustedProxies()
	sort.Slice(trustedProxies, func(i, j int) bool {
		return trustedProxies[i].IP < trustedProxies[j].IP
	})
	payload.TrustedProxies = trustedProxies

	certificates, err := tlsCertManager.ExportStoredCertificates()
	if err != nil {
		return nil, err
	}
	payload.Certificates = certificates

	databaseBackup, err := sysdb.BackupExcludeTables([]string{"stats"})
	if err != nil {
		return nil, err
	}
	databaseBackup, err = stripNodeLocalSettingsFromBackup(databaseBackup)
	if err != nil {
		return nil, err
	}
	payload.DatabaseBackup = databaseBackup

	streamConfigs, err := exportNodeStreamConfigs(targetNode)
	if err != nil {
		return nil, err
	}
	payload.StreamConfigs = streamConfigs

	redirectRules, err := exportNodeRedirectRules(targetNode)
	if err != nil {
		return nil, err
	}
	payload.RedirectRules = redirectRules

	return payload, nil
}

func exportNodeStreamConfigs(targetNode *node.Node) ([]json.RawMessage, error) {
	if streamProxyManager == nil {
		return []json.RawMessage{}, nil
	}

	results := make([]json.RawMessage, 0)
	streamConfigs := make([]*streamproxy.ProxyRelayInstance, 0)
	for _, config := range streamProxyManager.Configs {
		if config == nil || !shouldExportForNode(config.AssignedNodeID, targetNode) {
			continue
		}

		clonedConfig, err := streamproxy.CloneConfig(config)
		if err != nil {
			return nil, err
		}
		clonedConfig.AssignedNodeID = ""
		streamConfigs = append(streamConfigs, clonedConfig)
	}

	sort.Slice(streamConfigs, func(i, j int) bool {
		return streamConfigs[i].UUID < streamConfigs[j].UUID
	})

	for _, config := range streamConfigs {
		configJSON, err := json.Marshal(config)
		if err != nil {
			return nil, err
		}
		results = append(results, configJSON)
	}

	return results, nil
}

func exportNodeRedirectRules(targetNode *node.Node) ([]json.RawMessage, error) {
	if redirectTable == nil {
		return []json.RawMessage{}, nil
	}

	results := make([]json.RawMessage, 0)
	redirectRules := make([]*redirection.RedirectRules, 0)
	for _, rule := range redirectTable.GetAllRedirectRules() {
		if rule == nil || !shouldExportForNode(rule.AssignedNodeID, targetNode) {
			continue
		}

		clonedRule, err := redirection.CloneRedirectRule(rule)
		if err != nil {
			return nil, err
		}
		clonedRule.AssignedNodeID = ""
		redirectRules = append(redirectRules, clonedRule)
	}

	sort.Slice(redirectRules, func(i, j int) bool {
		return redirectRules[i].RedirectURL < redirectRules[j].RedirectURL
	})

	for _, rule := range redirectRules {
		ruleJSON, err := json.Marshal(rule)
		if err != nil {
			return nil, err
		}
		results = append(results, ruleJSON)
	}

	return results, nil
}

func hashNodeSyncPayload(payload *nodeSyncPayload) (string, error) {
	js, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(js)
	return hex.EncodeToString(sum[:]), nil
}

func getCurrentNodeConfigVersion() (string, error) {
	payload, err := buildNodeSyncPayload(nil)
	if err != nil {
		return "", err
	}

	return hashNodeSyncPayload(payload)
}

func exportNodeProxyConfigs() (*node.ProxyConfigSnapshot, error) {
	return exportNodeProxyConfigsForTarget(nil)
}

func exportNodeProxyConfigsForNode(targetNode *node.Node) (*node.ProxyConfigSnapshot, error) {
	return exportNodeProxyConfigsForTarget(targetNode)
}

func exportNodeProxyConfigsForTarget(targetNode *node.Node) (*node.ProxyConfigSnapshot, error) {
	payload, err := buildNodeSyncPayload(targetNode)
	if err != nil {
		return nil, err
	}

	configVersion, err := hashNodeSyncPayload(payload)
	if err != nil {
		return nil, err
	}

	return &node.ProxyConfigSnapshot{
		ConfigVersion:       configVersion,
		PrimaryVersion:      SYSTEM_VERSION,
		RequireVersionMatch: getRequireNodeVersionMatch(),
		RootConfig:          payload.RootConfig,
		ProxyConfigs:        payload.ProxyConfigs,
	}, nil
}

func exportNodeAccessRules() (*node.AccessSnapshot, error) {
	return exportNodeAccessRulesForTarget(nil)
}

func exportNodeAccessRulesForNode(targetNode *node.Node) (*node.AccessSnapshot, error) {
	return exportNodeAccessRulesForTarget(targetNode)
}

func exportNodeAccessRulesForTarget(targetNode *node.Node) (*node.AccessSnapshot, error) {
	payload, err := buildNodeSyncPayload(targetNode)
	if err != nil {
		return nil, err
	}

	configVersion, err := hashNodeSyncPayload(payload)
	if err != nil {
		return nil, err
	}

	return &node.AccessSnapshot{
		ConfigVersion:  configVersion,
		AccessRules:    payload.AccessRules,
		TrustedProxies: payload.TrustedProxies,
	}, nil
}

func exportNodeCertificates() (*node.CertificateSnapshot, error) {
	return exportNodeCertificatesForTarget(nil)
}

func exportNodeCertificatesForNode(targetNode *node.Node) (*node.CertificateSnapshot, error) {
	return exportNodeCertificatesForTarget(targetNode)
}

func exportNodeCertificatesForTarget(targetNode *node.Node) (*node.CertificateSnapshot, error) {
	payload, err := buildNodeSyncPayload(targetNode)
	if err != nil {
		return nil, err
	}

	configVersion, err := hashNodeSyncPayload(payload)
	if err != nil {
		return nil, err
	}

	return &node.CertificateSnapshot{
		ConfigVersion: configVersion,
		Certificates:  payload.Certificates,
	}, nil
}

func exportNodeSystemData() (*node.SystemSnapshot, error) {
	return exportNodeSystemDataForTarget(nil)
}

func exportNodeSystemDataForNode(targetNode *node.Node) (*node.SystemSnapshot, error) {
	return exportNodeSystemDataForTarget(targetNode)
}

func exportNodeSystemDataForTarget(targetNode *node.Node) (*node.SystemSnapshot, error) {
	payload, err := buildNodeSyncPayload(targetNode)
	if err != nil {
		return nil, err
	}

	configVersion, err := hashNodeSyncPayload(payload)
	if err != nil {
		return nil, err
	}

	return &node.SystemSnapshot{
		ConfigVersion:  configVersion,
		ServiceEnabled: payload.ServiceEnabled,
		DatabaseBackup: payload.DatabaseBackup,
		StreamConfigs:  payload.StreamConfigs,
		RedirectRules:  payload.RedirectRules,
	}, nil
}

func importNodeAccessRules(snapshot *node.AccessSnapshot) error {
	if snapshot == nil {
		return errors.New("access snapshot is empty")
	}
	if accessController == nil {
		return errors.New("access controller is not ready")
	}

	return accessController.ReplaceAllAccessRules(snapshot.AccessRules, snapshot.TrustedProxies)
}

func importNodeCertificates(snapshot *node.CertificateSnapshot) error {
	if snapshot == nil {
		return errors.New("certificate snapshot is empty")
	}
	if tlsCertManager == nil {
		return errors.New("tls certificate manager is not ready")
	}

	return tlsCertManager.ReplaceStoredCertificates(snapshot.Certificates)
}

func importNodeSystemData(snapshot *node.SystemSnapshot) error {
	if snapshot == nil {
		return errors.New("system snapshot is empty")
	}
	if sysdb == nil {
		return errors.New("system database is not ready")
	}

	preservedLocalSettings := capturePreservedLocalNodeSettings()

	if len(snapshot.DatabaseBackup) > 0 {
		if err := sysdb.RestoreReplacePreservingTables(snapshot.DatabaseBackup, getNodeLocalPreservedTables()); err != nil {
			return err
		}
	}
	if err := restorePreservedLocalNodeSettings(preservedLocalSettings); err != nil {
		return err
	}

	if err := importNodeRedirectRules(snapshot.RedirectRules); err != nil {
		return err
	}

	if err := importNodeStreamConfigs(snapshot.StreamConfigs); err != nil {
		return err
	}

	if err := setDesiredLocalNodeServiceState(snapshot.ServiceEnabled); err != nil {
		return err
	}
	if nodeManager != nil {
		nodeManager.SetUpdateInterval(getConfiguredNodeSyncInterval())
	}
	if err := applyDesiredLocalNodeServiceState(); err != nil {
		return err
	}

	return nil
}

func importNodeRedirectRules(rawRules []json.RawMessage) error {
	if redirectTable == nil {
		return nil
	}

	decodedRules := make([]*redirection.RedirectRules, 0, len(rawRules))
	for _, rawRule := range rawRules {
		if len(rawRule) == 0 {
			continue
		}

		decodedRule := &redirection.RedirectRules{}
		if err := json.Unmarshal(rawRule, decodedRule); err != nil {
			return err
		}
		decodedRules = append(decodedRules, decodedRule)
	}

	return redirectTable.ReplaceAllRules(decodedRules)
}

func importNodeStreamConfigs(rawConfigs []json.RawMessage) error {
	if streamProxyManager == nil {
		return nil
	}

	decodedConfigs := make([]*streamproxy.ProxyRelayInstance, 0, len(rawConfigs))
	for _, rawConfig := range rawConfigs {
		if len(rawConfig) == 0 {
			continue
		}

		decodedConfig := &streamproxy.ProxyRelayInstance{}
		if err := json.Unmarshal(rawConfig, decodedConfig); err != nil {
			return err
		}
		decodedConfigs = append(decodedConfigs, decodedConfig)
	}

	return streamProxyManager.ReplaceConfigsFromSync(decodedConfigs)
}

func importNodeProxyConfigs(snapshot *node.ProxyConfigSnapshot) error {
	if snapshot == nil {
		return errors.New("proxy configuration snapshot is empty")
	}
	if dynamicProxyRouter == nil {
		return errors.New("reverse proxy router is not ready")
	}

	rootConfig, proxyConfigs, err := decodeNodeProxySnapshot(snapshot)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(CONF_HTTP_PROXY, 0775); err != nil {
		return err
	}

	configFiles, err := filepath.Glob(filepath.Join(CONF_HTTP_PROXY, "*.config"))
	if err != nil {
		return err
	}

	for _, configFile := range configFiles {
		if err := os.Remove(configFile); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	dynamicProxyRouter.ProxyEndpoints.Range(func(key, value interface{}) bool {
		dynamicProxyRouter.ProxyEndpoints.Delete(key)
		return true
	})
	dynamicProxyRouter.Root = nil

	if rootConfig != nil {
		readyRoot, err := dynamicProxyRouter.PrepareProxyRoute(rootConfig)
		if err != nil {
			return err
		}
		if err := dynamicProxyRouter.SetProxyRouteAsRoot(readyRoot); err != nil {
			return err
		}
		if err := SaveReverseProxyConfig(rootConfig); err != nil {
			return err
		}
	} else {
		defaultRoot, err := GetDefaultRootConfig()
		if err != nil {
			return err
		}
		if err := dynamicProxyRouter.SetProxyRouteAsRoot(defaultRoot); err != nil {
			return err
		}
		if err := SaveReverseProxyConfig(dynamicproxy.CopyEndpoint(defaultRoot)); err != nil {
			return err
		}
	}

	for _, proxyConfig := range proxyConfigs {
		readyConfig, err := dynamicProxyRouter.PrepareProxyRoute(proxyConfig)
		if err != nil {
			return err
		}
		if err := dynamicProxyRouter.AddProxyRouteToRuntime(readyConfig); err != nil {
			return err
		}
		if err := SaveReverseProxyConfig(proxyConfig); err != nil {
			return err
		}
	}

	UpdateUptimeMonitorTargets()
	if *mode == "node" && uptimeMonitor != nil {
		uptimeMonitor.ExecuteUptimeCheck()
	}
	return nil
}

func decodeNodeProxySnapshot(snapshot *node.ProxyConfigSnapshot) (*dynamicproxy.ProxyEndpoint, []*dynamicproxy.ProxyEndpoint, error) {
	var rootConfig *dynamicproxy.ProxyEndpoint
	if len(snapshot.RootConfig) > 0 {
		decodedRoot, err := decodeNodeProxyConfig(snapshot.RootConfig)
		if err != nil {
			return nil, nil, err
		}
		decodedRoot.ProxyType = dynamicproxy.ProxyTypeRoot
		decodedRoot.RootOrMatchingDomain = "/"
		rootConfig = decodedRoot
	}

	proxyConfigs := make([]*dynamicproxy.ProxyEndpoint, 0, len(snapshot.ProxyConfigs))
	for _, rawConfig := range snapshot.ProxyConfigs {
		decodedConfig, err := decodeNodeProxyConfig(rawConfig)
		if err != nil {
			return nil, nil, err
		}

		if decodedConfig.ProxyType == dynamicproxy.ProxyTypeRoot || decodedConfig.RootOrMatchingDomain == "/" {
			if rootConfig == nil {
				decodedConfig.ProxyType = dynamicproxy.ProxyTypeRoot
				decodedConfig.RootOrMatchingDomain = "/"
				rootConfig = decodedConfig
			}
			continue
		}

		decodedConfig.ProxyType = dynamicproxy.ProxyTypeHost
		proxyConfigs = append(proxyConfigs, decodedConfig)
	}

	sort.Slice(proxyConfigs, func(i, j int) bool {
		return proxyConfigs[i].RootOrMatchingDomain < proxyConfigs[j].RootOrMatchingDomain
	})

	return rootConfig, proxyConfigs, nil
}

func decodeNodeProxyConfig(rawConfig json.RawMessage) (*dynamicproxy.ProxyEndpoint, error) {
	if len(rawConfig) == 0 {
		return nil, errors.New("proxy configuration payload is empty")
	}

	proxyConfig := dynamicproxy.GetDefaultProxyEndpoint()
	if err := json.Unmarshal(rawConfig, &proxyConfig); err != nil {
		return nil, err
	}

	if proxyConfig.RootOrMatchingDomain == "" {
		proxyConfig.RootOrMatchingDomain = "/"
	}
	if proxyConfig.Tags == nil {
		proxyConfig.Tags = []string{}
	}
	if proxyConfig.NodeDefaultSites == nil {
		proxyConfig.NodeDefaultSites = map[string]*dynamicproxy.NodeDefaultSiteConfig{}
	}
	if proxyConfig.TlsOptions == nil {
		proxyConfig.TlsOptions = tlscert.GetDefaultHostSpecificTlsBehavior()
	}
	proxyConfig.AssignedNodeID = strings.TrimSpace(proxyConfig.AssignedNodeID)
	for _, vdir := range proxyConfig.VirtualDirectories {
		if vdir == nil {
			continue
		}
		vdir.AssignedNodeID = strings.TrimSpace(vdir.AssignedNodeID)
	}

	return &proxyConfig, nil
}

func normalizeNodeProxyConfig(endpoint *dynamicproxy.ProxyEndpoint) (*dynamicproxy.ProxyEndpoint, error) {
	if endpoint == nil {
		return nil, errors.New("proxy endpoint is nil")
	}

	normalizedConfig := dynamicproxy.GetDefaultProxyEndpoint()
	endpointJSON, err := json.Marshal(dynamicproxy.CopyEndpoint(endpoint))
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(endpointJSON, &normalizedConfig); err != nil {
		return nil, err
	}

	if normalizedConfig.RootOrMatchingDomain == "" {
		normalizedConfig.RootOrMatchingDomain = "/"
	}
	if normalizedConfig.Tags == nil {
		normalizedConfig.Tags = []string{}
	}
	if normalizedConfig.NodeDefaultSites == nil {
		normalizedConfig.NodeDefaultSites = map[string]*dynamicproxy.NodeDefaultSiteConfig{}
	}
	if normalizedConfig.TlsOptions == nil {
		normalizedConfig.TlsOptions = tlscert.GetDefaultHostSpecificTlsBehavior()
	}
	normalizedConfig.AssignedNodeID = strings.TrimSpace(normalizedConfig.AssignedNodeID)
	for _, vdir := range normalizedConfig.VirtualDirectories {
		if vdir == nil {
			continue
		}
		vdir.AssignedNodeID = strings.TrimSpace(vdir.AssignedNodeID)
	}

	return &normalizedConfig, nil
}

func bootstrapNodeSync() error {
	if *mode != "node" {
		return nil
	}

	statusFile := getNodeSyncStatusFile()
	syncStatus, err := node.LoadSyncStatus(statusFile)
	if err != nil {
		return err
	}
	syncStatus.Mode = *mode
	syncStatus.PrimaryServer = *nodeServer
	syncStatus.LastAttemptAt = time.Now()
	_ = node.SaveSyncStatus(statusFile, syncStatus)

	nodeClient := node.NewNodeClient(*nodeServer, *nodeToken, *nodeSyncTimeout)
	proxySnapshot, systemSnapshot, accessSnapshot, certificateSnapshot, err := fetchNodeSnapshots(nodeClient)
	if err != nil {
		syncStatus.LastError = err.Error()
		_ = node.SaveSyncStatus(statusFile, syncStatus)
		if syncStatus.HasPreviousSync {
			return nil
		}
		return fmt.Errorf("primary sync unavailable and no previous sync exists: %w", err)
	}

	if err := applyBootstrapNodeSnapshots(systemSnapshot, certificateSnapshot, accessSnapshot, proxySnapshot); err != nil {
		syncStatus.LastError = err.Error()
		_ = node.SaveSyncStatus(statusFile, syncStatus)
		return err
	}

	syncStatus.HasPreviousSync = true
	syncStatus.LastSuccessAt = time.Now()
	syncStatus.LastConfigVersion = proxySnapshot.ConfigVersion
	syncStatus.LastError = ""
	return node.SaveSyncStatus(statusFile, syncStatus)
}

func fetchNodeSnapshots(nodeClient *node.NodeClient) (*node.ProxyConfigSnapshot, *node.SystemSnapshot, *node.AccessSnapshot, *node.CertificateSnapshot, error) {
	proxySnapshot, err := nodeClient.FetchProxyConfigs("/node/api/config")
	if err != nil {
		return nil, nil, nil, nil, err
	}

	systemSnapshot, err := nodeClient.FetchSystemData("/node/api/system")
	if err != nil {
		return nil, nil, nil, nil, err
	}

	accessSnapshot, err := nodeClient.FetchAccessRules("/node/api/access")
	if err != nil {
		return nil, nil, nil, nil, err
	}

	certificateSnapshot, err := nodeClient.FetchCertificates("/node/api/certs")
	if err != nil {
		return nil, nil, nil, nil, err
	}

	if proxySnapshot.ConfigVersion == "" ||
		systemSnapshot.ConfigVersion != proxySnapshot.ConfigVersion ||
		accessSnapshot.ConfigVersion != proxySnapshot.ConfigVersion ||
		certificateSnapshot.ConfigVersion != proxySnapshot.ConfigVersion {
		return nil, nil, nil, nil, fmt.Errorf(
			"inconsistent node snapshot versions: proxy=%s system=%s access=%s certs=%s",
			proxySnapshot.ConfigVersion,
			systemSnapshot.ConfigVersion,
			accessSnapshot.ConfigVersion,
			certificateSnapshot.ConfigVersion,
		)
	}

	return proxySnapshot, systemSnapshot, accessSnapshot, certificateSnapshot, nil
}

func applyBootstrapNodeSnapshots(systemSnapshot *node.SystemSnapshot, certificateSnapshot *node.CertificateSnapshot, accessSnapshot *node.AccessSnapshot, proxySnapshot *node.ProxyConfigSnapshot) error {
	if err := importBootstrapSystemData(systemSnapshot); err != nil {
		return err
	}
	if err := importBootstrapCertificates(certificateSnapshot); err != nil {
		return err
	}
	if err := importBootstrapAccessRules(accessSnapshot); err != nil {
		return err
	}
	return importBootstrapProxyConfigs(proxySnapshot)
}

func importBootstrapSystemData(snapshot *node.SystemSnapshot) error {
	if snapshot == nil {
		return errors.New("system snapshot is empty")
	}
	if sysdb == nil {
		return errors.New("system database is not initialized")
	}

	preservedLocalSettings := capturePreservedLocalNodeSettings()

	if len(snapshot.DatabaseBackup) > 0 {
		if err := sysdb.RestoreReplacePreservingTables(snapshot.DatabaseBackup, getNodeLocalPreservedTables()); err != nil {
			return err
		}
	}
	if err := restorePreservedLocalNodeSettings(preservedLocalSettings); err != nil {
		return err
	}

	if err := writeBootstrapRedirectRules(snapshot.RedirectRules); err != nil {
		return err
	}

	if err := writeBootstrapStreamConfigs(snapshot.StreamConfigs); err != nil {
		return err
	}

	if err := setDesiredLocalNodeServiceState(snapshot.ServiceEnabled); err != nil {
		return err
	}

	return nil
}

func writeBootstrapRedirectRules(rawRules []json.RawMessage) error {
	if err := os.MkdirAll(CONF_REDIRECTION, 0775); err != nil {
		return err
	}

	configFiles, err := filepath.Glob(filepath.Join(CONF_REDIRECTION, "*.json"))
	if err != nil {
		return err
	}
	for _, configFile := range configFiles {
		if err := os.Remove(configFile); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	redirectRules := make([]*redirection.RedirectRules, 0, len(rawRules))
	for _, rawRule := range rawRules {
		if len(rawRule) == 0 {
			continue
		}

		decodedRule := &redirection.RedirectRules{}
		if err := json.Unmarshal(rawRule, decodedRule); err != nil {
			return err
		}
		redirectRules = append(redirectRules, decodedRule)
	}

	sort.Slice(redirectRules, func(i, j int) bool {
		return redirectRules[i].RedirectURL < redirectRules[j].RedirectURL
	})

	for _, rule := range redirectRules {
		content, err := json.Marshal(rule)
		if err != nil {
			return err
		}

		filename := filepath.Join(CONF_REDIRECTION, utils.ReplaceSpecialCharacters(rule.RedirectURL)+".json")
		if err := os.WriteFile(filename, content, 0775); err != nil {
			return err
		}
	}

	return nil
}

func writeBootstrapStreamConfigs(rawConfigs []json.RawMessage) error {
	if err := os.MkdirAll(CONF_STREAM_PROXY, 0775); err != nil {
		return err
	}

	configFiles, err := filepath.Glob(filepath.Join(CONF_STREAM_PROXY, "*.config"))
	if err != nil {
		return err
	}
	for _, configFile := range configFiles {
		if err := os.Remove(configFile); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	streamConfigs := make([]*streamproxy.ProxyRelayInstance, 0, len(rawConfigs))
	for _, rawConfig := range rawConfigs {
		if len(rawConfig) == 0 {
			continue
		}

		decodedConfig := &streamproxy.ProxyRelayInstance{}
		if err := json.Unmarshal(rawConfig, decodedConfig); err != nil {
			return err
		}
		streamConfigs = append(streamConfigs, decodedConfig)
	}

	sort.Slice(streamConfigs, func(i, j int) bool {
		return streamConfigs[i].UUID < streamConfigs[j].UUID
	})

	for _, config := range streamConfigs {
		content, err := json.Marshal(config)
		if err != nil {
			return err
		}

		if err := os.WriteFile(filepath.Join(CONF_STREAM_PROXY, config.UUID+".config"), content, 0775); err != nil {
			return err
		}
	}

	return nil
}

func importBootstrapCertificates(snapshot *node.CertificateSnapshot) error {
	if snapshot == nil {
		return errors.New("certificate snapshot is empty")
	}

	return tlscert.ReplaceStoredCertificatesAtPath(CONF_CERT_STORE, snapshot.Certificates)
}

func importBootstrapAccessRules(snapshot *node.AccessSnapshot) error {
	if snapshot == nil {
		return errors.New("access snapshot is empty")
	}

	return access.ReplaceAccessRulesOnDisk(CONF_ACCESS_RULE, CONF_TRUSTED_PROXIES, snapshot.AccessRules, snapshot.TrustedProxies)
}

func importBootstrapProxyConfigs(snapshot *node.ProxyConfigSnapshot) error {
	if snapshot == nil {
		return errors.New("proxy configuration snapshot is empty")
	}

	rootConfig, proxyConfigs, err := decodeNodeProxySnapshot(snapshot)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(CONF_HTTP_PROXY, 0775); err != nil {
		return err
	}

	configFiles, err := filepath.Glob(filepath.Join(CONF_HTTP_PROXY, "*.config"))
	if err != nil {
		return err
	}
	for _, configFile := range configFiles {
		if err := os.Remove(configFile); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	if rootConfig != nil {
		if err := SaveReverseProxyConfig(rootConfig); err != nil {
			return err
		}
	}

	for _, proxyConfig := range proxyConfigs {
		if err := SaveReverseProxyConfig(proxyConfig); err != nil {
			return err
		}
	}

	return nil
}

func getNodeSyncStatusFile() string {
	return filepath.Join(CONF_NODES, "sync.status.json")
}

type nodeSyncStatusResponse struct {
	Mode                 string `json:"mode"`
	State                string `json:"state"`
	PrimaryServer        string `json:"primary_server,omitempty"`
	HasPreviousSync      bool   `json:"has_previous_sync"`
	LastAttemptAt        string `json:"last_attempt_at,omitempty"`
	LastSuccessAt        string `json:"last_success_at,omitempty"`
	LastConfigVersion    string `json:"last_config_version,omitempty"`
	LastError            string `json:"last_error,omitempty"`
	ACMEManagedByPrimary bool   `json:"acme_managed_by_primary"`
	ACMEMessage          string `json:"acme_message,omitempty"`
	LocalOverride        bool   `json:"local_override"`
	ConfigWriteUnlocked  bool   `json:"config_write_unlocked"`
	SyncIntervalSeconds  int    `json:"sync_interval_seconds"`
}

func buildNodeSyncStatusResponse() nodeSyncStatusResponse {
	status := &node.SyncStatus{
		Mode:          *mode,
		PrimaryServer: *nodeServer,
	}
	if nodeManager != nil {
		status = nodeManager.GetSyncStatus()
		if status.Mode == "" {
			status.Mode = *mode
		}
		if status.PrimaryServer == "" {
			status.PrimaryServer = *nodeServer
		}
	}

	response := nodeSyncStatusResponse{
		Mode:                 status.Mode,
		State:                resolveNodeSyncState(status),
		PrimaryServer:        status.PrimaryServer,
		HasPreviousSync:      status.HasPreviousSync,
		LastAttemptAt:        formatNodeSyncTime(status.LastAttemptAt),
		LastSuccessAt:        formatNodeSyncTime(status.LastSuccessAt),
		LastConfigVersion:    status.LastConfigVersion,
		LastError:            status.LastError,
		ACMEManagedByPrimary: isLocalNodeManagedByPrimary(),
		ConfigWriteUnlocked:  getLocalNodeConfigWriteUnlocked(),
		LocalOverride:        status.LocalOverride,
		SyncIntervalSeconds:  getConfiguredNodeSyncIntervalSeconds(),
	}
	if response.Mode == "node" && response.ConfigWriteUnlocked {
		response.LocalOverride = true
		response.State = "local_override"
	}
	if acmeAutoRenewer != nil && acmeAutoRenewer.IsDisabled() {
		response.ACMEMessage = acmeAutoRenewer.GetDisableReason()
	} else if isLocalNodeManagedByPrimary() {
		response.ACMEMessage = getLocalNodeManagedACMEMessage()
	}

	return response
}

func HandleNodeSyncStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := buildNodeSyncStatusResponse()

	js, err := json.Marshal(response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	utils.SendJSONResponse(w, string(js))
}

func HandleNodeSyncNow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if *mode != "node" {
		utils.SendErrorResponse(w, "Manual sync is only available in node mode")
		return
	}

	if nodeManager == nil {
		utils.SendErrorResponse(w, "Node manager is not available")
		return
	}

	if err := nodeManager.SyncNow(); err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	response := buildNodeSyncStatusResponse()
	js, err := json.Marshal(response)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	utils.SendJSONResponse(w, string(js))
}

func resolveNodeSyncState(status *node.SyncStatus) string {
	if status == nil || status.Mode == "" {
		return "primary"
	}
	if status.Mode != "node" {
		return "primary"
	}
	if status.LocalOverride {
		return "local_override"
	}
	if isPrimaryUnreachableSyncError(status.LastError) {
		return "primary_unreachable"
	}
	if status.LastError != "" && status.HasPreviousSync {
		return "degraded"
	}
	if status.LastError != "" {
		return "error"
	}
	if status.HasPreviousSync {
		return "synced"
	}
	return "pending"
}

func isPrimaryUnreachableSyncError(message string) bool {
	normalized := strings.ToLower(strings.TrimSpace(message))
	if normalized == "" {
		return false
	}

	indicators := []string{
		"connection refused",
		"connection reset by peer",
		"context deadline exceeded",
		"dial tcp",
		"i/o timeout",
		"lookup ",
		"network is unreachable",
		"no route to host",
		"no such host",
		"tls handshake timeout",
	}

	for _, indicator := range indicators {
		if strings.Contains(normalized, indicator) {
			return true
		}
	}

	return false
}

func formatNodeSyncTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}

	return value.Format(time.RFC3339)
}
