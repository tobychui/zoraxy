package plugins

import (
	"encoding/json"
	"os"
)

/*
	Plugin Tags

	This file contains the tags that are used to match the plugin tag
	to the one on HTTP proxy rule. Once the tag is matched, the plugin
	will be enabled on that given rule.
*/

// LoadTagPluginMap loads the plugin map into the manager
// This will only load the plugin tags to option.PluginGroups map
// to push the changes to runtime, call UpdateTagsToPluginMaps()
func (m *Manager) LoadPluginGroupsFromConfig() error {
	m.Options.pluginGroupsMutex.RLock()
	defer m.Options.pluginGroupsMutex.RUnlock()

	//Read the config file
	rawConfig, err := os.ReadFile(m.Options.PluginGroupsConfig)
	if err != nil {
		return err
	}

	var config map[string][]string
	err = json.Unmarshal(rawConfig, &config)
	if err != nil {
		return err
	}

	//Reset m.tagPluginList
	m.Options.PluginGroups = config
	return nil
}

// AddPluginToTag adds a plugin to a tag
func (m *Manager) AddPluginToTag(tag string, pluginID string) error {
	m.Options.pluginGroupsMutex.RLock()
	defer m.Options.pluginGroupsMutex.RUnlock()

	//Check if the plugin exists
	_, err := m.GetPluginByID(pluginID)
	if err != nil {
		return err
	}

	//Add to m.Options.PluginGroups
	pluginList, ok := m.Options.PluginGroups[tag]
	if !ok {
		pluginList = []string{}
	}
	pluginList = append(pluginList, pluginID)
	m.Options.PluginGroups[tag] = pluginList

	//Update to runtime
	m.UpdateTagsToPluginMaps()

	//Save to file
	return m.savePluginTagMap()
}

// RemovePluginFromTag removes a plugin from a tag
func (m *Manager) RemovePluginFromTag(tag string, pluginID string) error {
	// Check if the plugin exists in Options.PluginGroups
	m.Options.pluginGroupsMutex.RLock()
	defer m.Options.pluginGroupsMutex.RUnlock()
	pluginList, ok := m.Options.PluginGroups[tag]
	if !ok {
		return nil
	}

	// Remove the plugin from the list
	for i, id := range pluginList {
		if id == pluginID {
			pluginList = append(pluginList[:i], pluginList[i+1:]...)
			break
		}
	}
	m.Options.PluginGroups[tag] = pluginList

	// Update to runtime
	m.UpdateTagsToPluginMaps()

	// Save to file
	return m.savePluginTagMap()
}

// savePluginTagMap saves the plugin tag map to the config file
func (m *Manager) savePluginTagMap() error {
	m.Options.pluginGroupsMutex.RLock()
	defer m.Options.pluginGroupsMutex.RUnlock()

	js, _ := json.Marshal(m.Options.PluginGroups)
	return os.WriteFile(m.Options.PluginGroupsConfig, js, 0644)
}
