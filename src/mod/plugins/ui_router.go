package plugins

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

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

	upstreamOrigin := "127.0.0.1:" + strconv.Itoa(plugin.AssignedPort)
	matchingPath := "/plugin.ui/" + plugin.Spec.ID

	//Rewrite the request path to the plugin UI path
	rewrittenURL := r.RequestURI
	rewrittenURL = strings.TrimPrefix(rewrittenURL, matchingPath)
	rewrittenURL = strings.ReplaceAll(rewrittenURL, "//", "/")
	if rewrittenURL == "" {
		rewrittenURL = "/"
	}
	r.URL, _ = url.Parse(rewrittenURL)

	//Call the plugin UI handler
	plugin.uiProxy.ServeHTTP(w, r, &dpcore.ResponseRewriteRuleSet{
		UseTLS:       false,
		OriginalHost: r.Host,
		ProxyDomain:  upstreamOrigin,
		NoCache:      true,
		PathPrefix:   matchingPath,
		Version:      m.Options.SystemConst.ZoraxyVersion,
		UpstreamHeaders: [][]string{
			{"X-Zoraxy-Csrf", m.Options.CSRFTokenGen(r)},
		},
	})
}
