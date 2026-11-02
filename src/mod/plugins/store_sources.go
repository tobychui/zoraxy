package plugins

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"imuslab.com/zoraxy/mod/utils"
)

/*
	Plugin Store Sources

	Zoraxy ships with a built-in list of official plugin store index URLs
	(set in ManagerOptions.PluginStoreURLs on startup). Users can register
	additional community plugin store index URLs, which are persisted
	in the database and merged with the official ones on sync.
*/

const dbKeyCustomStoreURLs = "custom_store_urls"

type StoreSource struct {
	URL      string `json:"url"`      //The plugin store index URL
	Official bool   `json:"official"` //Whether this is a built-in (official) source
}

// getCustomStoreURLs returns the user-defined plugin store URLs from database
func (m *Manager) getCustomStoreURLs() []string {
	customURLs := []string{}
	err := m.Options.Database.Read("plugins", dbKeyCustomStoreURLs, &customURLs)
	if err != nil {
		return []string{}
	}
	return customURLs
}

// saveCustomStoreURLs persists the user-defined plugin store URLs to database
func (m *Manager) saveCustomStoreURLs(urls []string) error {
	return m.Options.Database.Write("plugins", dbKeyCustomStoreURLs, urls)
}

// isOfficialStoreURL checks if the given URL is one of the built-in store URLs
func (m *Manager) isOfficialStoreURL(targetURL string) bool {
	for _, url := range m.Options.PluginStoreURLs {
		if url == targetURL {
			return true
		}
	}
	return false
}

// GetAllStoreURLs returns the merged list of official and user-defined store URLs
func (m *Manager) GetAllStoreURLs() []string {
	allURLs := []string{}
	allURLs = append(allURLs, m.Options.PluginStoreURLs...)
	for _, customURL := range m.getCustomStoreURLs() {
		if !m.isOfficialStoreURL(customURL) {
			allURLs = append(allURLs, customURL)
		}
	}
	return allURLs
}

// ListStoreSources returns all registered plugin store sources
func (m *Manager) ListStoreSources() []*StoreSource {
	sources := []*StoreSource{}
	for _, url := range m.Options.PluginStoreURLs {
		sources = append(sources, &StoreSource{URL: url, Official: true})
	}
	for _, customURL := range m.getCustomStoreURLs() {
		if !m.isOfficialStoreURL(customURL) {
			sources = append(sources, &StoreSource{URL: customURL, Official: false})
		}
	}
	return sources
}

// AddStoreSource registers a new community plugin store index URL
func (m *Manager) AddStoreSource(sourceURL string) error {
	sourceURL = strings.TrimSpace(sourceURL)
	parsedURL, err := url.Parse(sourceURL)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return fmt.Errorf("invalid plugin store URL: %s", sourceURL)
	}

	//Check for duplicates
	for _, existingURL := range m.GetAllStoreURLs() {
		if existingURL == sourceURL {
			return fmt.Errorf("plugin store source already exists: %s", sourceURL)
		}
	}

	//Make sure the URL points to a valid plugin store index
	_, err = m.getPluginListFromURL(sourceURL)
	if err != nil {
		return fmt.Errorf("unable to load plugin index from source: %w", err)
	}

	customURLs := append(m.getCustomStoreURLs(), sourceURL)
	return m.saveCustomStoreURLs(customURLs)
}

// RemoveStoreSource removes a community plugin store index URL
func (m *Manager) RemoveStoreSource(sourceURL string) error {
	if m.isOfficialStoreURL(sourceURL) {
		return fmt.Errorf("cannot remove built-in plugin store source")
	}

	customURLs := m.getCustomStoreURLs()
	newURLs := []string{}
	found := false
	for _, customURL := range customURLs {
		if customURL == sourceURL {
			found = true
			continue
		}
		newURLs = append(newURLs, customURL)
	}
	if !found {
		return fmt.Errorf("plugin store source not found: %s", sourceURL)
	}
	return m.saveCustomStoreURLs(newURLs)
}

/*
	Handlers for Plugin Store Sources
*/

// HandleListStoreSources lists all the registered plugin store sources
func (m *Manager) HandleListStoreSources(w http.ResponseWriter, r *http.Request) {
	sources := m.ListStoreSources()
	js, _ := json.Marshal(sources)
	utils.SendJSONResponse(w, string(js))
}

// HandleAddStoreSource adds a new community plugin store source
func (m *Manager) HandleAddStoreSource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "Method not allowed")
		return
	}

	sourceURL, err := utils.PostPara(r, "url")
	if err != nil {
		utils.SendErrorResponse(w, "url is required")
		return
	}

	err = m.AddStoreSource(sourceURL)
	if err != nil {
		utils.SendErrorResponse(w, "Failed to add plugin store source: "+err.Error())
		return
	}

	//Resync the downloadable plugin list to include the new source
	err = m.UpdateDownloadablePluginList()
	if err != nil {
		m.Log("Failed to resync plugin list after adding source", err)
	}

	utils.SendOK(w)
}

// HandleRemoveStoreSource removes a community plugin store source
func (m *Manager) HandleRemoveStoreSource(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, "Method not allowed")
		return
	}

	sourceURL, err := utils.PostPara(r, "url")
	if err != nil {
		utils.SendErrorResponse(w, "url is required")
		return
	}

	err = m.RemoveStoreSource(sourceURL)
	if err != nil {
		utils.SendErrorResponse(w, "Failed to remove plugin store source: "+err.Error())
		return
	}

	//Resync the downloadable plugin list to drop plugins from the removed source
	err = m.UpdateDownloadablePluginList()
	if err != nil {
		m.Log("Failed to resync plugin list after removing source", err)
	}

	utils.SendOK(w)
}
