package streamproxy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
)

func CloneConfig(config *ProxyRelayInstance) (*ProxyRelayInstance, error) {
	if config == nil {
		return nil, nil
	}

	js, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	cloned := &ProxyRelayInstance{}
	if err := json.Unmarshal(js, cloned); err != nil {
		return nil, err
	}

	cloned.tcpStopChan = nil
	cloned.udpStopChan = nil
	cloned.udpClientMap = sync.Map{}
	cloned.aTobAccumulatedByteTransfer = atomic.Int64{}
	cloned.bToaAccumulatedByteTransfer = atomic.Int64{}
	cloned.aTobAccumulatedByteTransfer.Store(0)
	cloned.bToaAccumulatedByteTransfer.Store(0)

	return cloned, nil
}

func (m *Manager) ReplaceConfigsFromSync(configs []*ProxyRelayInstance) error {
	previousState := map[string]struct {
		Running   bool
		AutoStart bool
	}{}
	for _, config := range m.Configs {
		if config != nil {
			previousState[config.UUID] = struct {
				Running   bool
				AutoStart bool
			}{
				Running:   config.Running,
				AutoStart: config.AutoStart,
			}
		}
		if config != nil && config.IsRunning() {
			config.Stop()
		}
	}

	configFiles, err := filepath.Glob(filepath.Join(m.Options.ConfigStore, "*.config"))
	if err != nil {
		return err
	}
	for _, configFile := range configFiles {
		if err := os.Remove(configFile); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	newConfigs := make([]*ProxyRelayInstance, 0, len(configs))
	for _, incomingConfig := range configs {
		clonedConfig, err := CloneConfig(incomingConfig)
		if err != nil {
			return err
		}
		if clonedConfig == nil {
			continue
		}
		clonedConfig.parent = m
		if state, ok := previousState[clonedConfig.UUID]; ok {
			clonedConfig.Running = state.Running
			clonedConfig.AutoStart = state.AutoStart
		}
		newConfigs = append(newConfigs, clonedConfig)
	}

	sort.Slice(newConfigs, func(i, j int) bool {
		return newConfigs[i].UUID < newConfigs[j].UUID
	})

	m.Configs = newConfigs
	m.SaveConfigToDatabase()

	for _, config := range m.Configs {
		if config != nil && config.Running && m.isLocallyAssigned(config.AssignedNodeID) {
			config.Running = false
			if err := config.Start(); err != nil {
				return err
			}
			if config.AutoStart {
				config.AutoStart = false
			}
		} else if config != nil && config.Running {
			config.Running = false
		}
	}

	m.SaveConfigToDatabase()
	return nil
}
