package plugins

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
	zoraxyPlugin "imuslab.com/zoraxy/mod/plugins/zoraxy_plugin"
)

func (m *Manager) StartPlugin(pluginID string) error {
	thisPlugin, err := m.GetPluginByID(pluginID)
	if err != nil {
		return err
	}

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

	//Start the plugin with given configuration
	m.Log("Starting plugin "+thisPlugin.Spec.Name+" at :"+strconv.Itoa(pluginConfiguration.Port), nil)
	cmd := exec.Command(absolutePath, "-configure="+string(js))
	cmd.Dir = filepath.Dir(absolutePath)
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stdErrPipe, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	//Create a goroutine to handle the STDOUT of the plugin
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

	//Create a goroutine to handle the STDERR of the plugin
	go func() {
		buf := make([]byte, 1)
		lineBuf := ""
		for {
			n, err := stdErrPipe.Read(buf)
			if n > 0 {
				lineBuf += string(buf[:n])
				for {
					if idx := strings.IndexByte(lineBuf, '\n'); idx != -1 {
						m.handlePluginSTDERR(pluginID, lineBuf[:idx])
						lineBuf = lineBuf[idx+1:]
					} else {
						break
					}
				}
			}
			if err != nil {
				if err != io.EOF {
					m.handlePluginSTDERR(pluginID, lineBuf) // handle any remaining data
				}
				break
			}
		}
	}()

	//Create a goroutine to wait for the plugin to exit
	go func() {
		err := cmd.Wait()
		if err != nil {
			//In theory this should not happen except for a crash
			m.Log("plugin "+thisPlugin.Spec.ID+" encounted a fatal error. Disabling plugin...", err)

			//Set the plugin state to disabled
			thisPlugin.Enabled = false

			//Generate a new static forwarder radix tree
			m.UpdateTagsToPluginMaps()
			return
		}
	}()

	//Create a UI forwarder if the plugin has UI
	err = m.StartUIHandlerForPlugin(thisPlugin, pluginConfiguration.Port)
	if err != nil {
		return err
	}

	// Store the cmd object so it can be accessed later for stopping the plugin
	thisPlugin.process = cmd
	thisPlugin.Enabled = true

	//Create a new static forwarder router for each of the static capture paths
	thisPlugin.StartAllStaticPathRouters()

	//If the plugin contains dynamic capture, create a dynamic capture handler
	if thisPlugin.AcceptDynamicRoute() {
		thisPlugin.StartDynamicForwardRouter()
	}

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
			&dpcore.DpcoreOptions{},
		)
		targetPlugin.AssignedPort = pluginListeningPort
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

func (m *Manager) handlePluginSTDERR(pluginID string, line string) {
	thisPlugin, err := m.GetPluginByID(pluginID)
	if err != nil {
		return
	}
	processID := -1
	if thisPlugin.process != nil && thisPlugin.process.Process != nil {
		// Get the process ID of the plugin
		processID = thisPlugin.process.Process.Pid
	}
	m.Log("["+thisPlugin.Spec.Name+":"+strconv.Itoa(processID)+"] "+line, nil)
}

// StopPlugin stops a plugin, it is garanteed that the plugin is stopped after this function
func (m *Manager) StopPlugin(pluginID string) error {
	thisPlugin, err := m.GetPluginByID(pluginID)
	if err != nil {
		return err
	}

	//Make a GET request to plugin ui path /term to gracefully stop the plugin
	if thisPlugin.uiProxy != nil {
		uiRelativePath := thisPlugin.Spec.UIPath
		if !strings.HasPrefix(uiRelativePath, "/") {
			uiRelativePath = "/" + uiRelativePath
		}
		requestURI := "http://127.0.0.1:" + strconv.Itoa(thisPlugin.AssignedPort) + uiRelativePath + "/term"

		client := http.Client{
			Timeout: 3 * time.Second,
		}
		resp, err := client.Get(requestURI)
		if err != nil {
			// Plugin does not support termination request, do it the hard way
			m.Log("Plugin "+thisPlugin.Spec.ID+" termination request failed. Force shutting down", nil)
		} else {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				if resp.StatusCode == http.StatusNotFound {
					m.Log("Plugin "+thisPlugin.Spec.ID+" does not support termination request", nil)
				} else {
					m.Log("Plugin "+thisPlugin.Spec.ID+" termination request returned status: "+resp.Status, nil)
				}
			}
		}
	}

	if runtime.GOOS == "windows" && thisPlugin.process != nil {
		//There is no SIGTERM in windows, kill the process directly
		time.Sleep(300 * time.Millisecond)
		thisPlugin.process.Process.Kill()
	} else {
		//Send SIGTERM to the plugin process, if it is still running
		err = thisPlugin.process.Process.Signal(syscall.SIGTERM)
		if err != nil {
			m.Log("Failed to send Interrupt signal to plugin "+thisPlugin.Spec.Name+": "+err.Error(), nil)
		}

		//Wait for the plugin to stop
		for range 5 {
			time.Sleep(1 * time.Second)
			if thisPlugin.process.ProcessState != nil && thisPlugin.process.ProcessState.Exited() {
				m.Log("Plugin "+thisPlugin.Spec.Name+" background process stopped", nil)
				break
			}
		}
		if thisPlugin.process.ProcessState == nil || !thisPlugin.process.ProcessState.Exited() {
			m.Log("Plugin "+thisPlugin.Spec.Name+" failed to stop gracefully, killing it", nil)
			thisPlugin.process.Process.Kill()
		}
	}

	//Remove the UI proxy
	thisPlugin.uiProxy = nil
	thisPlugin.Enabled = false
	thisPlugin.StopAllStaticPathRouters()
	thisPlugin.StopDynamicForwardRouter()
	return nil
}

// Check if the plugin is still running
func (m *Manager) PluginIsRunning(pluginID string) bool {
	plugin, err := m.GetPluginByID(pluginID)
	if err != nil {
		return false
	}
	return plugin.IsRunning()
}
