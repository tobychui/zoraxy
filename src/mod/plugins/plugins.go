package plugins

/*
	Zoraxy Plugin Manager

	This module is responsible for managing plugins
	loading plugins from the disk
	enable / disable plugins
	and forwarding traffic to plugins
*/

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"imuslab.com/zoraxy/mod/utils"
)

// NewPluginManager creates a new plugin manager
func NewPluginManager(options *ManagerOptions) *Manager {
	//Create plugin directory if not exists
	if options.PluginDir == "" {
		options.PluginDir = "./plugins"
	}
	if !utils.FileExists(options.PluginDir) {
		os.MkdirAll(options.PluginDir, 0755)
	}

	//Create database table
	options.Database.NewTable("plugins")

	return &Manager{
		LoadedPlugins: sync.Map{},
		Options:       options,
	}
}

// LoadPluginsFromDisk loads all plugins from the plugin directory
func (m *Manager) LoadPluginsFromDisk() error {
	// Load all plugins from the plugin directory
	foldersInPluginDir, err := os.ReadDir(m.Options.PluginDir)
	if err != nil {
		return err
	}

	for _, folder := range foldersInPluginDir {
		if folder.IsDir() {
			pluginPath := filepath.Join(m.Options.PluginDir, folder.Name())
			thisPlugin, err := m.LoadPluginSpec(pluginPath)
			if err != nil {
				m.Log("Failed to load plugin: "+filepath.Base(pluginPath), err)
				continue
			}
			thisPlugin.RootDir = filepath.ToSlash(pluginPath)
			m.LoadedPlugins.Store(thisPlugin.Spec.ID, thisPlugin)
			m.Log("Loaded plugin: "+thisPlugin.Spec.Name, nil)

			// If the plugin was enabled, start it now
			if m.GetPluginPreviousEnableState(thisPlugin.Spec.ID) {
				err = m.StartPlugin(thisPlugin.Spec.ID)
				if err != nil {
					m.Log("Failed to enable plugin: "+thisPlugin.Spec.Name, err)
				}
			}
		}
	}

	return nil
}

// GetPluginByID returns a plugin by its ID
func (m *Manager) GetPluginByID(pluginID string) (*Plugin, error) {
	plugin, ok := m.LoadedPlugins.Load(pluginID)
	if !ok {
		return nil, errors.New("plugin not found")
	}
	return plugin.(*Plugin), nil
}

// EnablePlugin enables a plugin
func (m *Manager) EnablePlugin(pluginID string) error {
	err := m.StartPlugin(pluginID)
	if err != nil {
		return err
	}
	m.Options.Database.Write("plugins", pluginID, true)
	return nil
}

// DisablePlugin disables a plugin
func (m *Manager) DisablePlugin(pluginID string) error {
	err := m.StopPlugin(pluginID)
	m.Options.Database.Write("plugins", pluginID, false)
	if err != nil {
		return err
	}
	return nil
}

// GetPluginPreviousEnableState returns the previous enable state of a plugin
func (m *Manager) GetPluginPreviousEnableState(pluginID string) bool {
	enableState := true
	err := m.Options.Database.Read("plugins", pluginID, &enableState)
	if err != nil {
		//Default to true
		return true
	}
	return enableState
}

// ListLoadedPlugins returns a list of loaded plugins
func (m *Manager) ListLoadedPlugins() ([]*Plugin, error) {
	var plugins []*Plugin
	m.LoadedPlugins.Range(func(key, value interface{}) bool {
		plugin := value.(*Plugin)
		plugins = append(plugins, plugin)
		return true
	})
	return plugins, nil
}

// Terminate all plugins and exit
func (m *Manager) Close() {
	m.LoadedPlugins.Range(func(key, value interface{}) bool {
		plugin := value.(*Plugin)
		if plugin.Enabled {
			m.StopPlugin(plugin.Spec.ID)
		}
		return true
	})

	//Wait until all loaded plugin process are terminated
	m.BlockUntilAllProcessExited()
}
