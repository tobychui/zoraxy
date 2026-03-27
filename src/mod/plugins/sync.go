package plugins

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"imuslab.com/zoraxy/mod/utils"
)

// ExportPluginArchive serializes the plugin directory into a zip archive.
func ExportPluginArchive(pluginDir string) ([]byte, error) {
	buffer := &bytes.Buffer{}
	zipWriter := zip.NewWriter(buffer)

	if !utils.FileExists(pluginDir) {
		if err := zipWriter.Close(); err != nil {
			return nil, err
		}
		return buffer.Bytes(), nil
	}

	err := filepath.Walk(pluginDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == pluginDir {
			return nil
		}

		relativePath, err := filepath.Rel(pluginDir, path)
		if err != nil {
			return err
		}
		relativePath = filepath.ToSlash(relativePath)

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = relativePath
		header.Method = zip.Deflate

		if info.IsDir() {
			header.Name += "/"
		}

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(writer, file)
		return err
	})
	if err != nil {
		zipWriter.Close()
		return nil, err
	}

	if err := zipWriter.Close(); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

// ImportPluginArchive replaces the plugin directory with the provided zip archive.
func ImportPluginArchive(pluginDir string, archiveData []byte) error {
	if err := os.RemoveAll(pluginDir); err != nil {
		return err
	}
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return err
	}

	zipReader, err := zip.NewReader(bytes.NewReader(archiveData), int64(len(archiveData)))
	if err != nil {
		return err
	}

	for _, archiveFile := range zipReader.File {
		cleanName := filepath.Clean(archiveFile.Name)
		targetPath := filepath.Join(pluginDir, cleanName)
		relativePath, err := filepath.Rel(pluginDir, targetPath)
		if err != nil {
			return err
		}
		if relativePath == ".." || len(relativePath) >= 3 && relativePath[:3] == ".."+string(os.PathSeparator) {
			return errors.New("invalid plugin archive path")
		}

		if archiveFile.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, archiveFile.Mode()); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}

		reader, err := archiveFile.Open()
		if err != nil {
			return err
		}

		targetFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, archiveFile.Mode())
		if err != nil {
			reader.Close()
			return err
		}

		_, copyErr := io.Copy(targetFile, reader)
		reader.Close()
		closeErr := targetFile.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}

	return nil
}

// ReplacePluginsFromSync replaces plugins on disk, reloads them, and reapplies group configuration.
func (m *Manager) ReplacePluginsFromSync(archiveData []byte, pluginGroupsConfig json.RawMessage) error {
	if m == nil {
		return errors.New("plugin manager is not initialized")
	}

	m.loadedPluginsMutex.RLock()
	pluginsToStop := make([]*Plugin, 0, len(m.LoadedPlugins))
	for _, plugin := range m.LoadedPlugins {
		pluginsToStop = append(pluginsToStop, plugin)
	}
	m.loadedPluginsMutex.RUnlock()

	for _, plugin := range pluginsToStop {
		if plugin.Enabled {
			if err := m.StopPlugin(plugin.Spec.ID); err != nil {
				return err
			}
		}
	}

	if err := ImportPluginArchive(m.Options.PluginDir, archiveData); err != nil {
		return err
	}

	if m.Options.PluginGroupsConfig != "" {
		if err := os.MkdirAll(filepath.Dir(m.Options.PluginGroupsConfig), 0755); err != nil {
			return err
		}

		if len(pluginGroupsConfig) == 0 {
			pluginGroupsConfig = json.RawMessage(`{}`)
		}
		if err := os.WriteFile(m.Options.PluginGroupsConfig, pluginGroupsConfig, 0644); err != nil {
			return err
		}
	}

	m.loadedPluginsMutex.Lock()
	m.LoadedPlugins = make(map[string]*Plugin)
	m.loadedPluginsMutex.Unlock()

	return m.LoadPluginsFromDisk()
}
