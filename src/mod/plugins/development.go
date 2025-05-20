package plugins

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"imuslab.com/zoraxy/mod/utils"
)

// StartHotReloadTicker starts the hot reload ticker
func (m *Manager) StartHotReloadTicker() error {
	if m.pluginReloadTicker != nil {
		m.Options.Logger.PrintAndLog("plugin-manager", "Hot reload ticker already started", nil)
		return errors.New("hot reload ticker already started")
	}

	m.pluginReloadTicker = time.NewTicker(time.Duration(m.Options.HotReloadInterval) * time.Second)
	m.pluginReloadStop = make(chan bool)
	go func() {
		for {
			select {
			case <-m.pluginReloadTicker.C:
				err := m.UpdatePluginHashList(false)
				if err != nil {
					m.Options.Logger.PrintAndLog("plugin-manager", "Failed to update plugin hash list", err)
				}
			case <-m.pluginReloadStop:
				return
			}
		}
	}()
	m.Options.Logger.PrintAndLog("plugin-manager", "Hot reload ticker started", nil)
	return nil

}

// StopHotReloadTicker stops the hot reload ticker
func (m *Manager) StopHotReloadTicker() error {
	if m.pluginReloadTicker != nil {
		m.pluginReloadStop <- true
		m.pluginReloadTicker.Stop()
		m.pluginReloadTicker = nil
		m.pluginReloadStop = nil
		m.Options.Logger.PrintAndLog("plugin-manager", "Hot reload ticker stopped", nil)
	} else {
		m.Options.Logger.PrintAndLog("plugin-manager", "Hot reload ticker already stopped", nil)
	}
	return nil
}

func (m *Manager) InitPluginHashList() error {
	return m.UpdatePluginHashList(true)
}

// Update the plugin hash list and if there are change, reload the plugin
func (m *Manager) UpdatePluginHashList(noReload bool) error {
	for pluginId, plugin := range m.LoadedPlugins {
		//Get the plugin Entry point
		pluginEntryPoint, err := m.GetPluginEntryPoint(plugin.RootDir)
		if err != nil {
			//Unable to get the entry point of the plugin
			return err
		}

		file, err := os.Open(pluginEntryPoint)
		if err != nil {
			m.Options.Logger.PrintAndLog("plugin-manager", "Failed to open plugin entry point: "+pluginEntryPoint, err)
			return err
		}
		defer file.Close()

		//Calculate the hash of the file
		hasher := sha256.New()
		if _, err := file.Seek(0, 0); err != nil {
			m.Options.Logger.PrintAndLog("plugin-manager", "Failed to seek plugin entry point: "+pluginEntryPoint, err)
			return err
		}
		if _, err := io.Copy(hasher, file); err != nil {
			m.Options.Logger.PrintAndLog("plugin-manager", "Failed to copy plugin entry point: "+pluginEntryPoint, err)
			return err
		}
		hash := hex.EncodeToString(hasher.Sum(nil))
		m.pluginCheckMutex.Lock()
		if m.PluginHash[pluginId] != hash {
			m.PluginHash[pluginId] = hash
			m.pluginCheckMutex.Unlock()
			if !noReload {
				//Plugin file changed, reload the plugin
				m.Options.Logger.PrintAndLog("plugin-manager", "Plugin file changed, reloading plugin: "+pluginId, nil)
				err := m.HotReloadPlugin(pluginId)
				if err != nil {
					m.Options.Logger.PrintAndLog("plugin-manager", "Failed to reload plugin: "+pluginId, err)
					return err
				} else {
					m.Options.Logger.PrintAndLog("plugin-manager", "Plugin reloaded: "+pluginId, nil)
				}
			} else {
				m.Options.Logger.PrintAndLog("plugin-manager", "Plugin hash generated for: "+pluginId, nil)
			}
		} else {
			m.pluginCheckMutex.Unlock()
		}

	}
	return nil
}

// Reload the plugin from file system
func (m *Manager) HotReloadPlugin(pluginId string) error {
	//Check if the plugin is currently running
	thisPlugin, err := m.GetPluginByID(pluginId)
	if err != nil {
		return err
	}

	if thisPlugin.IsRunning() {
		err = m.StopPlugin(pluginId)
		if err != nil {
			return err
		}
	}

	//Remove the plugin from the loaded plugins list
	m.loadedPluginsMutex.Lock()
	if _, ok := m.LoadedPlugins[pluginId]; ok {
		delete(m.LoadedPlugins, pluginId)
	} else {
		m.loadedPluginsMutex.Unlock()
		return nil
	}
	m.loadedPluginsMutex.Unlock()

	//Reload the plugin from disk, it should reload the plugin from latest version
	m.ReloadPluginFromDisk()

	return nil
}

/*
Request handlers for developer options
*/
func (m *Manager) HandleEnableHotReload(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		//Return the current status of hot reload
		js, _ := json.Marshal(m.Options.EnableHotReload)
		utils.SendJSONResponse(w, string(js))
		return
	}

	enabled, err := utils.PostBool(r, "enabled")
	if err != nil {
		utils.SendErrorResponse(w, "enabled not found")
		return
	}
	m.Options.EnableHotReload = enabled
	if enabled {
		//Start the hot reload ticker
		err := m.StartHotReloadTicker()
		if err != nil {
			m.Options.Logger.PrintAndLog("plugin-manager", "Failed to start hot reload ticker", err)
			utils.SendErrorResponse(w, "Failed to start hot reload ticker")
			return
		}
		m.Options.Logger.PrintAndLog("plugin-manager", "Hot reload enabled", nil)
	} else {
		//Stop the hot reload ticker
		err := m.StopHotReloadTicker()
		if err != nil {
			m.Options.Logger.PrintAndLog("plugin-manager", "Failed to stop hot reload ticker", err)
			utils.SendErrorResponse(w, "Failed to stop hot reload ticker")
			return
		}
		m.Options.Logger.PrintAndLog("plugin-manager", "Hot reload disabled", nil)
	}
	utils.SendOK(w)
}

func (m *Manager) HandleSetHotReloadInterval(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		//Return the current status of hot reload
		js, _ := json.Marshal(m.Options.HotReloadInterval)
		utils.SendJSONResponse(w, string(js))
		return
	}

	interval, err := utils.PostInt(r, "interval")
	if err != nil {
		utils.SendErrorResponse(w, "interval not found")
		return
	}

	if interval < 1 {
		utils.SendErrorResponse(w, "interval must be at least 1 second")
		return
	}
	m.Options.HotReloadInterval = interval

	//Restart the hot reload ticker
	if m.pluginReloadTicker != nil {
		m.StopHotReloadTicker()
		time.Sleep(1 * time.Second)
		//Start the hot reload ticker again
		m.StartHotReloadTicker()
	}
	m.Options.Logger.PrintAndLog("plugin-manager", "Hot reload interval set to "+strconv.Itoa(interval)+" sec", nil)
	utils.SendOK(w)
}
