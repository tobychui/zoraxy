package dynamicproxy

import (
	"net/http"

	"imuslab.com/zoraxy/mod/plugins"
)

// inspectRoute runs after access/auth checks and before forwarding or upgrade.
func (router *Router) inspectRoute(w http.ResponseWriter, r *http.Request, endpoint *ProxyEndpoint) bool {
	if router.Option.PluginManager == nil {
		return false
	}
	handled, status := router.Option.PluginManager.InspectRoute(w, r, plugins.RequestInspectTarget{
		RouteID:  endpoint.RootOrMatchingDomain,
		Tags:     endpoint.Tags,
		ClientIP: router.GetClientIPForEndpoint(r, endpoint),
	})
	if handled {
		router.logRequest(r, false, status, "inspect-blocked", r.Host, "", endpoint)
	}
	return handled
}
