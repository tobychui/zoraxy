package plugins

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/utils"
)

type Plugin struct {
	RootDir string      //The root directory of the plugin
	Spec    *IntroSpect //The plugin specification
	Process *exec.Cmd   //The process of the plugin
	Enabled bool        //Whether the plugin is enabled
}

type ManagerOptions struct {
	PluginDir   string
	SystemConst *RuntimeConstantValue
	Database    *database.Database
	Logger      *logger.Logger
}

type Manager struct {
	LoadedPlugins sync.Map //Storing *Plugin
	Options       *ManagerOptions
}

// NewPluginManager creates a new plugin manager
func NewPluginManager(options *ManagerOptions) *Manager {
	if options.PluginDir == "" {
		options.PluginDir = "./plugins"
	}

	if !utils.FileExists(options.PluginDir) {
		os.MkdirAll(options.PluginDir, 0755)
	}

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
			thisPlugin.RootDir = pluginPath
			m.LoadedPlugins.Store(thisPlugin.Spec.ID, thisPlugin)
			m.Log("Loaded plugin: "+thisPlugin.Spec.Name, nil)

			//TODO: Move this to a separate function
			// Enable the plugin if it is enabled in the database
			err = m.StartPlugin(thisPlugin.Spec.ID)
			if err != nil {
				m.Log("Failed to enable plugin: "+thisPlugin.Spec.Name, err)
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
	//TODO: Add database record
	return nil
}

// DisablePlugin disables a plugin
func (m *Manager) DisablePlugin(pluginID string) error {
	err := m.StopPlugin(pluginID)
	//TODO: Add database record
	if err != nil {
		return err
	}
	return nil
}

// Terminate all plugins and exit
func (m *Manager) Close() {
	m.LoadedPlugins.Range(func(key, value interface{}) bool {
		plugin := value.(*Plugin)
		if plugin.Enabled {
			m.DisablePlugin(plugin.Spec.ID)
		}
		return true
	})

	//Wait until all loaded plugin process are terminated
	m.BlockUntilAllProcessExited()
}
