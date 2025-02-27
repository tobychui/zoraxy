package plugins

import (
	"net/http"

	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
	"imuslab.com/zoraxy/mod/utils"
)

// HandlePluginUI handles the request to the plugin UI
// This function will route the request to the correct plugin UI handler
func (m *Manager) HandlePluginUI(pluginID string, w http.ResponseWriter, r *http.Request) {
	plugin, err := m.GetPluginByID(pluginID)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Check if the plugin has UI
	if plugin.Spec.UIPath == "" {
		utils.SendErrorResponse(w, "Plugin does not have UI")
		return
	}

	//Check if the plugin has UI handler
	if plugin.uiProxy == nil {
		utils.SendErrorResponse(w, "Plugin does not have UI handler")
		return
	}

	//Call the plugin UI handler
	plugin.uiProxy.ServeHTTP(w, r, &dpcore.ResponseRewriteRuleSet{
		UseTLS:       false,
		OriginalHost: r.Host,
		ProxyDomain:  r.Host,
		NoCache:      true,
		PathPrefix:   "/plugin.ui/" + pluginID,
		Version:      m.Options.SystemConst.ZoraxyVersion,
	})

}
