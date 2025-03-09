package plugins

import (
	"net/http"
	"sync"

	"github.com/armon/go-radix"
)

// HandleRoute handles the request to the plugin
// return true if the request is handled by the plugin
func (m *Manager) HandleRoute(w http.ResponseWriter, r *http.Request, tags []string) bool {
	if len(tags) == 0 {
		return false
	}

	//For each tag, check if the request path matches the static capture path
	wg := sync.WaitGroup{}                     //Wait group for the goroutines
	mutex := sync.Mutex{}                      //Mutex for the dynamic route handler
	var staticRoutehandlers []*Plugin          //The handler for the request, can be multiple plugins
	var longestPrefixAcrossAlltags string = "" //The longest prefix across all tags
	var dynamicRouteHandlers []*Plugin         //The handler for the dynamic routes
	for _, tag := range tags {
		wg.Add(1)
		go func(thisTag string) {
			defer wg.Done()
			//Get the radix tree for the tag
			tree, ok := m.tagPluginMap.Load(thisTag)
			if !ok {
				return
			}

			//Check if the request path matches the static capture path
			longestPrefix, pluginList, ok := tree.(*radix.Tree).LongestPrefix(r.URL.Path)
			if ok {
				if longestPrefix > longestPrefixAcrossAlltags {
					longestPrefixAcrossAlltags = longestPrefix
					staticRoutehandlers = pluginList.([]*Plugin)
				}
			}

		}(tag)

		//Check if the plugin enabled dynamic route
		wg.Add(1)
		go func(thisTag string) {
			defer wg.Done()
			m.tagPluginListMutex.RLock()
			for _, plugin := range m.tagPluginList[thisTag] {
				if plugin.Enabled && plugin.Spec.DynamicCaptureSniff != "" && plugin.Spec.DynamicCaptureIngress != "" {
					mutex.Lock()
					dynamicRouteHandlers = append(dynamicRouteHandlers, plugin)
					mutex.Unlock()
				}
			}
			m.tagPluginListMutex.RUnlock()
		}(tag)
	}
	wg.Wait()

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
