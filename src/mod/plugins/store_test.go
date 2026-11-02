package plugins

import (
	"testing"

	"imuslab.com/zoraxy/mod/database"
)

func TestUpdateDownloadablePluginList(t *testing.T) {
	mockManager := &Manager{
		Options: &ManagerOptions{
			DownloadablePluginCache: []*DownloadablePlugin{},
			PluginStoreURLs:         []string{},
			Database: &database.Database{
				Backend: &fakeBackend{},
			},
		},
	}

	// Inject a mock URL for testing
	mockManager.Options.PluginStoreURLs = []string{"https://raw.githubusercontent.com/aroz-online/zoraxy-official-plugins/refs/heads/main/directories/index2.json"}

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

	pluginList, err := mockManager.getPluginListFromURL("https://raw.githubusercontent.com/aroz-online/zoraxy-official-plugins/refs/heads/main/directories/index2.json")
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

type fakeBackend struct{}

func (f *fakeBackend) NewTable(tableName string) error {
	return nil
}

func (f *fakeBackend) TableExists(tableName string) bool {
	return false
}

func (f *fakeBackend) DropTable(tableName string) error {
	return nil
}

func (f *fakeBackend) Write(tableName string, key string, value any) error {
	return nil
}

func (f *fakeBackend) Read(tableName string, key string, assignee any) error {
	return nil
}

func (f *fakeBackend) KeyExists(tableName string, key string) bool {
	return false
}

func (f *fakeBackend) Delete(tableName string, key string) error {
	return nil
}

func (f *fakeBackend) ListTable(tableName string) ([][][]byte, error) {
	return [][][]byte{}, nil
}

func (f *fakeBackend) Close() {}
