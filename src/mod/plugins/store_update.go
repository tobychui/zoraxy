package plugins

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"imuslab.com/zoraxy/mod/utils"
)

/*
	Plugin Store Update

	Check if an installed plugin has a newer version available on the
	plugin store index, and update the plugin binary in place.
*/

type PluginUpdateInfo struct {
	PluginID         string `json:"pluginID"`         //The ID of the plugin
	Name             string `json:"name"`             //The name of the plugin
	InstalledVersion string `json:"installedVersion"` //The version currently installed
	LatestVersion    string `json:"latestVersion"`    //The latest version available on the plugin store
	UpdateAvailable  bool   `json:"updateAvailable"`  //Whether the store version is newer than the installed one
}

// versionToString formats a plugin version tuple as a string
func versionToString(major int, minor int, patch int) string {
	return strconv.Itoa(major) + "." + strconv.Itoa(minor) + "." + strconv.Itoa(patch)
}

// storeVersionIsNewer compares the store copy against the installed copy of a plugin
func storeVersionIsNewer(storeCopy *DownloadablePlugin, installedCopy *Plugin) bool {
	storeVer := [3]int{storeCopy.PluginIntroSpect.VersionMajor, storeCopy.PluginIntroSpect.VersionMinor, storeCopy.PluginIntroSpect.VersionPatch}
	installedVer := [3]int{installedCopy.Spec.VersionMajor, installedCopy.Spec.VersionMinor, installedCopy.Spec.VersionPatch}
	for i := 0; i < 3; i++ {
		if storeVer[i] != installedVer[i] {
			return storeVer[i] > installedVer[i]
		}
	}
	return false
}

// getDownloadablePluginByID finds a plugin in the downloadable plugin cache by its ID.
// If the same plugin is offered by multiple sources, the one with the highest version wins.
func (m *Manager) getDownloadablePluginByID(pluginID string) *DownloadablePlugin {
	var best *DownloadablePlugin
	for _, p := range m.Options.DownloadablePluginCache {
		if p.PluginIntroSpect.ID != pluginID {
			continue
		}
		if best == nil || downloadableVersionIsNewer(p, best) {
			best = p
		}
	}
	return best
}

// downloadableVersionIsNewer checks if candidate has a higher version than current
func downloadableVersionIsNewer(candidate *DownloadablePlugin, current *DownloadablePlugin) bool {
	candidateVer := [3]int{candidate.PluginIntroSpect.VersionMajor, candidate.PluginIntroSpect.VersionMinor, candidate.PluginIntroSpect.VersionPatch}
	currentVer := [3]int{current.PluginIntroSpect.VersionMajor, current.PluginIntroSpect.VersionMinor, current.PluginIntroSpect.VersionPatch}
	for i := 0; i < 3; i++ {
		if candidateVer[i] != currentVer[i] {
			return candidateVer[i] > currentVer[i]
		}
	}
	return false
}

// GetPluginUpdateInfos returns the update state of all installed plugins
// that are also available on the plugin store
func (m *Manager) GetPluginUpdateInfos() []*PluginUpdateInfo {
	updateInfos := []*PluginUpdateInfo{}
	m.loadedPluginsMutex.RLock()
	defer m.loadedPluginsMutex.RUnlock()
	for pluginID, plugin := range m.LoadedPlugins {
		storeCopy := m.getDownloadablePluginByID(pluginID)
		if storeCopy == nil {
			//Not from the plugin store, cannot check for updates
			continue
		}
		updateInfos = append(updateInfos, &PluginUpdateInfo{
			PluginID:         pluginID,
			Name:             plugin.Spec.Name,
			InstalledVersion: versionToString(plugin.Spec.VersionMajor, plugin.Spec.VersionMinor, plugin.Spec.VersionPatch),
			LatestVersion:    versionToString(storeCopy.PluginIntroSpect.VersionMajor, storeCopy.PluginIntroSpect.VersionMinor, storeCopy.PluginIntroSpect.VersionPatch),
			UpdateAvailable:  storeVersionIsNewer(storeCopy, plugin),
		})
	}
	return updateInfos
}

// UpdatePlugin downloads the latest binary of an installed plugin from the
// plugin store and overwrites the existing resources in the plugin folder.
// If the plugin was running before the update, it is restarted afterwards.
func (m *Manager) UpdatePlugin(pluginID string) error {
	storeCopy := m.getDownloadablePluginByID(pluginID)
	if storeCopy == nil {
		return fmt.Errorf("plugin not found on the plugin store: %s", pluginID)
	}

	installedPlugin, err := m.GetPluginByID(pluginID)
	if err != nil {
		return fmt.Errorf("plugin is not installed: %s", pluginID)
	}

	if !storeVersionIsNewer(storeCopy, installedPlugin) {
		return fmt.Errorf("plugin is already up to date")
	}

	downloadURL, ok := storeCopy.DownloadURLs[runtime.GOOS+"_"+runtime.GOARCH]
	if !ok {
		return fmt.Errorf("no download URL available for the current platform")
	}

	//Locate the plugin binary to overwrite
	entryPoint, err := m.GetPluginEntryPoint(installedPlugin.RootDir)
	if err != nil {
		return fmt.Errorf("failed to locate plugin entry point: %w", err)
	}
	entryBase := filepath.Base(entryPoint)
	if entryBase == "start.sh" || entryBase == "start.bat" {
		return fmt.Errorf("script based plugins cannot be updated automatically")
	}

	//Stop the plugin if it is running
	wasRunning := installedPlugin.IsRunning()
	if wasRunning {
		err = m.StopPlugin(pluginID)
		if err != nil {
			return fmt.Errorf("failed to stop plugin before update: %w", err)
		}
	}

	//Download the new binary to a temporary file next to the current one
	//so a failed download never corrupts the existing installation
	tmpPath := entryPoint + ".download"
	err = downloadFileTo(downloadURL, tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to download plugin update: %w", err)
	}

	//Replace the old binary with the downloaded one
	err = os.Remove(entryPoint)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to remove old plugin binary: %w", err)
	}
	err = os.Rename(tmpPath, entryPoint)
	if err != nil {
		return fmt.Errorf("failed to replace plugin binary: %w", err)
	}
	err = os.Chmod(entryPoint, 0755)
	if err != nil {
		return fmt.Errorf("failed to set executable permissions: %w", err)
	}

	//Update the plugin icon if one is provided by the store
	iconURL := strings.TrimSpace(storeCopy.IconPath)
	if strings.HasPrefix(iconURL, "http://") || strings.HasPrefix(iconURL, "https://") {
		err = downloadFileTo(iconURL, filepath.Join(installedPlugin.RootDir, "icon.png"))
		if err != nil {
			//Icon is not critical, log and continue
			m.Log("Failed to update plugin icon for "+pluginID, err)
		}
	}

	//Reload the plugin spec from the updated binary
	updatedPlugin, err := m.LoadPluginSpec(installedPlugin.RootDir)
	if err != nil {
		return fmt.Errorf("plugin updated but failed to reload plugin spec: %w", err)
	}

	m.loadedPluginsMutex.Lock()
	if updatedPlugin.Spec.ID != pluginID {
		//Plugin changed its ID after the update, re-register under the new ID
		delete(m.LoadedPlugins, pluginID)
	}
	installedPlugin.Spec = updatedPlugin.Spec
	installedPlugin.SupportHotRebuild = updatedPlugin.SupportHotRebuild
	m.LoadedPlugins[updatedPlugin.Spec.ID] = installedPlugin
	m.loadedPluginsMutex.Unlock()

	//Update the plugin hash list to reflect the new binary
	m.InitPluginHashList()

	m.Log("Plugin "+updatedPlugin.Spec.Name+" updated to v"+versionToString(updatedPlugin.Spec.VersionMajor, updatedPlugin.Spec.VersionMinor, updatedPlugin.Spec.VersionPatch), nil)

	//Restart the plugin if it was running before the update
	if wasRunning {
		err = m.StartPlugin(updatedPlugin.Spec.ID)
		if err != nil {
			return fmt.Errorf("plugin updated but failed to restart: %w", err)
		}
	}

	return nil
}

// downloadFileTo downloads the content of the given URL to the given file path
func downloadFileTo(downloadURL string, destPath string) error {
	resp, err := http.Get(downloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

/*
	Handlers for Plugin Update
*/

// HandleCheckPluginUpdates returns the update state of all installed plugins
// that are available on the plugin store
func (m *Manager) HandleCheckPluginUpdates(w http.ResponseWriter, r *http.Request) {
	updateInfos := m.GetPluginUpdateInfos()
	js, _ := json.Marshal(updateInfos)
	utils.SendJSONResponse(w, string(js))
}

// HandleUpdatePlugin updates an installed plugin to the latest version on the plugin store
func (m *Manager) HandleUpdatePlugin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "Method not allowed")
		return
	}

	pluginID, err := utils.PostPara(r, "pluginID")
	if err != nil {
		utils.SendErrorResponse(w, "pluginID is required")
		return
	}

	err = m.UpdatePlugin(pluginID)
	if err != nil {
		utils.SendErrorResponse(w, "Failed to update plugin: "+err.Error())
		return
	}

	utils.SendOK(w)
}
