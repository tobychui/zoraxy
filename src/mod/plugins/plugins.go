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

	//Create database table
	options.Database.NewTable("plugins")

	return &Manager{
		LoadedPlugins: sync.Map{},
		TagPluginMap:  sync.Map{},
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
			thisPlugin.staticRouteProxy = make(map[string]*dpcore.ReverseProxy)
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

	//Generate the static forwarder radix tree
	m.UpdateTagsToTree()

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
	//Generate the static forwarder radix tree
	m.UpdateTagsToTree()
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
	m.UpdateTagsToTree()
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
	var plugins []*Plugin = []*Plugin{}
	m.LoadedPlugins.Range(func(key, value interface{}) bool {
		plugin := value.(*Plugin)
		plugins = append(plugins, plugin)
		return true
	})
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

func (m *Plugin) StopAllStaticPathRouters() {

}

func (p *Plugin) HandleRoute(w http.ResponseWriter, r *http.Request, longestPrefix string) {
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
