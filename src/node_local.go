package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"imuslab.com/zoraxy/mod/utils"
)

const (
	localNodeDisplayNameSettingKey    = "node_display_name"
	localNodeServiceEnabledSettingKey = "node_service_enabled"
	localNodeConfigWriteUnlockedKey   = "node_config_write_unlocked"
	nodeSyncIntervalSettingKey        = "node_sync_interval_seconds"
	nodeOfflineThresholdSettingKey    = "node_offline_threshold_seconds"
	nodeRequireVersionMatchSettingKey = "node_require_version_match"
	nodeDockerImageSettingKey         = "node_docker_image"
)

func getStoredLocalNodeName() string {
	if sysdb == nil || !sysdb.KeyExists("settings", localNodeDisplayNameSettingKey) {
		return ""
	}

	name := ""
	if err := sysdb.Read("settings", localNodeDisplayNameSettingKey, &name); err != nil {
		return ""
	}

	return strings.TrimSpace(name)
}

func getEffectiveLocalNodeName() string {
	if name := getStoredLocalNodeName(); name != "" {
		return name
	}

	if envNodeName := strings.TrimSpace(os.Getenv("ZORAXY_NODE_NAME")); envNodeName != "" {
		return envNodeName
	}

	hostName, err := os.Hostname()
	if err == nil && strings.TrimSpace(hostName) != "" {
		return strings.TrimSpace(hostName)
	}

	return nodeUUID
}

func getDefaultLocalNodeLabel() string {
	return "Local"
}

func setStoredLocalNodeName(name string) error {
	if sysdb == nil {
		return nil
	}

	name = strings.TrimSpace(name)
	if name == "" {
		if sysdb.KeyExists("settings", localNodeDisplayNameSettingKey) {
			return sysdb.Delete("settings", localNodeDisplayNameSettingKey)
		}
		return nil
	}

	return sysdb.Write("settings", localNodeDisplayNameSettingKey, name)
}

func getLocalNodeConfigWriteUnlocked() bool {
	if sysdb == nil || !sysdb.KeyExists("settings", localNodeConfigWriteUnlockedKey) {
		return false
	}

	unlocked := false
	if err := sysdb.Read("settings", localNodeConfigWriteUnlockedKey, &unlocked); err != nil {
		return false
	}

	return unlocked
}

func setLocalNodeConfigWriteUnlocked(enabled bool) error {
	if sysdb == nil {
		return nil
	}

	return sysdb.Write("settings", localNodeConfigWriteUnlockedKey, enabled)
}

func isLocalNodeConfigWriteAllowed() bool {
	return *mode != "node" || getLocalNodeConfigWriteUnlocked()
}

func requireLocalNodeConfigWriteAllowed(w http.ResponseWriter) bool {
	if isLocalNodeConfigWriteAllowed() {
		return true
	}

	utils.SendErrorResponse(w, "Configuration changes are locked in node mode. Open Status and enable Emergency Local Override to unlock local edits.")
	return false
}

func isLocalNodeManagedByPrimary() bool {
	return *mode == "node" && !getLocalNodeConfigWriteUnlocked()
}

func getLocalNodeManagedACMEMessage() string {
	return "ACME auto renewal is disabled in node mode. Certificates are synchronized from the primary node."
}

func updateLocalNodeManagedRuntimeState() {
	if acmeAutoRenewer == nil {
		return
	}

	if isLocalNodeManagedByPrimary() {
		acmeAutoRenewer.SetRuntimeDisableReason(getLocalNodeManagedACMEMessage())
		return
	}

	acmeAutoRenewer.SetRuntimeDisableReason("")
}

type localNodeStateResponse struct {
	Name                 string `json:"name"`
	ManagementPort       string `json:"management_port"`
	Mode                 string `json:"mode"`
	ConfigWriteUnlocked  bool   `json:"config_write_unlocked"`
	ConfigWriteAllowed   bool   `json:"config_write_allowed"`
	ConfigLocked         bool   `json:"config_locked"`
	SyncIntervalSeconds  int    `json:"sync_interval_seconds"`
	ACMEManagedByPrimary bool   `json:"acme_managed_by_primary"`
	ACMEMessage          string `json:"acme_message,omitempty"`
}

func buildLocalNodeStateResponse() localNodeStateResponse {
	acmeManagedByPrimary := isLocalNodeManagedByPrimary()
	acmeMessage := ""
	if acmeAutoRenewer != nil && acmeAutoRenewer.IsDisabled() {
		acmeMessage = acmeAutoRenewer.GetDisableReason()
	} else if acmeManagedByPrimary {
		acmeMessage = getLocalNodeManagedACMEMessage()
	}

	return localNodeStateResponse{
		Name:                 getEffectiveLocalNodeName(),
		ManagementPort:       getLocalManagementPort(),
		Mode:                 *mode,
		ConfigWriteUnlocked:  getLocalNodeConfigWriteUnlocked(),
		ConfigWriteAllowed:   isLocalNodeConfigWriteAllowed(),
		ConfigLocked:         !isLocalNodeConfigWriteAllowed(),
		SyncIntervalSeconds:  getConfiguredNodeSyncIntervalSeconds(),
		ACMEManagedByPrimary: acmeManagedByPrimary,
		ACMEMessage:          acmeMessage,
	}
}

func normalizeNodeSyncIntervalSeconds(seconds int) int {
	if seconds < 10 {
		return 10
	}
	if seconds > 3600 {
		return 3600
	}
	return seconds
}

func getConfiguredNodeSyncIntervalSeconds() int {
	defaultInterval := NODE_CONFIG_UPDATE_INTERVAL * 60
	if sysdb == nil || !sysdb.KeyExists("settings", nodeSyncIntervalSettingKey) {
		return normalizeNodeSyncIntervalSeconds(defaultInterval)
	}

	intervalSeconds := defaultInterval
	if err := sysdb.Read("settings", nodeSyncIntervalSettingKey, &intervalSeconds); err != nil {
		return normalizeNodeSyncIntervalSeconds(defaultInterval)
	}

	return normalizeNodeSyncIntervalSeconds(intervalSeconds)
}

func getConfiguredNodeSyncInterval() time.Duration {
	return time.Duration(getConfiguredNodeSyncIntervalSeconds()) * time.Second
}

func setConfiguredNodeSyncIntervalSeconds(seconds int) error {
	if sysdb == nil {
		return nil
	}

	return sysdb.Write("settings", nodeSyncIntervalSettingKey, normalizeNodeSyncIntervalSeconds(seconds))
}

func normalizeNodeOfflineThresholdSeconds(seconds int) int {
	defaultThreshold := (getConfiguredNodeSyncIntervalSeconds() * 2) + 30
	if seconds <= 0 {
		seconds = defaultThreshold
	}
	if seconds < 30 {
		return 30
	}
	if seconds > 86400 {
		return 86400
	}
	return seconds
}

func getConfiguredNodeOfflineThresholdSeconds() int {
	if sysdb == nil || !sysdb.KeyExists("settings", nodeOfflineThresholdSettingKey) {
		return normalizeNodeOfflineThresholdSeconds(0)
	}

	thresholdSeconds := 0
	if err := sysdb.Read("settings", nodeOfflineThresholdSettingKey, &thresholdSeconds); err != nil {
		return normalizeNodeOfflineThresholdSeconds(0)
	}

	return normalizeNodeOfflineThresholdSeconds(thresholdSeconds)
}

func setConfiguredNodeOfflineThresholdSeconds(seconds int) error {
	if sysdb == nil {
		return nil
	}

	return sysdb.Write("settings", nodeOfflineThresholdSettingKey, normalizeNodeOfflineThresholdSeconds(seconds))
}

func getRequireNodeVersionMatch() bool {
	if sysdb == nil || !sysdb.KeyExists("settings", nodeRequireVersionMatchSettingKey) {
		return true
	}

	required := true
	if err := sysdb.Read("settings", nodeRequireVersionMatchSettingKey, &required); err != nil {
		return true
	}

	return required
}

func setRequireNodeVersionMatch(enabled bool) error {
	if sysdb == nil {
		return nil
	}

	return sysdb.Write("settings", nodeRequireVersionMatchSettingKey, enabled)
}

func getStoredNodeDockerImage() string {
	if sysdb == nil || !sysdb.KeyExists("settings", nodeDockerImageSettingKey) {
		return ""
	}

	image := ""
	if err := sysdb.Read("settings", nodeDockerImageSettingKey, &image); err != nil {
		return ""
	}

	return strings.TrimSpace(image)
}

func setStoredNodeDockerImage(image string) error {
	if sysdb == nil {
		return nil
	}

	image = strings.TrimSpace(image)
	if image == "" {
		if sysdb.KeyExists("settings", nodeDockerImageSettingKey) {
			return sysdb.Delete("settings", nodeDockerImageSettingKey)
		}
		return nil
	}

	return sysdb.Write("settings", nodeDockerImageSettingKey, image)
}

func getDefaultNodeDockerImage() (string, string) {
	defaultImage := "zoraxydocker/zoraxy:v" + SYSTEM_VERSION
	if DockerUXOptimizer == nil {
		return defaultImage, "default"
	}

	return DockerUXOptimizer.ResolveSuggestedNodeImage(defaultImage)
}

func getConfiguredNodeDockerImage() (string, string) {
	if image := getStoredNodeDockerImage(); image != "" {
		return image, "configured"
	}

	return getDefaultNodeDockerImage()
}

func getDesiredLocalNodeServiceState() bool {
	if sysdb == nil || !sysdb.KeyExists("settings", localNodeServiceEnabledSettingKey) {
		return true
	}

	enabled := true
	if err := sysdb.Read("settings", localNodeServiceEnabledSettingKey, &enabled); err != nil {
		return true
	}

	return enabled
}

func setDesiredLocalNodeServiceState(enabled bool) error {
	if sysdb == nil {
		return nil
	}

	return sysdb.Write("settings", localNodeServiceEnabledSettingKey, enabled)
}

func applyDesiredLocalNodeServiceState() error {
	if *mode != "node" {
		return nil
	}

	enabled := getDesiredLocalNodeServiceState()

	if dynamicProxyRouter != nil {
		if enabled {
			if dynamicProxyRouter.Root == nil {
				// The node bootstrap applies config after the router is initialized.
			} else if !dynamicProxyRouter.Running {
				if err := dynamicProxyRouter.StartProxyService(); err != nil && !strings.Contains(err.Error(), "already running") {
					return err
				}
			}
		} else if dynamicProxyRouter.Running {
			if err := dynamicProxyRouter.StopProxyService(); err != nil {
				return err
			}
		}
	}

	if streamProxyManager != nil {
		for _, config := range streamProxyManager.Configs {
			if config == nil {
				continue
			}
			if enabled {
				if (config.AutoStart || config.Running) && !config.IsRunning() {
					if err := config.Start(); err == nil && config.AutoStart {
						config.AutoStart = false
						streamProxyManager.SaveConfigToDatabase()
					}
				}
			} else {
				if config.Running || config.IsRunning() {
					config.AutoStart = true
				}
				if config.IsRunning() {
					config.Stop()
				}
			}
		}
	}

	return nil
}

func getLocalManagementPort() string {
	port := strings.TrimSpace(*webUIPort)
	if port == "" {
		return "8000"
	}

	if strings.HasPrefix(port, ":") {
		return strings.TrimPrefix(port, ":")
	}

	if host, parsedPort, err := net.SplitHostPort(port); err == nil {
		_ = host
		return parsedPort
	}

	return port
}

func detectLocalNodeIP() string {
	if nodeIP != nil {
		overrideIP := strings.TrimSpace(*nodeIP)
		if overrideIP != "" {
			return overrideIP
		}
	}

	if nodeServer == nil {
		return ""
	}

	parsedURL, err := url.Parse(strings.TrimSpace(*nodeServer))
	if err != nil || parsedURL.Hostname() == "" {
		return ""
	}

	dialPort := parsedURL.Port()
	if dialPort == "" {
		if strings.EqualFold(parsedURL.Scheme, "https") {
			dialPort = "443"
		} else {
			dialPort = "80"
		}
	}

	conn, err := net.DialTimeout("udp", net.JoinHostPort(parsedURL.Hostname(), dialPort), 3*time.Second)
	if err != nil {
		return ""
	}
	defer conn.Close()

	switch localAddr := conn.LocalAddr().(type) {
	case *net.UDPAddr:
		if localAddr.IP != nil {
			return strings.TrimSpace(localAddr.IP.String())
		}
	}

	return ""
}

func HandleNodeFleetSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.Method == http.MethodPost {
		if *mode != "primary" {
			utils.SendErrorResponse(w, "Node fleet settings can only be changed on the primary node")
			return
		}

		if err := r.ParseForm(); err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}

		if _, exists := r.PostForm["syncIntervalSeconds"]; exists {
			rawInterval := r.PostForm.Get("syncIntervalSeconds")
			intervalSeconds, err := strconv.Atoi(strings.TrimSpace(rawInterval))
			if err != nil {
				utils.SendErrorResponse(w, "invalid sync interval")
				return
			}

			if err := setConfiguredNodeSyncIntervalSeconds(intervalSeconds); err != nil {
				utils.SendErrorResponse(w, err.Error())
				return
			}
			if nodeManager != nil {
				nodeManager.SetUpdateInterval(getConfiguredNodeSyncInterval())
			}
		}

		if _, exists := r.PostForm["requireVersionMatch"]; exists {
			requireVersionMatch, err := strconv.ParseBool(strings.TrimSpace(r.PostForm.Get("requireVersionMatch")))
			if err != nil {
				utils.SendErrorResponse(w, fmt.Sprintf("invalid requireVersionMatch value: %v", err))
				return
			}
			if err := setRequireNodeVersionMatch(requireVersionMatch); err != nil {
				utils.SendErrorResponse(w, err.Error())
				return
			}
		}

		if _, exists := r.PostForm["offlineThresholdSeconds"]; exists {
			rawThreshold := r.PostForm.Get("offlineThresholdSeconds")
			thresholdSeconds, err := strconv.Atoi(strings.TrimSpace(rawThreshold))
			if err != nil {
				utils.SendErrorResponse(w, "invalid offline threshold")
				return
			}
			if err := setConfiguredNodeOfflineThresholdSeconds(thresholdSeconds); err != nil {
				utils.SendErrorResponse(w, err.Error())
				return
			}
		}

		if _, exists := r.PostForm["dockerImage"]; exists {
			if err := setStoredNodeDockerImage(r.PostForm.Get("dockerImage")); err != nil {
				utils.SendErrorResponse(w, err.Error())
				return
			}
		}
	}

	dockerImage, dockerImageSource := getConfiguredNodeDockerImage()

	response, err := json.Marshal(struct {
		Mode                    string `json:"mode"`
		SyncIntervalSeconds     int    `json:"sync_interval_seconds"`
		OfflineThresholdSeconds int    `json:"offline_threshold_seconds"`
		RequireVersionMatch     bool   `json:"require_version_match"`
		DockerImage             string `json:"docker_image"`
		DockerImageSource       string `json:"docker_image_source"`
	}{
		Mode:                    *mode,
		SyncIntervalSeconds:     getConfiguredNodeSyncIntervalSeconds(),
		OfflineThresholdSeconds: getConfiguredNodeOfflineThresholdSeconds(),
		RequireVersionMatch:     getRequireNodeVersionMatch(),
		DockerImage:             dockerImage,
		DockerImageSource:       dockerImageSource,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	utils.SendJSONResponse(w, string(response))
}

func HandleLocalNodeState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response, err := json.Marshal(buildLocalNodeStateResponse())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	utils.SendJSONResponse(w, string(response))
}

func HandleLocalNodeName(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// pass
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}

		nameUpdated := false
		overrideUpdated := false
		if _, exists := r.PostForm["name"]; exists {
			if err := setStoredLocalNodeName(r.PostForm.Get("name")); err != nil {
				utils.SendErrorResponse(w, err.Error())
				return
			}
			nameUpdated = true
		}

		if _, exists := r.PostForm["configWriteUnlocked"]; exists {
			currentUnlocked := getLocalNodeConfigWriteUnlocked()
			unlocked, err := strconv.ParseBool(strings.TrimSpace(r.PostForm.Get("configWriteUnlocked")))
			if err != nil {
				utils.SendErrorResponse(w, fmt.Sprintf("invalid configWriteUnlocked value: %v", err))
				return
			}
			if err := setLocalNodeConfigWriteUnlocked(unlocked); err != nil {
				utils.SendErrorResponse(w, err.Error())
				return
			}
			overrideUpdated = currentUnlocked != unlocked
		}

		updateLocalNodeManagedRuntimeState()

		if *mode == "node" && nodeManager != nil && (nameUpdated || overrideUpdated) {
			go func() {
				_ = nodeManager.SyncNow()
			}()
		}
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response, err := json.Marshal(buildLocalNodeStateResponse())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	utils.SendJSONResponse(w, string(response))
}
