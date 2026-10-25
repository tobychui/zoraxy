package uptime

import (
	"strings"
	"sync"
	"time"

	"imuslab.com/zoraxy/mod/info/logger"
)

const (
	LOG_MODULE_NAME           = "uptime-monitor"
	UPTIME_MONITOR_USER_AGENT = "zoraxy-uptime/1.4"
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
	ID                string
	Name              string
	URL               string
	Protocol          string
	ProxyType         ProxyType
	SkipTlsValidation bool
	HealthCheckURI    string //Optional URI path appended to URL for the health check. If empty, "/" is used.
	CheckMethod       string //HTTP method used for the check (GET, HEAD, OPTIONS). Empty = GET
	CheckInterval     int    //Check interval in seconds. <=0 = use the monitor default
	MaxRecords        int    //Max number of records kept. <=0 = use the monitor default
}

type Config struct {
	Targets              []*Target
	Interval             int //Check interval for online targets in seconds (default 300)
	MaxRecordsStore      int
	OnlineStateNotify    func(upstreamIP string, isOnline bool)
	Logger               *logger.Logger
	Verbal               bool
	OfflineCheckInterval int //Check interval for offline targets in seconds (default 30)
	OfflineCheckTimeout  int //HTTP timeout for offline target checks in seconds (default 10)
}

type Monitor struct {
	Config          *Config
	OnlineStatusLog map[string][]*Record
	logMutex        sync.RWMutex //Mutex for OnlineStatusLog map access

	// Separate lists for online and offline targets with dedicated tickers
	onlineTargets  []*Target
	offlineTargets []*Target
	targetMutex    sync.Mutex //Mutex for online/offline target list access
	lastCheck      map[string]time.Time
}

// normalizeMethod returns a supported HTTP method, defaulting to GET
func normalizeMethod(method string) string {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case "HEAD":
		return "HEAD"
	case "OPTIONS":
		return "OPTIONS"
	default:
		return "GET"
	}
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
