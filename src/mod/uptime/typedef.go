package uptime

import (
	"sync"

	"imuslab.com/zoraxy/mod/info/logger"
)

const (
	logModuleName = "uptime-monitor"
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

type ProxyType string

const (
	ProxyType_Host ProxyType = "Origin Server"
	ProxyType_Vdir ProxyType = "Virtual Directory"
)

type Target struct {
	ID        string
	Name      string
	URL       string
	Protocol  string
	ProxyType ProxyType
}

type Config struct {
	Targets           []*Target
	Interval          int
	MaxRecordsStore   int
	OnlineStateNotify func(upstreamIP string, isOnline bool)
	Logger            *logger.Logger
	Verbal            bool
}

type Monitor struct {
	Config              *Config
	OnlineStatusLog     map[string][]*Record
	logMutex            sync.RWMutex //Mutex for OnlineStatusLog map access
	runningUptimeChecks bool         //To prevent overlapping uptime checks
}

// Default configs
var exampleTarget = Target{
	ID:       "example",
	Name:     "Example",
	URL:      "example.com",
	Protocol: "https",
}

func defaultNotify(upstreamIP string, isOnline bool) {
	// Do nothing
}
