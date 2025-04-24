package plugins

/*
	Zoraxy Plugin Manager

	This module is responsible for managing plugins
	loading plugins from the disk
	enable / disable plugins
	and forwarding traffic to plugins
*/

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
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

	//Create the plugin config file if not exists
	if !utils.FileExists(options.PluginGroupsConfig) {
		js, _ := json.Marshal(map[string][]string{})
		err := os.WriteFile(options.PluginGroupsConfig, js, 0644)
		if err != nil {
			options.Logger.PrintAndLog("plugin-manager", "Failed to create plugin group config file", err)
		}
	}

	//Create database table
	options.Database.NewTable("plugins")

	return &Manager{
		LoadedPlugins:      make(map[string]*Plugin),
		tagPluginMap:       sync.Map{},
		tagPluginListMutex: sync.RWMutex{},
		tagPluginList:      make(map[string][]*Plugin),
		Options:            options,
		/* Internal */
		loadedPluginsMutex: sync.RWMutex{},
	}
}

// Reload all plugins from disk
func (m *Manager) ReloadPluginFromDisk() {
	//Check each of the current plugins if the directory exists
	//If not, remove the plugin from the loaded plugins list
	m.loadedPluginsMutex.Lock()
	for pluginID, plugin := range m.LoadedPlugins {
		if !utils.FileExists(plugin.RootDir) {
			m.Log("Plugin directory not found, removing plugin from runtime: "+pluginID, nil)
			delete(m.LoadedPlugins, pluginID)
			//Remove the plugin enable state from the database
			m.Options.Database.Delete("plugins", pluginID)
		}
	}

	m.loadedPluginsMutex.Unlock()

	//Scan the plugin directory for new plugins
	foldersInPluginDir, err := os.ReadDir(m.Options.PluginDir)
	if err != nil {
		m.Log("Failed to read plugin directory", err)
		return
	}

	for _, folder := range foldersInPluginDir {
		if folder.IsDir() {
			pluginPath := filepath.Join(m.Options.PluginDir, folder.Name())
			thisPlugin, err := m.LoadPluginSpec(pluginPath)
			if err != nil {
				m.Log("Failed to load plugin: "+filepath.Base(pluginPath), err)
				continue
			}

			//Check if the plugin id is already loaded into the runtime
			m.loadedPluginsMutex.RLock()
			_, ok := m.LoadedPlugins[thisPlugin.Spec.ID]
			m.loadedPluginsMutex.RUnlock()
			if ok {
				//Plugin already loaded, skip it
				continue
			}

			thisPlugin.RootDir = filepath.ToSlash(pluginPath)
			thisPlugin.staticRouteProxy = make(map[string]*dpcore.ReverseProxy)
			m.loadedPluginsMutex.Lock()
			m.LoadedPlugins[thisPlugin.Spec.ID] = thisPlugin
			m.loadedPluginsMutex.Unlock()
			m.Log("Added new plugin: "+thisPlugin.Spec.Name, nil)

			// The default state of the plugin is disabled, so no need to start it
		}
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
			thisPlugin.staticRouteProxy = make(map[string]*dpcore.ReverseProxy)
			m.loadedPluginsMutex.Lock()
			m.LoadedPlugins[thisPlugin.Spec.ID] = thisPlugin
			m.loadedPluginsMutex.Unlock()
			m.Log("Loaded plugin: "+thisPlugin.Spec.Name, nil)

			// If the plugin was enabled, start it now
			//fmt.Println("Plugin enabled state", m.GetPluginPreviousEnableState(thisPlugin.Spec.ID))
			if m.GetPluginPreviousEnableState(thisPlugin.Spec.ID) {
				err = m.StartPlugin(thisPlugin.Spec.ID)
				if err != nil {
					m.Log("Failed to enable plugin: "+thisPlugin.Spec.Name, err)
				}
			}
		}
	}

	if m.Options.PluginGroupsConfig != "" {
		//Load the plugin groups from the config file
		err = m.LoadPluginGroupsFromConfig()
		if err != nil {
			m.Log("Failed to load plugin groups", err)
		}
	}

	//Generate the static forwarder radix tree
	m.UpdateTagsToPluginMaps()

	return nil
}

// GetPluginByID returns a plugin by its ID
func (m *Manager) GetPluginByID(pluginID string) (*Plugin, error) {
	m.loadedPluginsMutex.RLock()
	plugin, ok := m.LoadedPlugins[pluginID]
	m.loadedPluginsMutex.RUnlock()
	if !ok {
		return nil, errors.New("plugin not found")
	}
	return plugin, nil
}

// EnablePlugin enables a plugin
func (m *Manager) EnablePlugin(pluginID string) error {
	m.Options.Database.Write("plugins", pluginID, true)
	err := m.StartPlugin(pluginID)
	if err != nil {
		return err
	}
	//Generate the static forwarder radix tree
	m.UpdateTagsToPluginMaps()
	return nil
}

// DisablePlugin disables a plugin
func (m *Manager) DisablePlugin(pluginID string) error {
	err := m.StopPlugin(pluginID)
	m.Options.Database.Write("plugins", pluginID, false)
	if err != nil {
		return err
	}
	//Generate the static forwarder radix tree
	m.UpdateTagsToPluginMaps()
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
	plugins := []*Plugin{}
	m.loadedPluginsMutex.RLock()
	for _, plugin := range m.LoadedPlugins {
		plugins = append(plugins, plugin)
	}
	m.loadedPluginsMutex.RUnlock()
	return plugins, nil
}

// Log a message with the plugin name
func (m *Manager) LogForPlugin(p *Plugin, message string, err error) {
	processID := -1
	if p.process != nil && p.process.Process != nil {
		// Get the process ID of the plugin
		processID = p.process.Process.Pid
	}
	m.Log("["+p.Spec.Name+":"+strconv.Itoa(processID)+"] "+message, err)
}

// Terminate all plugins and exit
func (m *Manager) Close() {
	m.loadedPluginsMutex.Lock()
	pluginsToStop := make([]*Plugin, 0)
	for _, plugin := range m.LoadedPlugins {
		if plugin.Enabled {
			pluginsToStop = append(pluginsToStop, plugin)
		}
	}
	m.loadedPluginsMutex.Unlock()

	for _, thisPlugin := range pluginsToStop {
		m.Options.Logger.PrintAndLog("plugin-manager", "Stopping plugin: "+thisPlugin.Spec.Name, nil)
		m.StopPlugin(thisPlugin.Spec.ID)
	}

}

/* Plugin Functions */
func (m *Plugin) StartAllStaticPathRouters() {
	// Create a dpcore object for each of the static capture paths of the plugin
	for _, captureRule := range m.Spec.StaticCapturePaths {
		//Make sure the captureRule consists / prefix and no trailing /
		if captureRule.CapturePath == "" {
			continue
		}
		if !strings.HasPrefix(captureRule.CapturePath, "/") {
			captureRule.CapturePath = "/" + captureRule.CapturePath
		}
		captureRule.CapturePath = strings.TrimSuffix(captureRule.CapturePath, "/")

		// Create a new dpcore object to forward the traffic to the plugin
		targetURL, err := url.Parse("http://127.0.0.1:" + strconv.Itoa(m.AssignedPort) + m.Spec.StaticCaptureIngress)
		if err != nil {
			fmt.Println("Failed to parse target URL: "+targetURL.String(), err)
			continue
		}
		thisRouter := dpcore.NewDynamicProxyCore(targetURL, captureRule.CapturePath, &dpcore.DpcoreOptions{})
		m.staticRouteProxy[captureRule.CapturePath] = thisRouter
	}
}

// StopAllStaticPathRouters stops all static path routers
func (m *Plugin) StopAllStaticPathRouters() {
	for path := range m.staticRouteProxy {
		m.staticRouteProxy[path] = nil
		delete(m.staticRouteProxy, path)
	}
	m.staticRouteProxy = make(map[string]*dpcore.ReverseProxy)
}

// HandleStaticRoute handles the request to the plugin via static path captures (static forwarder)
func (p *Plugin) HandleStaticRoute(w http.ResponseWriter, r *http.Request, longestPrefix string) {
	longestPrefix = strings.TrimSuffix(longestPrefix, "/")
	targetRouter := p.staticRouteProxy[longestPrefix]
	if targetRouter == nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		fmt.Println("Error: target router not found for prefix", longestPrefix)
		return
	}

	originalRequestURI := r.RequestURI

	//Rewrite the request path to the plugin UI path
	rewrittenURL := r.RequestURI
	rewrittenURL = strings.TrimPrefix(rewrittenURL, longestPrefix)
	rewrittenURL = strings.ReplaceAll(rewrittenURL, "//", "/")
	if rewrittenURL == "" {
		rewrittenURL = "/"
	}
	r.URL, _ = url.Parse(rewrittenURL)

	targetRouter.ServeHTTP(w, r, &dpcore.ResponseRewriteRuleSet{
		UseTLS:       false,
		OriginalHost: r.Host,
		ProxyDomain:  "127.0.0.1:" + strconv.Itoa(p.AssignedPort),
		NoCache:      true,
		PathPrefix:   longestPrefix,
		UpstreamHeaders: [][]string{
			{"X-Zoraxy-Capture", longestPrefix},
			{"X-Zoraxy-URI", originalRequestURI},
		},
	})

}

// IsRunning checks if the plugin is currently running
func (p *Plugin) IsRunning() bool {
	return p.process != nil && p.process.Process != nil
}
