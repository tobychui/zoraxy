package uptime

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/utils"
)

// Create a new uptime monitor
func NewUptimeMonitor(config *Config) (*Monitor, error) {
	//Create new monitor object
	thisMonitor := Monitor{
		Config:          config,
		OnlineStatusLog: map[string][]*Record{},
		onlineTargets:   make([]*Target, len(config.Targets)),
		offlineTargets:  []*Target{},
		lastCheck:       map[string]time.Time{},
	}

	// All targets start in the online list
	copy(thisMonitor.onlineTargets, config.Targets)

	if config.Logger == nil {
		//Use default fmt to log if logger is nil
		config.Logger, _ = logger.NewFmtLogger()
	}

	if len(config.Targets) > 0 {
		//Initialize the OnlineStatusLog map with empty slices for each target
		for _, target := range config.Targets {
			thisMonitor.OnlineStatusLog[target.ID] = []*Record{}
		}
	}

	if config.OnlineStateNotify == nil {
		//Use default notify function if not provided
		config.OnlineStateNotify = defaultNotify
	}

	// Apply defaults for adaptive interval settings
	if config.OfflineCheckInterval <= 0 {
		config.OfflineCheckInterval = 30 // 30 seconds for offline targets
	}
	if config.OfflineCheckTimeout <= 0 {
		config.OfflineCheckTimeout = 5 // 5 seconds timeout for offline target checks
	}

	// Run initial check for all targets (via the online ticker path)
	thisMonitor.pingTargets(time.Duration(config.Interval)*time.Second, config.Targets, true)

	// Start the online target ticker
	// Ticks at a fine granularity; each target is only checked when its own interval elapsed
	onlineTicker := time.NewTicker(5 * time.Second)
	go func() {
		for range onlineTicker.C {
			thisMonitor.masterPingTargets(false)
		}
	}()

	// Start the offline target ticker (faster interval)
	offlineTicker := time.NewTicker(time.Duration(config.OfflineCheckInterval) * time.Second)
	go func() {
		for range offlineTicker.C {
			thisMonitor.shadowPingTargets()
		}
	}()

	return &thisMonitor, nil
}

// shadowPingTargets is a higher frequency check that only pings targets currently in the offline
// list using the offline timeout setting, these will not go into the online status log
func (m *Monitor) shadowPingTargets() {
	m.targetMutex.Lock()
	var targets []*Target = m.offlineTargets
	m.targetMutex.Unlock()
	m.pingTargets(time.Duration(m.Config.OfflineCheckTimeout)*time.Second, targets, false)
}

// masterPingTargets is the main check that pings all targets and updates the online status log,
// this is called by the main ticker and also externally after config changes
// If force is false, only targets whose own check interval has elapsed are checked.
func (m *Monitor) masterPingTargets(force bool) {
	m.targetMutex.Lock()
	targets := []*Target{}
	now := time.Now()
	for _, t := range m.Config.Targets {
		interval := t.CheckInterval
		if interval <= 0 {
			interval = m.Config.Interval
		}
		last, ok := m.lastCheck[t.ID]
		// 1s slack so ticker jitter doesn't delay a check by a whole tick
		if force || !ok || now.Sub(last) >= time.Duration(interval)*time.Second-time.Second {
			targets = append(targets, t)
		}
	}
	m.targetMutex.Unlock()
	m.pingTargets(5*time.Second, targets, true)
}

// pingTargets pings a list of targets with a given timeout and whether to log the results in the online status log
func (m *Monitor) pingTargets(timeout time.Duration, targets []*Target, requireOnlineStatusLog bool) {
	for _, target := range targets {
		if target.Protocol != "http" && target.Protocol != "https" {
			m.Config.Logger.PrintAndLog(LOG_MODULE_NAME, "Unknown protocol: "+target.Protocol, errors.New("unsupported protocol"))
			continue
		}

		if requireOnlineStatusLog {
			m.targetMutex.Lock()
			m.lastCheck[target.ID] = time.Now()
			m.targetMutex.Unlock()
		}

		// Run each check in a separate goroutine to avoid blocking
		go func(target *Target) {
			// Get online status and latency, and handle state transitions and logging accordingly
			// this also handle notifying the dynamic proxy to take down or bring up the upstream when state changes
			online, latency, statusCode := m.getWebsiteStatusWithLatency(target, timeout)
			now := time.Now().Unix()
			thisRecord := Record{
				Timestamp:  now,
				ID:         target.ID,
				Name:       target.Name,
				URL:        buildHealthCheckURL(target.URL, target.HealthCheckURI),
				Protocol:   target.Protocol,
				Online:     online,
				StatusCode: statusCode,
				Latency:    latency,
			}

			// Handle state transitions
			if !online && targetExistsInList(m.onlineTargets, target.ID) {
				// Was in online list but check failed -> move to offline list
				m.moveTargetToOffline(target)
				m.Config.Logger.PrintAndLog(LOG_MODULE_NAME, "Target "+target.Name+" ("+target.URL+") went offline, switching to fast check interval", nil)
			} else if online && targetExistsInList(m.offlineTargets, target.ID) {
				// Was in offline list but check succeeded -> move back to online list
				m.moveTargetToOnline(target)
				m.Config.Logger.PrintAndLog(LOG_MODULE_NAME, "Target "+target.Name+" ("+target.URL+") is back online, reverting to normal check interval", nil)
			}

			// Append record to the status log if required
			if requireOnlineStatusLog {
				m.logMutex.Lock()
				thisRecords, ok := m.OnlineStatusLog[target.ID]
				if !ok {
					m.OnlineStatusLog[target.ID] = []*Record{&thisRecord}
				} else {
					thisRecords = append(thisRecords, &thisRecord)
					maxRecords := target.MaxRecords
					if maxRecords <= 0 {
						maxRecords = m.Config.MaxRecordsStore
					}
					for len(thisRecords) > maxRecords && len(thisRecords) > 0 {
						thisRecords = thisRecords[1:]
					}
					m.OnlineStatusLog[target.ID] = thisRecords
				}
				m.logMutex.Unlock()
			}
		}(target)
	}
}

// moveTargetToOffline moves a target from the online list to the offline list
func (m *Monitor) moveTargetToOffline(target *Target) {
	m.targetMutex.Lock()
	defer m.targetMutex.Unlock()
	m.onlineTargets = removeTargetFromList(m.onlineTargets, target.ID)
	m.offlineTargets = append(m.offlineTargets, target)
}

// moveTargetToOnline moves a target from the offline list to the online list
func (m *Monitor) moveTargetToOnline(target *Target) {
	m.targetMutex.Lock()
	defer m.targetMutex.Unlock()
	m.offlineTargets = removeTargetFromList(m.offlineTargets, target.ID)
	m.onlineTargets = append(m.onlineTargets, target)
}

// removeTargetFromList removes a target by ID from a list and returns the new list
func removeTargetFromList(list []*Target, id string) []*Target {
	for i, t := range list {
		if t.ID == id {
			return append(list[:i], list[i+1:]...)
		}
	}
	return list
}

func targetExistsInList(list []*Target, id string) bool {
	for _, t := range list {
		if t.ID == id {
			return true
		}
	}
	return false
}

// ExecuteUptimeCheck runs an immediate check on all targets (both online and offline).
// This is called externally after config changes.
func (m *Monitor) ExecuteUptimeCheck() {
	m.masterPingTargets(true)
}

func (m *Monitor) UpdateReocrdsAfterConfigChange() { // Populate the Record map with new targets, preserving existing records where possible
	m.logMutex.Lock()
	for _, target := range m.Config.Targets {
		if _, exists := m.OnlineStatusLog[target.ID]; !exists {
			m.OnlineStatusLog[target.ID] = []*Record{}
		}
	}

	// Clean up records for targets that no longer exist
	for id := range m.OnlineStatusLog {
		idExistsInNewTargets := false
		for _, target := range m.Config.Targets {
			if target.ID == id {
				idExistsInNewTargets = true
				break
			}
		}
		if !idExistsInNewTargets {
			delete(m.OnlineStatusLog, id)
		}
	}
	m.logMutex.Unlock()
}

// SetTargets replaces the full target list atomically.
// New targets are placed in the online list; targets that were previously
// offline and still exist in the new set are kept in the offline list.
func (m *Monitor) SetTargets(newTargets []*Target) {
	m.targetMutex.Lock()
	defer m.targetMutex.Unlock()

	// Build a set of previously-offline target IDs
	offlineIDs := make(map[string]bool, len(m.offlineTargets))
	for _, t := range m.offlineTargets {
		offlineIDs[t.ID] = true
	}

	// Distribute new targets into online/offline lists
	var newOnline, newOffline []*Target
	for _, t := range newTargets {
		if offlineIDs[t.ID] {
			newOffline = append(newOffline, t)
		} else {
			newOnline = append(newOnline, t)
		}
	}

	if newOnline == nil {
		newOnline = []*Target{}
	}
	if newOffline == nil {
		newOffline = []*Target{}
	}

	m.onlineTargets = newOnline
	m.offlineTargets = newOffline
	m.Config.Targets = newTargets
	m.UpdateReocrdsAfterConfigChange()
}

func (m *Monitor) AddTargetToMonitor(target *Target) {
	// Add target to Config
	m.Config.Targets = append(m.Config.Targets, target)

	// New targets are assumed online
	m.targetMutex.Lock()
	m.onlineTargets = append(m.onlineTargets, target)
	m.targetMutex.Unlock()

	// Add target to OnlineStatusLog
	m.logMutex.Lock()
	m.OnlineStatusLog[target.ID] = []*Record{}
	m.logMutex.Unlock()
}

func (m *Monitor) RemoveTargetFromMonitor(targetId string) {
	// Remove target from Config
	for i, target := range m.Config.Targets {
		if target.ID == targetId {
			m.Config.Targets = append(m.Config.Targets[:i], m.Config.Targets[i+1:]...)
			break
		}
	}

	// Remove from both online and offline lists
	m.targetMutex.Lock()
	m.onlineTargets = removeTargetFromList(m.onlineTargets, targetId)
	m.offlineTargets = removeTargetFromList(m.offlineTargets, targetId)
	m.targetMutex.Unlock()

	// Remove target from OnlineStatusLog
	m.logMutex.Lock()
	delete(m.OnlineStatusLog, targetId)
	m.logMutex.Unlock()
}

// Scan the config target. If a target exists in m.OnlineStatusLog no longer
// exists in m.Monitor.Config.Targets, it remove it from the log as well.
func (m *Monitor) CleanRecords() {
	// Create a set of IDs for all targets in the config
	targetIDs := make(map[string]bool)
	for _, target := range m.Config.Targets {
		targetIDs[target.ID] = true
	}

	// Iterate over all log entries and remove any that have a target ID that
	// is not in the set of current target IDs
	m.logMutex.Lock()
	for id := range m.OnlineStatusLog {
		_, idExistsInTargets := targetIDs[id]
		if !idExistsInTargets {
			delete(m.OnlineStatusLog, id)
		}
	}
	m.logMutex.Unlock()
}

/*
	Web Interface Handler
*/

func (m *Monitor) HandleUptimeLogRead(w http.ResponseWriter, r *http.Request) {
	id, _ := utils.GetPara(r, "id")
	if id == "" {
		m.logMutex.RLock()
		js, _ := json.Marshal(m.OnlineStatusLog)
		m.logMutex.RUnlock()
		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	} else {
		//Check if that id exists
		m.logMutex.RLock()
		log, ok := m.OnlineStatusLog[id]
		m.logMutex.RUnlock()
		if !ok {
			http.NotFound(w, r)
			return
		}

		js, _ := json.MarshalIndent(log, "", " ")
		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	}

}
