package plugins

import "net/http"

/*
	Forwarder.go

	This file handles the dynamic proxy routing forwarding
	request to plugin capture path that handles the matching
	request path registered when the plugin started
*/

func (m *Manager) GetHandlerPlugins(w http.ResponseWriter, r *http.Request) {

}

func (m *Manager) GetHandlerPluginsSubsets(w http.ResponseWriter, r *http.Request) {

}

func (p *Plugin) HandlePluginRoute(w http.ResponseWriter, r *http.Request) {
	//Find the plugin that matches the request path
	//If no plugin found, return 404
	//If found, forward the request to the plugin

}
