package plugins

import (
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
)

func (m *Manager) StartPlugin(pluginID string) error {
	plugin, ok := m.LoadedPlugins.Load(pluginID)
	if !ok {
		return errors.New("plugin not found")
	}

	//Get the plugin Entry point
	pluginEntryPoint, err := m.GetPluginEntryPoint(pluginID)
	if err != nil {
		//Plugin removed after introspect?
		return err
	}

	//Get the absolute path of the plugin entry point to prevent messing up with the cwd
	absolutePath, err := filepath.Abs(pluginEntryPoint)
	if err != nil {
		return err
	}

	//Prepare plugin start configuration
	pluginConfiguration := ConfigureSpec{
		Port:         getRandomPortNumber(),
		RuntimeConst: *m.Options.SystemConst,
	}
	js, _ := json.Marshal(pluginConfiguration)

	cmd := exec.Command(absolutePath, "-configure="+string(js))
	cmd.Dir = filepath.Dir(absolutePath)
	if err := cmd.Start(); err != nil {
		return err
	}

	// Store the cmd object so it can be accessed later for stopping the plugin
	plugin.(*Plugin).Process = cmd
	plugin.(*Plugin).Enabled = true
	return nil
}

// Check if the plugin is still running
func (m *Manager) PluginStillRunning(pluginID string) bool {
	plugin, ok := m.LoadedPlugins.Load(pluginID)
	if !ok {
		return false
	}
	return plugin.(*Plugin).Process.ProcessState == nil
}

// BlockUntilAllProcessExited blocks until all the plugins processes have exited
func (m *Manager) BlockUntilAllProcessExited() {
	m.LoadedPlugins.Range(func(key, value interface{}) bool {
		plugin := value.(*Plugin)
		if m.PluginStillRunning(value.(*Plugin).Spec.ID) {
			//Wait for the plugin to exit
			plugin.Process.Wait()
		}
		return true
	})
}
