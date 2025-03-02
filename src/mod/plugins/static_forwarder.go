package plugins

import (
	"errors"
	"sync"

	"github.com/armon/go-radix"
)

/*
	Static Forwarder

	This file handles the dynamic proxy routing forwarding
	request to plugin capture path that handles the matching
	request path registered when the plugin started
*/

func (m *Manager) UpdateTagsToPluginMaps() {
	//build the tag to plugin pointer sync.Map
	m.tagPluginMap = sync.Map{}
	for tag, pluginIds := range m.Options.PluginGroups {
		tree := m.GetForwarderRadixTreeFromPlugins(pluginIds)
		m.tagPluginMap.Store(tag, tree)
	}

	//build the plugin list for each tag
	m.tagPluginList = make(map[string][]*Plugin)
	for tag, pluginIds := range m.Options.PluginGroups {
		for _, pluginId := range pluginIds {
			plugin, err := m.GetPluginByID(pluginId)
			if err != nil {
				m.Log("Failed to get plugin by ID: "+pluginId, err)
				continue
			}
			m.tagPluginList[tag] = append(m.tagPluginList[tag], plugin)
		}
	}
}

// GenerateForwarderRadixTree generates the radix tree for static forwarders
func (m *Manager) GetForwarderRadixTreeFromPlugins(pluginIds []string) *radix.Tree {
	// Create a new radix tree
	r := radix.New()

	// Iterate over the loaded plugins and insert their paths into the radix tree
	m.LoadedPlugins.Range(func(key, value interface{}) bool {
		plugin := value.(*Plugin)
		if !plugin.Enabled {
			//Ignore disabled plugins
			return true
		}

		// Check if the plugin ID is in the list of plugin IDs
		includeThisPlugin := false
		for _, id := range pluginIds {
			if plugin.Spec.ID == id {
				includeThisPlugin = true
			}
		}
		if !includeThisPlugin {
			return true
		}

		//For each of the plugin, insert the requested static capture paths
		if len(plugin.Spec.StaticCapturePaths) > 0 {
			for _, captureRule := range plugin.Spec.StaticCapturePaths {
				_, ok := r.Get(captureRule.CapturePath)
				m.LogForPlugin(plugin, "Assigned static capture path: "+captureRule.CapturePath, nil)
				if !ok {
					//If the path does not exist, create a new list
					newPluginList := make([]*Plugin, 0)
					newPluginList = append(newPluginList, plugin)
					r.Insert(captureRule.CapturePath, newPluginList)
				} else {
					//The path has already been assigned to another plugin
					pluginList, _ := r.Get(captureRule.CapturePath)

					//Warn the path is already assigned to another plugin
					if plugin.Spec.ID == pluginList.([]*Plugin)[0].Spec.ID {
						m.Log("Duplicate path register for plugin: "+plugin.Spec.Name+" ("+plugin.Spec.ID+")", errors.New("duplcated path: "+captureRule.CapturePath))
						continue
					}
					incompatiblePluginAInfo := pluginList.([]*Plugin)[0].Spec.Name + " (" + pluginList.([]*Plugin)[0].Spec.ID + ")"
					incompatiblePluginBInfo := plugin.Spec.Name + " (" + plugin.Spec.ID + ")"
					m.Log("Incompatible plugins: "+incompatiblePluginAInfo+" and "+incompatiblePluginBInfo, errors.New("incompatible plugins found for path: "+captureRule.CapturePath))
				}
			}
		}
		return true
	})

	return r
}
