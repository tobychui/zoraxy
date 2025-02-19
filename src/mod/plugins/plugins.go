package plugins

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/info/logger"
)

type Plugin struct {
	Spec    *IntroSpect //The plugin specification
	Process *exec.Cmd   //The process of the plugin
	Enabled bool        //Whether the plugin is enabled
}

type ManagerOptions struct {
	ZoraxyVersion string
	PluginDir     string
	SystemConst   *RuntimeConstantValue
	Database      database.Database
	Logger        *logger.Logger
}

type Manager struct {
	LoadedPlugins sync.Map //Storing *Plugin
	Options       *ManagerOptions
}

func NewPluginManager(options *ManagerOptions) *Manager {
	return &Manager{
		LoadedPlugins: sync.Map{},
		Options:       options,
	}
}

// LoadPlugins loads all plugins from the plugin directory
func (m *Manager) LoadPlugins() error {
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
			m.LoadedPlugins.Store(thisPlugin.Spec.ID, thisPlugin)
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
	plugin, ok := m.LoadedPlugins.Load(pluginID)
	if !ok {
		return errors.New("plugin not found")
	}
	plugin.(*Plugin).Enabled = true
	return nil
}

// DisablePlugin disables a plugin
func (m *Manager) DisablePlugin(pluginID string) error {
	plugin, ok := m.LoadedPlugins.Load(pluginID)
	if !ok {
		return errors.New("plugin not found")
	}

	thisPlugin := plugin.(*Plugin)
	thisPlugin.Process.Process.Signal(os.Interrupt)
	go func() {
		//Wait for 10 seconds for the plugin to stop gracefully
		time.Sleep(10 * time.Second)
		if thisPlugin.Process.ProcessState == nil || !thisPlugin.Process.ProcessState.Exited() {
			m.Log("Plugin "+thisPlugin.Spec.Name+" failed to stop gracefully, killing it", nil)
			thisPlugin.Process.Process.Kill()
		} else {
			m.Log("Plugin "+thisPlugin.Spec.Name+" background process stopped", nil)
		}
	}()
	thisPlugin.Enabled = false
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
