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

/* Plugin Groups */
// HandleListPluginGroups handles the request to list all plugin groups
func (m *Manager) HandleListPluginGroups(w http.ResponseWriter, r *http.Request) {
	targetTag, err := utils.GetPara(r, "tag")
	if err != nil {
		//List all tags
		pluginGroups := m.ListPluginGroups()
		js, _ := json.Marshal(pluginGroups)
		utils.SendJSONResponse(w, string(js))
	} else {
		//List the plugins under the tag
		m.tagPluginListMutex.RLock()
		plugins, ok := m.tagPluginList[targetTag]
		m.tagPluginListMutex.RUnlock()
		if !ok {
			//Return empty array
			js, _ := json.Marshal([]string{})
			utils.SendJSONResponse(w, string(js))
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
}

// HandleAddPluginToGroup handles the request to add a plugin to a group
func (m *Manager) HandleAddPluginToGroup(w http.ResponseWriter, r *http.Request) {
	tag, err := utils.PostPara(r, "tag")
	if err != nil {
		utils.SendErrorResponse(w, "tag not found")
		return
	}

	pluginID, err := utils.PostPara(r, "plugin_id")
	if err != nil {
		utils.SendErrorResponse(w, "plugin_id not found")
		return
	}

	//Check if plugin exists
	_, err = m.GetPluginByID(pluginID)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Add the plugin to the group
	err = m.AddPluginToGroup(tag, pluginID)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Save the plugin groups to file
	err = m.SavePluginGroupsToFile()
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Update the radix tree mapping
	m.UpdateTagsToPluginMaps()

	utils.SendOK(w)
}

// HandleRemovePluginFromGroup handles the request to remove a plugin from a group
func (m *Manager) HandleRemovePluginFromGroup(w http.ResponseWriter, r *http.Request) {
	tag, err := utils.PostPara(r, "tag")
	if err != nil {
		utils.SendErrorResponse(w, "tag not found")
		return
	}

	pluginID, err := utils.PostPara(r, "plugin_id")
	if err != nil {
		utils.SendErrorResponse(w, "plugin_id not found")
		return
	}

	//Remove the plugin from the group
	err = m.RemovePluginFromGroup(tag, pluginID)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Save the plugin groups to file
	err = m.SavePluginGroupsToFile()
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Update the radix tree mapping
	m.UpdateTagsToPluginMaps()

	utils.SendOK(w)
}

// HandleRemovePluginGroup handles the request to remove a plugin group
func (m *Manager) HandleRemovePluginGroup(w http.ResponseWriter, r *http.Request) {
	tag, err := utils.PostPara(r, "tag")
	if err != nil {
		utils.SendErrorResponse(w, "tag not found")
		return
	}

	//Remove the plugin group
	err = m.RemovePluginGroup(tag)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Save the plugin groups to file
	err = m.SavePluginGroupsToFile()
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Update the radix tree mapping
	m.UpdateTagsToPluginMaps()

	utils.SendOK(w)
}

/* Plugin APIs */
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

func (m *Manager) HandlePluginInfo(w http.ResponseWriter, r *http.Request) {
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

	js, err := json.Marshal(plugin)
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

/* Plugin Store */
