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
	var wg sync.WaitGroup                      //Wait group for the goroutines
	var handler []*Plugin                      //The handler for the request, can be multiple plugins
	var longestPrefixAcrossAlltags string = "" //The longest prefix across all tags
	for _, tag := range tags {
		wg.Add(1)
		go func(thisTag string) {
			defer wg.Done()
			//Get the radix tree for the tag
			tree, ok := m.TagPluginMap.Load(thisTag)
			if !ok {
				return
			}
			//Check if the request path matches the static capture path
			longestPrefix, pluginList, ok := tree.(*radix.Tree).LongestPrefix(r.URL.Path)
			if ok {
				if longestPrefix > longestPrefixAcrossAlltags {
					longestPrefixAcrossAlltags = longestPrefix
					handler = pluginList.([]*Plugin)
				}
			}
		}(tag)
	}
	wg.Wait()
	if len(handler) > 0 {
		//Handle the request
		handler[0].HandleRoute(w, r, longestPrefixAcrossAlltags)
		return true
	}
	return false
}
