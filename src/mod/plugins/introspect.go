package plugins

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// LoadPlugin loads a plugin from the plugin directory
func (m *Manager) IsValidPluginFolder(path string) bool {
	_, err := m.GetPluginEntryPoint(path)
	return err == nil
}

/*
LoadPluginSpec loads a plugin specification from the plugin directory
Zoraxy will start the plugin binary or the entry point script
with -introspect flag to get the plugin specification
*/
func (m *Manager) LoadPluginSpec(pluginPath string) (*Plugin, error) {
	pluginEntryPoint, err := m.GetPluginEntryPoint(pluginPath)
	if err != nil {
		return nil, err
	}

	pluginSpec, err := m.GetPluginSpec(pluginEntryPoint)
	if err != nil {
		return nil, err
	}

	err = validatePluginSpec(pluginSpec)
	if err != nil {
		return nil, err
	}

	return &Plugin{
		Spec:    pluginSpec,
		Enabled: false,
	}, nil
}

// GetPluginEntryPoint returns the plugin entry point
func (m *Manager) GetPluginSpec(entryPoint string) (*IntroSpect, error) {
	pluginSpec := IntroSpect{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, entryPoint, "-introspect")
	output, err := cmd.Output()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("plugin introspect timed out")
	}
	if err != nil {
		return nil, err
	}

	// Assuming the output is JSON and needs to be unmarshaled into pluginSpec
	err = json.Unmarshal(output, &pluginSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal plugin spec: %v", err)
	}

	return &pluginSpec, nil
}
