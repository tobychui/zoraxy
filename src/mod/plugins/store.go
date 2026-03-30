package plugins

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	urlpkg "net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"imuslab.com/zoraxy/mod/eventsystem"
	"imuslab.com/zoraxy/mod/plugins/zoraxy_plugin"
	"imuslab.com/zoraxy/mod/plugins/zoraxy_plugin/events"
	"imuslab.com/zoraxy/mod/utils"
)

/*
	Plugin Store
*/

// See https://github.com/aroz-online/zoraxy-official-plugins/blob/main/directories/index2.json for the standard format

var defaultPluginStoreURLs = []string{
	"https://raw.githubusercontent.com/aroz-online/zoraxy-official-plugins/refs/heads/main/directories/index2.json",
}

type DownloadablePlugin struct {
	IconPath         string                   `json:"IconPath"`         //Icon path or URL for the plugin
	PluginIntroSpect zoraxy_plugin.IntroSpect `json:"PluginIntroSpect"` //Plugin introspect information
	DownloadURLs     map[string]string        `json:"DownloadURLs"`     //Download URLs for different platforms
}

func GetDefaultPluginStoreURLs() []string {
	return append([]string{}, defaultPluginStoreURLs...)
}

func normalizePluginStoreURLs(rawURLs []string) []string {
	seen := map[string]bool{}
	normalized := []string{}
	for _, rawURL := range rawURLs {
		trimmed := strings.TrimSpace(rawURL)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		normalized = append(normalized, trimmed)
	}
	if len(normalized) == 0 {
		return GetDefaultPluginStoreURLs()
	}
	return normalized
}

func (m *Manager) GetPluginStoreURLs() []string {
	return normalizePluginStoreURLs(m.Options.PluginStoreURLs)
}

func (m *Manager) LoadPluginStoreURLs() error {
	if m.Options.Database == nil {
		m.Options.PluginStoreURLs = normalizePluginStoreURLs(m.Options.PluginStoreURLs)
		return nil
	}

	loaded := []string{}
	if err := m.Options.Database.Read("plugins", "store_urls", &loaded); err != nil {
		m.Options.PluginStoreURLs = normalizePluginStoreURLs(m.Options.PluginStoreURLs)
		return nil
	}

	m.Options.PluginStoreURLs = normalizePluginStoreURLs(loaded)
	return nil
}

func (m *Manager) SavePluginStoreURLs(urls []string) error {
	normalized := normalizePluginStoreURLs(urls)
	m.Options.PluginStoreURLs = normalized
	if m.Options.Database == nil {
		return nil
	}
	return m.Options.Database.Write("plugins", "store_urls", normalized)
}

/* Plugin Store Index List Sync */
//Update the plugin list from the plugin store URLs
func (m *Manager) UpdateDownloadablePluginList() error {
	//Get downloadable plugins from each of the plugin store URLS
	m.Options.PluginStoreURLs = normalizePluginStoreURLs(m.Options.PluginStoreURLs)
	combined := []*DownloadablePlugin{}
	seenPluginIDs := map[string]bool{}
	errors := []string{}
	successfulSources := 0
	for _, sourceURL := range m.Options.PluginStoreURLs {
		pluginList, err := m.getPluginListFromURL(sourceURL)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %s", sourceURL, err.Error()))
			continue
		}
		successfulSources++
		for _, plugin := range pluginList {
			if plugin == nil || plugin.PluginIntroSpect.ID == "" {
				continue
			}
			if seenPluginIDs[plugin.PluginIntroSpect.ID] {
				continue
			}
			seenPluginIDs[plugin.PluginIntroSpect.ID] = true
			combined = append(combined, plugin)
		}
	}

	m.Options.DownloadablePluginCache = combined

	if successfulSources > 0 {
		m.Options.LastSuccPluginSyncTime = time.Now().Unix()
	}

	if successfulSources == 0 && len(errors) > 0 {
		return fmt.Errorf("failed to sync any plugin repository: %s", strings.Join(errors, "; "))
	}

	return nil
}

// Get the plugin list from the URL
func (m *Manager) getPluginListFromURL(url string) ([]*DownloadablePlugin, error) {
	//Get the plugin list from the URL
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get plugin list from %s: %s", url, resp.Status)
	}

	var pluginList []*DownloadablePlugin
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read plugin list from %s: %w", url, err)
	}
	content = []byte(strings.TrimSpace(string(content)))

	err = json.Unmarshal(content, &pluginList)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal plugin list from %s: %w", url, err)
	}

	baseURL, _ := urlpkg.Parse(url)

	// Filter out if IconPath is empty string, set it to "/img/plugin_icon.png"
	for _, plugin := range pluginList {
		if strings.TrimSpace(plugin.IconPath) == "" {
			plugin.IconPath = "/img/plugin_icon.png"
		} else if resolvedURL := resolvePluginStoreAssetURL(baseURL, plugin.IconPath); resolvedURL != "" {
			plugin.IconPath = resolvedURL
		}

		for platform, downloadURL := range plugin.DownloadURLs {
			if resolvedURL := resolvePluginStoreAssetURL(baseURL, downloadURL); resolvedURL != "" {
				plugin.DownloadURLs[platform] = resolvedURL
			}
		}
	}

	return pluginList, nil
}

func resolvePluginStoreAssetURL(baseURL *urlpkg.URL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	parsed, err := urlpkg.Parse(raw)
	if err != nil {
		return raw
	}
	if parsed.IsAbs() || baseURL == nil {
		return raw
	}
	return baseURL.ResolveReference(parsed).String()
}

func (m *Manager) ListDownloadablePlugins() []*DownloadablePlugin {
	//List all downloadable plugins
	if len(m.Options.DownloadablePluginCache) == 0 {
		return []*DownloadablePlugin{}
	}
	return m.Options.DownloadablePluginCache
}

// InstallPlugin installs the given plugin by moving it to the PluginDir.
func (m *Manager) InstallPlugin(plugin *DownloadablePlugin) error {
	pluginDir := filepath.Join(m.Options.PluginDir, plugin.PluginIntroSpect.Name)
	pluginFile := plugin.PluginIntroSpect.Name
	if runtime.GOOS == "windows" {
		pluginFile += ".exe"
	}

	//Check if the plugin id already exists in runtime plugin map
	if _, ok := m.LoadedPlugins[plugin.PluginIntroSpect.ID]; ok {
		return fmt.Errorf("plugin already installed: %s", plugin.PluginIntroSpect.ID)
	}

	// Create the plugin directory if it doesn't exist
	err := os.MkdirAll(pluginDir, os.ModePerm)
	if err != nil {
		return fmt.Errorf("failed to create plugin directory: %w", err)
	}

	// Download the plugin binary
	downloadURL, ok := plugin.DownloadURLs[runtime.GOOS+"_"+runtime.GOARCH]
	if !ok {
		return fmt.Errorf("no download URL available for the current platform")
	}

	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download plugin: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download plugin: %s", resp.Status)
	}

	// Write the plugin binary to the plugin directory
	pluginPath := filepath.Join(pluginDir, pluginFile)
	out, err := os.Create(pluginPath)
	if err != nil {
		return fmt.Errorf("failed to create plugin file: %w", err)
	}

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		out.Close()
		return fmt.Errorf("failed to write plugin file: %w", err)
	}

	// Make the plugin executable
	err = os.Chmod(pluginPath, 0755)
	if err != nil {
		out.Close()
		return fmt.Errorf("failed to set executable permissions: %w", err)
	}

	//Ok, also download the icon if exists
	if plugin.IconPath != "" {
		iconURL := strings.TrimSpace(plugin.IconPath)
		if iconURL != "" {
			resp, err := http.Get(iconURL)
			if err != nil {
				return fmt.Errorf("failed to download plugin icon: %w", err)
			}
			defer resp.Body.Close()

			//Save the icon to the plugin directory
			iconPath := filepath.Join(pluginDir, "icon.png")
			out, err := os.Create(iconPath)
			if err != nil {
				return fmt.Errorf("failed to create plugin icon file: %w", err)
			}
			defer out.Close()

			io.Copy(out, resp.Body)
		}
	}
	//Close the plugin exeutable
	out.Close()

	//Reload the plugin list
	m.ReloadPluginFromDisk()

	return nil
}

// UninstallPlugin uninstalls the plugin by removing its directory.
func (m *Manager) UninstallPlugin(pluginID string) error {

	//Stop the plugin process if it's running
	plugin, ok := m.LoadedPlugins[pluginID]
	if !ok {
		return fmt.Errorf("plugin not found: %s", pluginID)
	}

	if plugin.IsRunning() {
		err := m.StopPlugin(plugin.Spec.ID)
		if err != nil {
			return fmt.Errorf("failed to stop plugin: %w", err)
		}
	}

	//Make sure the plugin process is stopped
	m.Options.Logger.PrintAndLog("plugin-manager", "Removing plugin in 3 seconds...", nil)
	time.Sleep(3 * time.Second)

	// Remove the plugin directory
	err := os.RemoveAll(plugin.RootDir)
	if err != nil {
		return fmt.Errorf("failed to remove plugin directory: %w", err)
	}

	//Reload the plugin list
	m.ReloadPluginFromDisk()
	return nil
}

/*
	Handlers for Plugin Store
*/

func (m *Manager) HandleListDownloadablePlugins(w http.ResponseWriter, r *http.Request) {
	//List all downloadable plugins
	plugins := m.ListDownloadablePlugins()
	js, _ := json.Marshal(plugins)
	utils.SendJSONResponse(w, string(js))
}

func (m *Manager) HandlePluginStoreURLs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		payload := map[string]any{
			"urls":               m.GetPluginStoreURLs(),
			"default_urls":       GetDefaultPluginStoreURLs(),
			"last_sync_unix":     m.Options.LastSuccPluginSyncTime,
			"repository_count":   len(m.GetPluginStoreURLs()),
			"downloadable_count": len(m.Options.DownloadablePluginCache),
		}
		js, _ := json.Marshal(payload)
		utils.SendJSONResponse(w, string(js))
	case http.MethodPost:
		rawURLs, err := utils.PostPara(r, "urls")
		if err != nil && !strings.Contains(err.Error(), "invalid urls") {
			utils.SendErrorResponse(w, "urls not found")
			return
		}
		urls := []string{}
		if strings.TrimSpace(rawURLs) != "" {
			for _, line := range strings.Split(rawURLs, "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					urls = append(urls, line)
				}
			}
		}
		if err := m.SavePluginStoreURLs(urls); err != nil {
			utils.SendErrorResponse(w, "failed to save plugin store URLs: "+err.Error())
			return
		}
		utils.SendOK(w)
	default:
		utils.SendErrorResponse(w, "Method not allowed")
	}
}

// HandleResyncPluginList is the handler for resyncing the plugin list from the plugin store URLs
func (m *Manager) HandleResyncPluginList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		//Make sure this function require csrf token
		utils.SendErrorResponse(w, "Method not allowed")
		return
	}

	//Resync the plugin list from the plugin store URLs
	err := m.UpdateDownloadablePluginList()
	if err != nil {
		utils.SendErrorResponse(w, "Failed to resync plugin list: "+err.Error())
		return
	}
	utils.SendOK(w)
}

// HandleInstallPlugin is the handler for installing a plugin
func (m *Manager) HandleInstallPlugin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "Method not allowed")
		return
	}

	pluginID, err := utils.PostPara(r, "pluginID")
	if err != nil {
		utils.SendErrorResponse(w, "pluginID is required")
		return
	}

	// Find the plugin info from cache
	var plugin *DownloadablePlugin
	for _, p := range m.Options.DownloadablePluginCache {
		if p.PluginIntroSpect.ID == pluginID {
			plugin = p
			break
		}
	}

	if plugin == nil {
		utils.SendErrorResponse(w, "Plugin not found")
		return
	}

	// Install the plugin (implementation depends on your system)
	err = m.InstallPlugin(plugin)
	if err != nil {
		utils.SendErrorResponse(w, "Failed to install plugin: "+err.Error())
		return
	}

	utils.SendOK(w)
}

// HandleUninstallPlugin is the handler for uninstalling a plugin
func (m *Manager) HandleUninstallPlugin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "Method not allowed")
		return
	}

	pluginID, err := utils.PostPara(r, "pluginID")
	if err != nil {
		utils.SendErrorResponse(w, "pluginID is required")
		return
	}

	// Uninstall the plugin (implementation depends on your system)
	err = m.UninstallPlugin(pluginID)
	if err != nil {
		utils.SendErrorResponse(w, "Failed to uninstall plugin: "+err.Error())
		return
	}

	utils.SendOK(w)
}

// HandleEmitCustomEvent is the handler for emitting a custom event from a plugin to other plugins
func (m *Manager) HandleEmitCustomEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "Method not allowed")
		return
	}

	contentType := r.Header.Get("Content-Type")
	if contentType == "" || !strings.HasPrefix(strings.ToLower(contentType), "application/json") {
		utils.SendErrorResponse(w, "Invalid or missing Content-Type, expected application/json")
		return
	}

	// parse the event payload from the request body
	var payload events.CustomEvent
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		utils.SendErrorResponse(w, "Failed to parse event: "+err.Error())
		return
	}

	// collect the recipients
	if len(payload.Recipients) > 0 {
		recipients := make([]eventsystem.ListenerID, 0, len(payload.Recipients))
		for _, rid := range payload.Recipients {
			recipients = append(recipients, eventsystem.ListenerID(rid))
		}
		// Emit the event to subscribers and specified recipients
		eventsystem.Publisher.EmitToSubscribersAnd(recipients, &payload)
	} else {
		// Emit the event to all subscribers
		eventsystem.Publisher.Emit(&payload)
	}

	utils.SendOK(w)
}
