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
	AssignedPort int                  //The assigned port for the plugin
	uiProxy      *dpcore.ReverseProxy //The reverse proxy for the plugin UI
	process      *exec.Cmd            //The process of the plugin
}

type ManagerOptions struct {
	PluginDir    string
	SystemConst  *zoraxyPlugin.RuntimeConstantValue
	Database     *database.Database
	Logger       *logger.Logger
	CSRFTokenGen func(*http.Request) string //The CSRF token generator function
}

type Manager struct {
	LoadedPlugins sync.Map //Storing *Plugin
	Options       *ManagerOptions
}
