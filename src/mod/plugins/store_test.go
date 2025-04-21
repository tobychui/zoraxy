package plugins

import (
	"testing"
)

func TestUpdateDownloadablePluginList(t *testing.T) {
	mockManager := &Manager{
		Options: &ManagerOptions{
			DownloadablePluginCache: []*DownloadablePlugin{},
			PluginStoreURLs:         []string{},
		},
	}

	//Inject a mock URL for testing
	mockManager.Options.PluginStoreURLs = []string{"https://raw.githubusercontent.com/aroz-online/zoraxy-official-plugins/refs/heads/main/directories/index.json"}

	err := mockManager.UpdateDownloadablePluginList()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(mockManager.Options.DownloadablePluginCache) == 0 {
		t.Fatalf("expected plugin cache to be updated, but it was empty")
	}

	if mockManager.Options.LastSuccPluginSyncTime == 0 {
		t.Fatalf("expected LastSuccPluginSyncTime to be updated, but it was not")
	}
}

func TestGetPluginListFromURL(t *testing.T) {
	mockManager := &Manager{
		Options: &ManagerOptions{
			DownloadablePluginCache: []*DownloadablePlugin{},
			PluginStoreURLs:         []string{},
		},
	}

	pluginList, err := mockManager.getPluginListFromURL("https://raw.githubusercontent.com/aroz-online/zoraxy-official-plugins/refs/heads/main/directories/index.json")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(pluginList) == 0 {
		t.Fatalf("expected plugin list to be populated, but it was empty")
	}

	for _, plugin := range pluginList {
		t.Logf("Plugin: %+v", plugin)
	}
}
