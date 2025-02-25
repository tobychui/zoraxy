package plugins

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (m *Manager) StartPlugin(pluginID string) error {
	plugin, ok := m.LoadedPlugins.Load(pluginID)
	if !ok {
		return errors.New("plugin not found")
	}

	thisPlugin := plugin.(*Plugin)

	//Get the plugin Entry point
	pluginEntryPoint, err := m.GetPluginEntryPoint(thisPlugin.RootDir)
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

	m.Log("Starting plugin "+thisPlugin.Spec.Name+" at :"+strconv.Itoa(pluginConfiguration.Port), nil)
	cmd := exec.Command(absolutePath, "-configure="+string(js))
	cmd.Dir = filepath.Dir(absolutePath)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	go func() {
		buf := make([]byte, 1)
		lineBuf := ""
		for {
			n, err := stdoutPipe.Read(buf)
			if n > 0 {
				lineBuf += string(buf[:n])
				for {
					if idx := strings.IndexByte(lineBuf, '\n'); idx != -1 {
						m.handlePluginSTDOUT(pluginID, lineBuf[:idx])
						lineBuf = lineBuf[idx+1:]
					} else {
						break
					}
				}
			}
			if err != nil {
				if err != io.EOF {
					m.handlePluginSTDOUT(pluginID, lineBuf) // handle any remaining data
				}
				break
			}
		}
	}()

	// Store the cmd object so it can be accessed later for stopping the plugin
	plugin.(*Plugin).Process = cmd
	plugin.(*Plugin).Enabled = true
	return nil
}

func (m *Manager) handlePluginSTDOUT(pluginID string, line string) {
	thisPlugin, err := m.GetPluginByID(pluginID)
	processID := -1
	if thisPlugin.Process != nil && thisPlugin.Process.Process != nil {
		// Get the process ID of the plugin
		processID = thisPlugin.Process.Process.Pid
	}
	if err != nil {
		m.Log("[unknown:"+strconv.Itoa(processID)+"] "+line, err)
		return
	}
	m.Log("["+thisPlugin.Spec.Name+":"+strconv.Itoa(processID)+"] "+line, nil)
}

func (m *Manager) StopPlugin(pluginID string) error {
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
	plugin.(*Plugin).Enabled = false
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
