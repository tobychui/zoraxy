package plugins

import (
	"bytes"
	"encoding/json"
	"net/http"
	"path/filepath"
	"sort"
	"time"

	"imuslab.com/zoraxy/mod/utils"
)

// HandleListPlugins handles the request to list all loaded plugins
func (m *Manager) HandleListPlugins(w http.ResponseWriter, r *http.Request) {
	plugins, err := m.ListLoadedPlugins()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	//Sort the plugin by its name
	sort.Slice(plugins, func(i, j int) bool {
		return plugins[i].Spec.Name < plugins[j].Spec.Name
	})

	js, err := json.Marshal(plugins)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	utils.SendJSONResponse(w, string(js))
}

func (m *Manager) HandleLoadPluginIcon(w http.ResponseWriter, r *http.Request) {
	pluginID, err := utils.GetPara(r, "plugin_id")
	if err != nil {
		utils.SendErrorResponse(w, "plugin_id not found")
		return
	}

	plugin, err := m.GetPluginByID(pluginID)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Check if the icon.png exists under plugin root directory
	expectedIconPath := filepath.Join(plugin.RootDir, "icon.png")
	if !utils.FileExists(expectedIconPath) {
		http.ServeContent(w, r, "no_img.png", time.Now(), bytes.NewReader(noImg))
		return
	}

	http.ServeFile(w, r, expectedIconPath)
}

func (m *Manager) HandleEnablePlugin(w http.ResponseWriter, r *http.Request) {
	pluginID, err := utils.PostPara(r, "plugin_id")
	if err != nil {
		utils.SendErrorResponse(w, "plugin_id not found")
		return
	}

	err = m.EnablePlugin(pluginID)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

func (m *Manager) HandleDisablePlugin(w http.ResponseWriter, r *http.Request) {
	pluginID, err := utils.PostPara(r, "plugin_id")
	if err != nil {
		utils.SendErrorResponse(w, "plugin_id not found")
		return
	}

	err = m.DisablePlugin(pluginID)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}
