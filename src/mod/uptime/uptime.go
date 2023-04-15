package uptime

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"imuslab.com/zoraxy/mod/utils"
)

type Record struct {
	Timestamp  int64
	ID         string
	Name       string
	URL        string
	Protocol   string
	Online     bool
	StatusCode int
	Latency    int64
}

type Target struct {
	ID       string
	Name     string
	URL      string
	Protocol string
}

type Config struct {
	Targets         []*Target
	Interval        int
	MaxRecordsStore int
}

type Monitor struct {
	Config          *Config
	OnlineStatusLog map[string][]*Record
}

// Default configs
var exampleTarget = Target{
	ID:       "example",
	Name:     "Example",
	URL:      "example.com",
	Protocol: "https",
}

//Create a new uptime monitor
func NewUptimeMonitor(config *Config) (*Monitor, error) {
	//Create new monitor object
	thisMonitor := Monitor{
		Config:          config,
		OnlineStatusLog: map[string][]*Record{},
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
				log.Println("Uptime updated - ", t.Unix())
				thisMonitor.ExecuteUptimeCheck()
			}
		}
	}()

	return &thisMonitor, nil
}

func (m *Monitor) ExecuteUptimeCheck() {
	for _, target := range m.Config.Targets {
		//For each target to check online, do the following
		var thisRecord Record
		if target.Protocol == "http" || target.Protocol == "https" {
			online, laterncy, statusCode := getWebsiteStatusWithLatency(target.URL)
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

			//fmt.Println(thisRecord)

		} else {
			log.Println("Unknown protocol: " + target.Protocol + ". Skipping")
			continue
		}

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
	}

	//TODO: Write results to db
}

func (m *Monitor) AddTargetToMonitor(target *Target) {
	// Add target to Config
	m.Config.Targets = append(m.Config.Targets, target)

	// Add target to OnlineStatusLog
	m.OnlineStatusLog[target.ID] = []*Record{}
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
	delete(m.OnlineStatusLog, targetId)
}

//Scan the config target. If a target exists in m.OnlineStatusLog no longer
//exists in m.Monitor.Config.Targets, it remove it from the log as well.
func (m *Monitor) CleanRecords() {
	// Create a set of IDs for all targets in the config
	targetIDs := make(map[string]bool)
	for _, target := range m.Config.Targets {
		targetIDs[target.ID] = true
	}

	// Iterate over all log entries and remove any that have a target ID that
	// is not in the set of current target IDs
	newStatusLog := m.OnlineStatusLog
	for id, _ := range m.OnlineStatusLog {
		_, idExistsInTargets := targetIDs[id]
		if !idExistsInTargets {
			delete(newStatusLog, id)
		}
	}

	m.OnlineStatusLog = newStatusLog
}

/*
	Web Interface Handler
*/

func (m *Monitor) HandleUptimeLogRead(w http.ResponseWriter, r *http.Request) {
	id, _ := utils.GetPara(r, "id")
	if id == "" {
		js, _ := json.Marshal(m.OnlineStatusLog)
		w.Header().Set("Content-Type", "application/json")
		w.Write(js)
	} else {
		//Check if that id exists
		log, ok := m.OnlineStatusLog[id]
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
func getWebsiteStatusWithLatency(url string) (bool, int64, int) {
	start := time.Now().UnixNano() / int64(time.Millisecond)
	statusCode, err := getWebsiteStatus(url)
	end := time.Now().UnixNano() / int64(time.Millisecond)
	if err != nil {
		log.Println(err.Error())
		return false, 0, 0
	} else {
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

		return succ, diff, statusCode
	}

}

func getWebsiteStatus(url string) (int, error) {
	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	status_code := resp.StatusCode
	return status_code, nil
}
