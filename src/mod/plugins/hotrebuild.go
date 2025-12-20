package plugins

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// RebuildPlugin rebuilds a plugin by executing its Makefile or building with Go
func (m *Manager) RebuildPlugin(pluginID string) error {
	plugin, err := m.GetPluginByID(pluginID)
	if err != nil {
		return err
	}

	// Check if Makefile or main.go exists
	makefilePath := filepath.Join(plugin.RootDir, "Makefile")
	mainGoPath := filepath.Join(plugin.RootDir, "main.go")

	hasMakefile := false
	hasMainGo := false

	if _, err := os.Stat(makefilePath); err == nil {
		hasMakefile = true
	}
	if _, err := os.Stat(mainGoPath); err == nil {
		hasMainGo = true
	}

	if !hasMakefile && !hasMainGo {
		return errors.New("plugin does not contain a Makefile or main.go file")
	}

	// Remember if the plugin was enabled
	wasEnabled := plugin.Enabled

	// Stop the plugin if it's running
	if wasEnabled {
		m.Log("Stopping plugin "+plugin.Spec.Name+" for rebuild", nil)
		err = m.StopPlugin(pluginID)
		if err != nil {
			return fmt.Errorf("failed to stop plugin: %w", err)
		}
	}

	// Build the plugin
	var cmd *exec.Cmd
	if hasMakefile {
		// Use make to build
		m.Log("Building plugin "+plugin.Spec.Name+" using Makefile", nil)
		cmd = exec.Command("make")
		cmd.Dir = plugin.RootDir
	} else {
		// Check if go is installed
		goPath, err := exec.LookPath("go")
		if err != nil {
			return errors.New("Go compiler not found. Please install Go to build this plugin")
		}

		m.Log("Building plugin "+plugin.Spec.Name+" using Go build", nil)

		// Build the binary
		outputBinary := filepath.Base(plugin.RootDir)
		if runtime.GOOS == "windows" {
			outputBinary += ".exe"
		}

		fmt.Println("Building plugin binary at:", outputBinary)

		cmd = exec.Command(goPath, "build", "-o", outputBinary, ".")
		cmd.Dir = plugin.RootDir
	}

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		buildError := fmt.Sprintf("Build failed: %s\n\nBuild output:\n%s", err.Error(), string(output))
		return errors.New(buildError)
	}

	m.Log("Plugin \""+plugin.Spec.Name+"\" rebuilt successfully", nil)

	// Restart the plugin if it was enabled before
	if wasEnabled {
		m.Log("Restarting plugin \""+plugin.Spec.Name+"\"", nil)
		err = m.StartPlugin(pluginID)
		if err != nil {
			return fmt.Errorf("plugin rebuilt but failed to restart: %w", err)
		}
		m.UpdateTagsToPluginMaps()
	}

	return nil
}
