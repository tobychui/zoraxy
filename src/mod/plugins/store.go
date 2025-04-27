package plugins

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"imuslab.com/zoraxy/mod/plugins/zoraxy_plugin"
	"imuslab.com/zoraxy/mod/utils"
)

/*
	Plugin Store
*/

// See https://github.com/aroz-online/zoraxy-official-plugins/blob/main/directories/index.json for the standard format

type Checksums struct {
	LinuxAmd64   string `json:"linux_amd64"`
	Linux386     string `json:"linux_386"`
	LinuxArm     string `json:"linux_arm"`
	LinuxArm64   string `json:"linux_arm64"`
	LinuxMipsle  string `json:"linux_mipsle"`
	LinuxRiscv64 string `json:"linux_riscv64"`
	WindowsAmd64 string `json:"windows_amd64"`
}

type DownloadablePlugin struct {
	IconPath         string
	PluginIntroSpect zoraxy_plugin.IntroSpect //Plugin introspect information
	ChecksumsSHA256  Checksums                //Checksums for the plugin binary
	DownloadURLs     map[string]string        //Download URLs for different platforms
}

/* Plugin Store Index List Sync */
//Update the plugin list from the plugin store URLs
func (m *Manager) UpdateDownloadablePluginList() error {
	//Get downloadable plugins from each of the plugin store URLS
	m.Options.DownloadablePluginCache = []*DownloadablePlugin{}
	for _, url := range m.Options.PluginStoreURLs {
		pluginList, err := m.getPluginListFromURL(url)
		if err != nil {
			return fmt.Errorf("failed to get plugin list from %s: %w", url, err)
		}
		m.Options.DownloadablePluginCache = append(m.Options.DownloadablePluginCache, pluginList...)
	}

	m.Options.LastSuccPluginSyncTime = time.Now().Unix()

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

	return pluginList, nil
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

	// Verify the checksum of the downloaded plugin binary
	checksums, err := plugin.ChecksumsSHA256.GetCurrentPlatformChecksum()
	if err == nil {
		if !verifyChecksumForFile(pluginPath, checksums) {
			out.Close()
			return fmt.Errorf("checksum verification failed for plugin binary")
		}
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

// GetCurrentPlatformChecksum returns the checksum for the current platform
func (c *Checksums) GetCurrentPlatformChecksum() (string, error) {
	switch runtime.GOOS {
	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			return c.LinuxAmd64, nil
		case "386":
			return c.Linux386, nil
		case "arm":
			return c.LinuxArm, nil
		case "arm64":
			return c.LinuxArm64, nil
		case "mipsle":
			return c.LinuxMipsle, nil
		case "riscv64":
			return c.LinuxRiscv64, nil
		default:
			return "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
		}
	case "windows":
		switch runtime.GOARCH {
		case "amd64":
			return c.WindowsAmd64, nil
		default:
			return "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
		}
	default:
		return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

// VerifyChecksum verifies the checksum of the downloaded plugin binary.
func verifyChecksumForFile(filePath string, checksum string) bool {
	file, err := os.Open(filePath)
	if err != nil {
		return false
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return false
	}
	calculatedChecksum := fmt.Sprintf("%x", hash.Sum(nil))

	return calculatedChecksum == checksum
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
