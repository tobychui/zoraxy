package plugins

import (
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
	zoraxyPlugin "imuslab.com/zoraxy/mod/plugins/zoraxy_plugin"
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
	pluginConfiguration := zoraxyPlugin.ConfigureSpec{
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

	//Create a UI forwarder if the plugin has UI
	err = m.StartUIHandlerForPlugin(thisPlugin, pluginConfiguration.Port)
	if err != nil {
		return err
	}

	// Store the cmd object so it can be accessed later for stopping the plugin
	plugin.(*Plugin).process = cmd
	plugin.(*Plugin).Enabled = true
	return nil
}

// StartUIHandlerForPlugin starts a UI handler for the plugin
func (m *Manager) StartUIHandlerForPlugin(targetPlugin *Plugin, pluginListeningPort int) error {
	// Create a dpcore object to reverse proxy the plugin ui
	pluginUIRelPath := targetPlugin.Spec.UIPath
	if !strings.HasPrefix(pluginUIRelPath, "/") {
		pluginUIRelPath = "/" + pluginUIRelPath
	}

	// Remove the trailing slash if it exists
	pluginUIRelPath = strings.TrimSuffix(pluginUIRelPath, "/")

	pluginUIURL, err := url.Parse("http://127.0.0.1:" + strconv.Itoa(pluginListeningPort) + pluginUIRelPath)
	if err != nil {
		return err
	}

	// Generate the plugin subpath to be trimmed
	pluginMatchingPath := filepath.ToSlash(filepath.Join("/plugin.ui/"+targetPlugin.Spec.ID+"/")) + "/"
	if targetPlugin.Spec.UIPath != "" {
		targetPlugin.uiProxy = dpcore.NewDynamicProxyCore(
			pluginUIURL,
			pluginMatchingPath,
			&dpcore.DpcoreOptions{
				IgnoreTLSVerification: true,
			},
		)
		targetPlugin.AssignedPort = pluginListeningPort
		m.LoadedPlugins.Store(targetPlugin.Spec.ID, targetPlugin)
	}
	return nil
}

func (m *Manager) handlePluginSTDOUT(pluginID string, line string) {
	thisPlugin, err := m.GetPluginByID(pluginID)
	processID := -1
	if thisPlugin.process != nil && thisPlugin.process.Process != nil {
		// Get the process ID of the plugin
		processID = thisPlugin.process.Process.Pid
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
	thisPlugin.process.Process.Signal(os.Interrupt)
	go func() {
		//Wait for 10 seconds for the plugin to stop gracefully
		time.Sleep(10 * time.Second)
		if thisPlugin.process.ProcessState == nil || !thisPlugin.process.ProcessState.Exited() {
			m.Log("Plugin "+thisPlugin.Spec.Name+" failed to stop gracefully, killing it", nil)
			thisPlugin.process.Process.Kill()
		} else {
			m.Log("Plugin "+thisPlugin.Spec.Name+" background process stopped", nil)
		}

		//Remove the UI proxy
		thisPlugin.uiProxy = nil
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
	if plugin.(*Plugin).process == nil {
		return false
	}
	return plugin.(*Plugin).process.ProcessState == nil
}

// BlockUntilAllProcessExited blocks until all the plugins processes have exited
func (m *Manager) BlockUntilAllProcessExited() {
	m.LoadedPlugins.Range(func(key, value interface{}) bool {
		plugin := value.(*Plugin)
		if m.PluginStillRunning(value.(*Plugin).Spec.ID) {
			//Wait for the plugin to exit
			plugin.process.Wait()
		}
		return true
	})
}
