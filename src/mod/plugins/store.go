package plugins

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
