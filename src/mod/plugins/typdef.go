package plugins

import (
	_ "embed"
	"net/http"
	"os/exec"
	"sync"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
	"imuslab.com/zoraxy/mod/info/logger"
	zoraxyPlugin "imuslab.com/zoraxy/mod/plugins/zoraxy_plugin"
)

//go:embed no_img.png
var noImg []byte

type Plugin struct {
	RootDir string                   //The root directory of the plugin
	Spec    *zoraxyPlugin.IntroSpect //The plugin specification
	Enabled bool                     //Whether the plugin is enabled

	//Runtime
	AssignedPort      int                             //The assigned port for the plugin
	uiProxy           *dpcore.ReverseProxy            //The reverse proxy for the plugin UI
	staticRouteProxy  map[string]*dpcore.ReverseProxy //Storing longest prefix => dpcore map for static route
	dynamicRouteProxy *dpcore.ReverseProxy            //The reverse proxy for the dynamic route
	process           *exec.Cmd                       //The process of the plugin
}

type ManagerOptions struct {
	/* Plugins */
	PluginDir          string              //The directory where the plugins are stored
	PluginGroups       map[string][]string //The plugin groups,key is the tag name and the value is an array of plugin IDs
	PluginGroupsConfig string              //The group / tag configuration file, if set the plugin groups will be loaded from this file

	/* Plugin Downloader */
	PluginStoreURLs         []string              //The plugin store URLs, used to download the plugins
	DownloadablePluginCache []*DownloadablePlugin //The cache for the downloadable plugins, key is the plugin ID and value is the DownloadablePlugin struct
	LastSuccPluginSyncTime  int64                 //The last sync time for the plugin store URLs, used to check if the plugin store URLs need to be synced again

	/* Runtime */
	SystemConst  *zoraxyPlugin.RuntimeConstantValue //The system constant value
	CSRFTokenGen func(*http.Request) string         `json:"-"` //The CSRF token generator function
	Database     *database.Database                 `json:"-"`
	Logger       *logger.Logger                     `json:"-"`

	/* Internal */
	pluginGroupsMutex sync.RWMutex //Mutex for the pluginGroups
}

type Manager struct {
	LoadedPlugins      map[string]*Plugin   //Storing *Plugin
	tagPluginMap       sync.Map             //Storing *radix.Tree for each plugin tag
	tagPluginListMutex sync.RWMutex         //Mutex for the tagPluginList
	tagPluginList      map[string][]*Plugin //Storing the plugin list for each tag, only concurrent READ is allowed
	Options            *ManagerOptions

	/* Internal */
	loadedPluginsMutex sync.RWMutex //Mutex for the loadedPlugins
}
