package plugins

import (
	"net/http"

	"github.com/armon/go-radix"
)

// HandleRoute handles the request to the plugin
// return true if the request is handled by the plugin
func (m *Manager) HandleRoute(w http.ResponseWriter, r *http.Request, tags []string) bool {
	if len(tags) == 0 {
		return false
	}

	//For each tag, check if the request path matches the static capture path                    //Wait group for the goroutines
	var staticRoutehandlers []*Plugin          //The handler for the request, can be multiple plugins
	var longestPrefixAcrossAlltags string = "" //The longest prefix across all tags
	var dynamicRouteHandlers []*Plugin         //The handler for the dynamic routes
	for _, tag := range tags {
		//Get the radix tree for the tag
		tree, ok := m.tagPluginMap.Load(tag)
		if ok {
			//Check if the request path matches the static capture path
			longestPrefix, pluginList, ok := tree.(*radix.Tree).LongestPrefix(r.URL.Path)
			if ok {
				if longestPrefix > longestPrefixAcrossAlltags {
					longestPrefixAcrossAlltags = longestPrefix
					staticRoutehandlers = pluginList.([]*Plugin)
				}
			}
		}

		//Check if the plugin enabled dynamic route
		m.tagPluginListMutex.RLock()
		for _, plugin := range m.tagPluginList[tag] {
			if plugin.Enabled && plugin.Spec.DynamicCaptureSniff != "" && plugin.Spec.DynamicCaptureIngress != "" {
				dynamicRouteHandlers = append(dynamicRouteHandlers, plugin)
			}
		}
		m.tagPluginListMutex.RUnlock()
	}

	//Handle the static route if found
	if len(staticRoutehandlers) > 0 {
		//Handle the request
		staticRoutehandlers[0].HandleStaticRoute(w, r, longestPrefixAcrossAlltags)
		return true
	}

	//No static route handler found, check for dynamic route handler
	for _, plugin := range dynamicRouteHandlers {
		if plugin.HandleDynamicRoute(w, r) {
			return true
		}
	}
	return false
}
