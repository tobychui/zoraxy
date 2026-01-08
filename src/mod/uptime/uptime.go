package uptime

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/utils"
)

// Create a new uptime monitor
func NewUptimeMonitor(config *Config) (*Monitor, error) {
	//Create new monitor object
	thisMonitor := Monitor{
		Config:          config,
		OnlineStatusLog: map[string][]*Record{},
	}

	if config.Logger == nil {
		//Use default fmt to log if logger is not provided
		config.Logger, _ = logger.NewFmtLogger()
	}

	if config.OnlineStateNotify == nil {
		//Use default notify function if not provided
		config.OnlineStateNotify = defaultNotify
	}

	//Start the endpoint listener
	ticker := time.NewTicker(time.Duration(config.Interval) * time.Second)
	done := make(chan bool)

	//Start the uptime check once first before entering loop
	thisMonitor.ExecuteUptimeCheck()

	go func() {
		for {
			select {
			case <-done:
				return
			case t := <-ticker.C:
				thisMonitor.Config.Logger.PrintAndLog(logModuleName, "Uptime updated - "+strconv.Itoa(int(t.Unix())), nil)
				thisMonitor.ExecuteUptimeCheck()
			}
		}
	}()

	return &thisMonitor, nil
}

func (m *Monitor) ExecuteUptimeCheck() {
	if m.runningUptimeChecks {
		//Prevent overlapping uptime checks
		m.Config.Logger.PrintAndLog(logModuleName, "Another uptime check is running in the background. Skipped", nil)
		return
	}
	m.runningUptimeChecks = true
	defer func() {
		m.runningUptimeChecks = false
	}()
	for _, target := range m.Config.Targets {
		//For each target to check online, do the following
		var thisRecord Record
		if target.Protocol == "http" || target.Protocol == "https" {
			online, laterncy, statusCode := m.getWebsiteStatusWithLatency(target)
			thisRecord = Record{
				Timestamp:  time.Now().Unix(),
				ID:         target.ID,
				Name:       target.Name,
				URL:        target.URL,
				Protocol:   target.Protocol,
				Online:     online,
				StatusCode: statusCode,
				Latency:    laterncy,
			}

		} else {
			m.Config.Logger.PrintAndLog(logModuleName, "Unknown protocol: "+target.Protocol, errors.New("unsupported protocol"))
			continue
		}

		m.logMutex.Lock()
		thisRecords, ok := m.OnlineStatusLog[target.ID]
		if !ok {
			//First record. Create the array
			m.OnlineStatusLog[target.ID] = []*Record{&thisRecord}
		} else {
			//Append to the previous record
			thisRecords = append(thisRecords, &thisRecord)

			//Check if the record is longer than the logged record. If yes, clear out the old records
			if len(thisRecords) > m.Config.MaxRecordsStore {
				thisRecords = thisRecords[1:]
			}

			m.OnlineStatusLog[target.ID] = thisRecords
		}
		m.logMutex.Unlock()
	}
}

func (m *Monitor) AddTargetToMonitor(target *Target) {
	// Add target to Config
	m.Config.Targets = append(m.Config.Targets, target)

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

/*
	Utilities
*/

// Get website stauts with latency given URL, return is conn succ and its latency and status code
func (m *Monitor) getWebsiteStatusWithLatency(target *Target) (bool, int64, int) {
	start := time.Now().UnixNano() / int64(time.Millisecond)
	statusCode, err := getWebsiteStatus(target.URL)
	end := time.Now().UnixNano() / int64(time.Millisecond)
	if err != nil {
		m.Config.Logger.PrintAndLog(logModuleName, "Ping upstream timeout. Assume offline", err)

		// Check if this is the first record
		// sometime after startup the first check may fail due to network issues
		// we will log it as failed but not notify dynamic proxy to take down the upstream
		m.logMutex.RLock()
		records, ok := m.OnlineStatusLog[target.ID]
		m.logMutex.RUnlock()
		if !ok || len(records) == 0 {
			return false, 0, 0
		}

		// Otherwise assume offline
		m.Config.OnlineStateNotify(target.URL, false)
		return false, 0, 0
	}

	diff := end - start
	succ := false
	if statusCode >= 200 && statusCode < 300 {
		//OK
		succ = true
	} else if statusCode >= 300 && statusCode < 400 {
		//Redirection code
		succ = true
	} else {
		succ = false
	}
	m.Config.OnlineStateNotify(target.URL, true)
	return succ, diff, statusCode

}

func getWebsiteStatus(url string) (int, error) {
	// Create a one-time use cookie jar to store cookies
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return 0, err
	}

	client := http.Client{
		Jar:     jar,
		Timeout: 5 * time.Second,
	}

	req, _ := http.NewRequest("GET", url, nil)
	req.Header = http.Header{
		"User-Agent": {"zoraxy-uptime/1.1"},
	}

	resp, err := client.Do(req)
	if err != nil {
		//Try replace the http with https and vise versa
		rewriteURL := ""
		if strings.Contains(url, "https://") {
			rewriteURL = strings.ReplaceAll(url, "https://", "http://")
		} else if strings.Contains(url, "http://") {
			rewriteURL = strings.ReplaceAll(url, "http://", "https://")
		}

		req, _ := http.NewRequest("GET", rewriteURL, nil)
		req.Header = http.Header{
			"User-Agent": {"zoraxy-uptime/1.1"},
		}

		resp, err := client.Do(req)
		if err != nil {
			if strings.Contains(err.Error(), "http: server gave HTTP response to HTTPS client") {
				//Invalid downstream reverse proxy settings, but it is online
				//return SSL handshake failed
				return 525, nil
			}
			return 0, err
		}
		defer resp.Body.Close()
		status_code := resp.StatusCode
		return status_code, nil
	}
	defer resp.Body.Close()
	status_code := resp.StatusCode
	return status_code, nil
}
