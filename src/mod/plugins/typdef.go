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
	PluginDir          string              //The directory where the plugins are stored
	PluginGroups       map[string][]string //The plugin groups,key is the tag name and the value is an array of plugin IDs
	PluginGroupsConfig string              //The group / tag configuration file, if set the plugin groups will be loaded from this file

	/* Runtime */
	SystemConst  *zoraxyPlugin.RuntimeConstantValue
	CSRFTokenGen func(*http.Request) string `json:"-"` //The CSRF token generator function
	Database     *database.Database         `json:"-"`
	Logger       *logger.Logger             `json:"-"`

	/* Internal */
	pluginGroupsMutex sync.RWMutex //Mutex for the pluginGroups
}

type Manager struct {
	LoadedPlugins      sync.Map             //Storing *Plugin
	tagPluginMap       sync.Map             //Storing *radix.Tree for each plugin tag
	tagPluginListMutex sync.RWMutex         //Mutex for the tagPluginList
	tagPluginList      map[string][]*Plugin //Storing the plugin list for each tag, only concurrent READ is allowed
	Options            *ManagerOptions
}
