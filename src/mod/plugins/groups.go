package plugins

import (
	"encoding/json"
	"errors"
	"os"

	"imuslab.com/zoraxy/mod/plugins/zoraxy_plugin"
)

// ListPluginGroups returns a map of plugin groups
func (m *Manager) ListPluginGroups() map[string][]string {
	pluginGroup := map[string][]string{}
	m.pluginGroupsMutex.RLock()
	for k, v := range m.Options.PluginGroups {
		pluginGroup[k] = append([]string{}, v...)
	}
	m.pluginGroupsMutex.RUnlock()
	return pluginGroup
}

// AddPluginToGroup adds a plugin to a group
func (m *Manager) AddPluginToGroup(tag, pluginID string) error {
	//Check if the plugin exists
	plugin, ok := m.LoadedPlugins[pluginID]
	if !ok {
		return errors.New("plugin not found")
	}

	//Check if the plugin is a router type plugin
	if plugin.Spec.Type != zoraxy_plugin.PluginType_Router {
		return errors.New("plugin is not a router type plugin")
	}

	m.pluginGroupsMutex.Lock()
	//Check if the tag exists
	_, ok = m.Options.PluginGroups[tag]
	if !ok {
		m.Options.PluginGroups[tag] = []string{pluginID}
		m.pluginGroupsMutex.Unlock()
		return nil
	}

	//Add the plugin to the group
	m.Options.PluginGroups[tag] = append(m.Options.PluginGroups[tag], pluginID)

	m.pluginGroupsMutex.Unlock()
	return nil
}

// RemovePluginFromGroup removes a plugin from a group
func (m *Manager) RemovePluginFromGroup(tag, pluginID string) error {
	m.pluginGroupsMutex.Lock()
	defer m.pluginGroupsMutex.Unlock()
	//Check if the tag exists
	_, ok := m.Options.PluginGroups[tag]
	if !ok {
		return errors.New("tag not found")
	}

	//Remove the plugin from the group
	pluginList := m.Options.PluginGroups[tag]
	for i, id := range pluginList {
		if id == pluginID {
			pluginList = append(pluginList[:i], pluginList[i+1:]...)
			m.Options.PluginGroups[tag] = pluginList
			return nil
		}
	}
	return errors.New("plugin not found")
}

// RemovePluginGroup removes a plugin group
func (m *Manager) RemovePluginGroup(tag string) error {
	m.pluginGroupsMutex.Lock()
	defer m.pluginGroupsMutex.Unlock()
	_, ok := m.Options.PluginGroups[tag]
	if !ok {
		return errors.New("tag not found")
	}
	delete(m.Options.PluginGroups, tag)
	return nil
}

// SavePluginGroupsFromFile loads plugin groups from a file
func (m *Manager) SavePluginGroupsToFile() error {
	m.pluginGroupsMutex.RLock()
	pluginGroupsCopy := make(map[string][]string)
	for k, v := range m.Options.PluginGroups {
		pluginGroupsCopy[k] = append([]string{}, v...)
	}
	m.pluginGroupsMutex.RUnlock()

	//Write to file
	js, _ := json.Marshal(pluginGroupsCopy)
	err := os.WriteFile(m.Options.PluginGroupsConfig, js, 0644)
	if err != nil {
		return err
	}
	return nil
}
