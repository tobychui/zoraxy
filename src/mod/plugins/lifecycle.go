package plugins

import (
	"encoding/json"
	"errors"
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
	"imuslab.com/zoraxy/mod/eventsystem"
	zoraxyPlugin "imuslab.com/zoraxy/mod/plugins/zoraxy_plugin"
	"imuslab.com/zoraxy/mod/plugins/zoraxy_plugin/events"
)

func (m *Manager) StartPlugin(pluginID string) error {
	thisPlugin, err := m.GetPluginByID(pluginID)
	if err != nil {
		return err
	}

	//Retain an unavailable inspection binding if startup fails. Only an
	//explicit stop/disable withdraws a configured inspector.
	m.lifecycleMutex.Lock()
	thisPlugin.inspectFailed = true
	m.publishInspectorsLocked()
	m.lifecycleMutex.Unlock()

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

	// Generate API key if the plugin has permitted endpoints
	if len(thisPlugin.Spec.PermittedAPIEndpoints) > 0 {
		apiKey, err := m.Options.APIKeyManager.GenerateAPIKey(thisPlugin.Spec.ID, thisPlugin.Spec.PermittedAPIEndpoints)
		if err != nil {
			return err
		}
		pluginConfiguration.APIKey = apiKey.APIKey
		pluginConfiguration.ZoraxyPort = m.Options.ZoraxyPort
		m.Log("Generated API key for plugin "+thisPlugin.Spec.Name, nil)
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
	// The output readers below belong to exactly this process. Pass its PID to
	// them instead of reading thisPlugin.process, which is assigned later and
	// may already belong to a newer process after a restart.
	pid := cmd.Process.Pid
	exited := make(chan struct{})
	m.lifecycleMutex.Lock()
	thisPlugin.process = cmd
	thisPlugin.processExited = exited
	m.lifecycleMutex.Unlock()

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
						m.handlePluginSTDOUT(pluginID, pid, lineBuf[:idx])
						lineBuf = lineBuf[idx+1:]
					} else {
						break
					}
				}
			}
			if err != nil {
				if err != io.EOF {
					m.handlePluginSTDOUT(pluginID, pid, lineBuf) // handle any remaining data
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
						m.handlePluginSTDERR(pluginID, pid, lineBuf[:idx])
						lineBuf = lineBuf[idx+1:]
					} else {
						break
					}
				}
			}
			if err != nil {
				if err != io.EOF {
					m.handlePluginSTDERR(pluginID, pid, lineBuf) // handle any remaining data
				}
				break
			}
		}
	}()

	//Create a goroutine to wait for the plugin to exit
	go func() {
		err := cmd.Wait()
		close(exited)
		m.handlePluginExit(thisPlugin, cmd, err)
	}()

	//Create a UI forwarder if the plugin has UI
	m.lifecycleMutex.Lock()
	err = m.StartUIHandlerForPlugin(thisPlugin, pluginConfiguration.Port)
	select {
	case <-exited:
		//A crash before this point must not leave a dead process enabled
		err = errors.New("plugin " + thisPlugin.Spec.ID + " exited during startup")
	default:
	}
	if err == nil {
		thisPlugin.Enabled = true
		thisPlugin.inspectFailed = false
		m.publishInspectorsLocked()
	}
	m.lifecycleMutex.Unlock()
	if err != nil {
		return err
	}

	// Register event subscriptions
	if thisPlugin.Spec.SubscriptionsEvents != nil {
		for eventName := range thisPlugin.Spec.SubscriptionsEvents {
			eventType := events.EventName(eventName)
			if !eventType.IsValid() {
				m.Log("Invalid event name: "+string(eventName), nil)
				continue
			}
			err := eventsystem.Publisher.RegisterSubscriberToEvent(thisPlugin, eventType)
			if err != nil {
				m.Log("Failed to subscribe plugin "+thisPlugin.Spec.Name+" to event "+string(eventName), err)
			} else {
				m.Log("Subscribed plugin "+thisPlugin.Spec.Name+" to event "+string(eventName), nil)
			}
		}
	}

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

// handlePluginExit disables a plugin whose current process failed. The exit
// of a replaced process must not disable the instance that superseded it.
func (m *Manager) handlePluginExit(thisPlugin *Plugin, cmd *exec.Cmd, err error) {
	m.lifecycleMutex.Lock()
	current := thisPlugin.process == cmd && thisPlugin.Enabled
	if current {
		thisPlugin.inspectFailed = true
		thisPlugin.Enabled = false
		m.publishInspectorsLocked()
	}
	m.lifecycleMutex.Unlock()
	if !current {
		return
	}

	//In theory this should not happen except for a crash
	m.Log("plugin "+thisPlugin.Spec.ID+" encounted a fatal error. Disabling plugin...", err)

	//Generate a new static forwarder radix tree
	m.UpdateTagsToPluginMaps()
}

// handlePluginSTDOUT logs one stdout line of the plugin process with the given
// PID. The PID is passed in by the reader goroutine that owns the process, so
// no shared plugin state is read concurrently with StartPlugin.
func (m *Manager) handlePluginSTDOUT(pluginID string, processID int, line string) {
	thisPlugin, err := m.GetPluginByID(pluginID)
	if err != nil {
		m.Log("[unknown:"+strconv.Itoa(processID)+"] "+line, err)
		return
	}
	m.Log("["+thisPlugin.Spec.Name+":"+strconv.Itoa(processID)+"] "+line, nil)
}

// handlePluginSTDERR logs one stderr line of the plugin process with the given PID.
func (m *Manager) handlePluginSTDERR(pluginID string, processID int, line string) {
	thisPlugin, err := m.GetPluginByID(pluginID)
	if err != nil {
		return
	}
	m.Log("["+thisPlugin.Spec.Name+":"+strconv.Itoa(processID)+"] "+line, nil)
}

// StopPlugin stops a plugin, it is garanteed that the plugin is stopped after this function
func (m *Manager) StopPlugin(pluginID string) error {
	thisPlugin, err := m.GetPluginByID(pluginID)
	if err != nil {
		return err
	}

	//Withdraw the plugin from request inspection before it starts shutting down
	m.lifecycleMutex.Lock()
	thisPlugin.Enabled = false
	thisPlugin.inspectFailed = false
	m.publishInspectorsLocked()
	exited := thisPlugin.processExited
	m.lifecycleMutex.Unlock()

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
	} else if thisPlugin.process != nil && thisPlugin.process.Process != nil {
		//Send SIGTERM to the plugin process, if it is still running
		err = thisPlugin.process.Process.Signal(syscall.SIGTERM)
		if err != nil {
			m.Log("Failed to send Interrupt signal to plugin "+thisPlugin.Spec.Name+": "+err.Error(), nil)
		}

		//Wait for the plugin to stop. ProcessState is owned by the goroutine
		//running cmd.Wait, so wait for its exit signal instead of polling it.
		select {
		case <-exited:
			m.Log("Plugin "+thisPlugin.Spec.Name+" background process stopped", nil)
		case <-time.After(5 * time.Second):
			m.Log("Plugin "+thisPlugin.Spec.Name+" failed to stop gracefully, killing it", nil)
			thisPlugin.process.Process.Kill()
		}
	}

	//Remove the UI proxy
	m.lifecycleMutex.Lock()
	thisPlugin.uiProxy = nil
	thisPlugin.Enabled = false
	m.publishInspectorsLocked()
	m.lifecycleMutex.Unlock()
	thisPlugin.StopAllStaticPathRouters()
	thisPlugin.StopDynamicForwardRouter()

	//Clean up API key
	err = m.Options.APIKeyManager.RevokeAPIKeysForPlugin(thisPlugin.Spec.ID)
	if err != nil {
		m.Log("Failed to revoke API keys for plugin "+thisPlugin.Spec.Name, err)
	}
	//Unsubscribe from all events
	err = eventsystem.Publisher.UnregisterSubscriber(eventsystem.ListenerID(thisPlugin.Spec.ID))
	if err != nil {
		m.Log("Failed to unsubscribe plugin "+thisPlugin.Spec.Name+" from events", err)
	}

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
