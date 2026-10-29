package plugins

import (
	"bytes"
	"encoding/json"
	"path/filepath"

	"imuslab.com/zoraxy/mod/plugins/zoraxy_plugin"
)

const inspectPolicyTable = "plugin_inspection"

// persistedInspector retains the validated control plane, not an inspection verdict.
// The binding is needed when even plugin introspection fails on a cold restart.
type persistedInspector struct {
	RootDir string                   `json:"root_dir"`
	Spec    zoraxy_plugin.IntroSpect `json:"spec"`
	Policy  json.RawMessage          `json:"policy"`
}

func (m *Manager) newInspectPolicyCache(p *Plugin) *inspectPolicyCache {
	c := &inspectPolicyCache{}
	if m.Options == nil || m.Options.Database == nil {
		return c
	}
	db := m.Options.Database
	if err := db.NewTable(inspectPolicyTable); err != nil {
		m.logInspect("Create inspection policy table", err)
		return c
	}
	var saved persistedInspector
	if db.KeyExists(inspectPolicyTable, p.Spec.ID) && db.Read(inspectPolicyTable, p.Spec.ID, &saved) == nil {
		policy, err := parseInspectPolicy(saved.Policy)
		if err != nil {
			m.logInspect("Invalid persisted inspection policy: "+p.Spec.ID, err)
		} else {
			c.current = policy
		}
	}
	// Copy the spec once; requests never read the mutable runtime Plugin.
	specData, _ := json.Marshal(p.Spec)
	var spec zoraxy_plugin.IntroSpect
	json.Unmarshal(specData, &spec)
	root, _ := filepath.Abs(p.RootDir)
	var persisted []byte
	c.persist = func(policy *inspectPolicy) error {
		data, err := json.Marshal(policy.RequestInspectPolicy)
		if err != nil {
			return err
		}
		if bytes.Equal(data, persisted) {
			return nil
		}
		err = db.Write(inspectPolicyTable, spec.ID, persistedInspector{RootDir: root, Spec: spec, Policy: data})
		if err == nil {
			persisted = data
		}
		return err
	}
	return c
}

// restoreInspector restores only previously enabled inspectors with a valid saved
// policy, bound to this exact directory. Other plugin forwarding remains disabled.
func (m *Manager) restoreInspector(pluginPath string) *Plugin {
	entries, err := m.Options.Database.ListTable(inspectPolicyTable)
	if err != nil {
		return nil
	}
	root, _ := filepath.Abs(pluginPath)
	for _, entry := range entries {
		var saved persistedInspector
		if json.Unmarshal(entry[1], &saved) != nil || saved.RootDir != root || saved.Spec.ID != string(entry[0]) || saved.Spec.RequestInspect == "" || saved.Spec.RequestInspectPolicy == "" {
			continue
		}
		if _, err := parseInspectPolicy(saved.Policy); err != nil {
			m.logInspect("Invalid persisted inspection policy: "+saved.Spec.ID, err)
			continue
		}
		if !m.GetPluginPreviousEnableState(saved.Spec.ID) {
			return nil
		}
		return &Plugin{RootDir: pluginPath, Spec: &saved.Spec, inspectFailed: true}
	}
	return nil
}
