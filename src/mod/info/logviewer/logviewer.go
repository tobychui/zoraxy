package logviewer

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"imuslab.com/zoraxy/mod/utils"
)

type ViewerOption struct {
	RootFolder string //The root folder to scan for log
	Extension  string //The extension the root files use, include the . in your ext (e.g. .log)
}

type Viewer struct {
	option *ViewerOption
}

type LogSummary struct {
	TotalReqests   int64              `json:"total_requests"`
	TotalValid     int64              `json:"total_valid"`
	TotalErrors    int64              `json:"total_errors"`
	LogSource      string             `json:"log_source"`
	RequestMethods map[string]int64   `json:"request_methods"` //Request methods (key: method, value: hit count)
	HitPerDay      map[string]int64   `json:"hit_per_day"`     //Total hit count per day (key: date, value: hit count)
	HiPerSite      map[string][]int64 `json:"hit_per_site"`    //origin to hit count per day (key: origin, value: []int64{hit count per day})
	UniqueIPs      map[string]int64   `json:"unique_ips"`      //Unique IPs per day (key: date, value: unique IP count)
	TopOrigins     map[string]int64   `json:"top_origins"`     //Top origins (key: origin, value: hit count)
	TopUserAgents  map[string]int64   `json:"top_user_agents"` //Top user agents (key: user agent, value: hit count)
	TopPaths       map[string]int64   `json:"top_paths"`       //Top paths (key: path, value: hit count)
	TotalSize      int64              `json:"total_size"`      //Total size of the log file
}

type LogFile struct {
	Title    string
	Filename string
	Fullpath string
	Filesize int64
}

func NewLogViewer(option *ViewerOption) *Viewer {
	return &Viewer{option: option}
}

/*
	Log Request Handlers
*/
//List all the log files in the log folder. Return in map[string]LogFile format
func (v *Viewer) HandleListLog(w http.ResponseWriter, r *http.Request) {
	logFiles := v.ListLogFiles(false)
	js, _ := json.Marshal(logFiles)
	utils.SendJSONResponse(w, string(js))
}

// Read log of a given catergory and filename
// Require GET varaible: file and catergory
func (v *Viewer) HandleReadLog(w http.ResponseWriter, r *http.Request) {
	filename, err := utils.GetPara(r, "file")
	if err != nil {
		utils.SendErrorResponse(w, "invalid filename given")
		return
	}

	filter, err := utils.GetPara(r, "filter")
	if err != nil {
		filter = ""
	}

	content, err := v.LoadLogFile(strings.TrimSpace(filepath.Base(filename)))
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//If filter is given, only return lines that contains the filter string
	if filter != "" {
		lines := strings.Split(content, "\n")
		filteredLines := []string{}
		for _, line := range lines {
			switch filter {
			case "error":
				if strings.Contains(line, ":error]") {
					filteredLines = append(filteredLines, line)
				}
			case "request":
				if strings.Contains(line, "[router:") {
					filteredLines = append(filteredLines, line)
				}
			case "system":
				if strings.Contains(line, "[system:") {
					filteredLines = append(filteredLines, line)
				}
			case "all":
				filteredLines = append(filteredLines, line)
			default:
				if strings.Contains(line, filter) {
					filteredLines = append(filteredLines, line)
				}
			}
		}
		content = strings.Join(filteredLines, "\n")
	}

	utils.SendTextResponse(w, content)
}

func (v *Viewer) HandleReadLogSummary(w http.ResponseWriter, r *http.Request) {
	filename, err := utils.GetPara(r, "file")
	if err != nil {
		utils.SendErrorResponse(w, "invalid filename given")
		return
	}

	summary, err := v.LoadLogSummary(strings.TrimSpace(filepath.Base(filename)))
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendJSONResponse(w, summary)
}

/*
	Log Access Functions
*/

func (v *Viewer) ListLogFiles(showFullpath bool) map[string][]*LogFile {
	result := map[string][]*LogFile{}
	filepath.WalkDir(v.option.RootFolder, func(path string, di fs.DirEntry, err error) error {
		if filepath.Ext(path) == v.option.Extension {
			catergory := filepath.Base(filepath.Dir(path))
			logList, ok := result[catergory]
			if !ok {
				//this catergory hasn't been scanned before.
				logList = []*LogFile{}
			}

			fullpath := filepath.ToSlash(path)
			if !showFullpath {
				fullpath = ""
			}

			st, err := os.Stat(path)
			if err != nil {
				return nil
			}

			logList = append(logList, &LogFile{
				Title:    strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
				Filename: filepath.Base(path),
				Fullpath: fullpath,
				Filesize: st.Size(),
			})

			result[catergory] = logList
		}

		return nil
	})
	return result
}

func (v *Viewer) LoadLogFile(filename string) (string, error) {
	filename = filepath.ToSlash(filename)
	filename = strings.ReplaceAll(filename, "../", "")
	logFilepath := filepath.Join(v.option.RootFolder, filename)
	if utils.FileExists(logFilepath) {
		//Load it
		content, err := os.ReadFile(logFilepath)
		if err != nil {
			return "", err
		}

		return string(content), nil
	} else {
		return "", errors.New("log file not found")
	}
}

func (v *Viewer) LoadLogSummary(filename string) (string, error) {
	filename = filepath.ToSlash(filename)
	filename = strings.ReplaceAll(filename, "../", "")
	logFilepath := filepath.Join(v.option.RootFolder, filename)
	if utils.FileExists(logFilepath) {
		//Load it
		content, err := os.ReadFile(logFilepath)
		if err != nil {
			return "", err
		}

		var summary LogSummary
		summary.LogSource = filepath.Base(filename)
		summary.TotalSize = int64(len(content))
		summary.RequestMethods = map[string]int64{}
		summary.HitPerDay = map[string]int64{} // Initialize to avoid nil map error
		summary.HiPerSite = map[string][]int64{}
		summary.UniqueIPs = map[string]int64{}
		summary.TopOrigins = map[string]int64{}
		summary.TopUserAgents = map[string]int64{}
		summary.TopPaths = map[string]int64{}

		lines := strings.Split(string(content), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue // Skip empty lines
			}

			if !strings.Contains(line, "[router:") {
				continue // Only process router: type logs
			}

			summary.TotalReqests++

			// Extract the date from the log line
			parts := strings.Split(line, "]")
			if len(parts) < 2 {
				continue // Skip malformed lines
			}

			datePart := strings.TrimSpace(parts[0][1:]) // Remove the leading '['
			date := datePart[:10]                       // Get the date part (YYYY-MM-DD)

			// Increment hit count for the day
			summary.HitPerDay[date]++

			// Extract origin, user agent, and path
			origin := ""
			userAgent := ""
			path := ""
			method := ""

			for _, part := range parts {
				part = strings.TrimSpace(part)
				if strings.HasPrefix(part, "[origin:") {
					origin = strings.TrimPrefix(part, "[origin:")
					origin = strings.TrimSuffix(origin, "]")
				} else if strings.HasPrefix(part, "[useragent:") {
					userAgent = strings.TrimPrefix(part, "[useragent:")
					userAgent = strings.TrimSuffix(userAgent, "]")
				} else if !strings.HasPrefix(part, "[") && !strings.HasSuffix(part, "]") && method == "" {
					// This is likely the HTTP method (GET, POST, etc.)
					fields := strings.Fields(part)
					if len(fields) > 0 {
						method = fields[0]
						summary.RequestMethods[method]++
						if len(fields) > 1 {
							path = fields[1] // The path is the second field
						}
					}
				}
			}

			if origin != "" {
				if _, exists := summary.HiPerSite[origin]; !exists {
					summary.HiPerSite[origin] = make([]int64, 32) // Initialize for 31 days
				}

				//Get the day of month from date
				dayIndex := 0
				if len(date) >= 10 {
					dayStr := date[8:10]               // Get the day part (DD)
					dayIndex, _ = strconv.Atoi(dayStr) // Convert to integer
				}

				if dayIndex < 1 || dayIndex > 31 {
					dayIndex = 0 // Default to 0 if out of range
				}

				summary.HiPerSite[origin][dayIndex-1]++ // Increment hit count for the specific day
				summary.HitPerDay[date]++               // Increment total hit count for the date
			}

			if userAgent != "" {
				summary.TopUserAgents[userAgent]++
			}

			if path != "" {
				if idx := strings.IndexAny(path, "?#"); idx != -1 {
					path = path[:idx]
				}
				summary.TopPaths[path]++
			}

			// Increment unique IPs (assuming IP is the first part of the line)
			ipPart := strings.Split(line, "[client:")[1]
			if ipPart != "" {
				ip := strings.TrimSpace(strings.Split(ipPart, "]")[0])
				if _, exists := summary.UniqueIPs[ip]; !exists {
					summary.UniqueIPs[ip] = 0
				}
				summary.UniqueIPs[ip]++ // Increment unique IP count for the day
			}

			// Check for errors: count if status code is not 1xx or 2xx
			statusParts := strings.Fields(line)
			if len(statusParts) > 0 {
				statusStr := statusParts[len(statusParts)-1]
				if len(statusStr) == 3 {
					if statusCode := statusStr[0]; statusCode != '1' && statusCode != '2' {
						summary.TotalErrors++
					} else {
						summary.TotalValid++
					}
				}
			}
		}

		js, err := json.Marshal(summary)
		if err != nil {
			return "", err
		}

		return string(js), nil

	} else {
		return "", errors.New("log file not found")
	}
}

/*
Log examples:

[2025-08-18 21:02:15.664246] [router:host-http] [origin:test1.localhost] [client: 127.0.0.1] [useragent: Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:141.0) Gecko/20100101 Firefox/141.0] GET /system/file_system/listDirHash?dir=s2%3A%2FMusic%2FMusic%20Bank%2FYear%202025%2F08-2025%2F 200
[2025-08-18 21:02:20.682091] [router:host-http] [origin:test1.localhost] [client: 127.0.0.1] [useragent: Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:141.0) Gecko/20100101 Firefox/141.0] GET /system/file_system/listDirHash?dir=s2%3A%2FMusic%2FMusic%20Bank%2FYear%202025%2F08-2025%2F 200
[2025-08-18 21:02:20.725569] [internal] [system:info] mDNS scan result updated
[2025-08-17 20:24:38.669488] [uptime-monitor] [system:info] Uptime updated - 1755433478
[2025-08-17 20:25:08.247535] [internal] [system:info] mDNS scan result updated
[2025-08-17 20:29:38.669187] [uptime-monitor] [system:info] Uptime updated - 1755433778
[2025-08-17 20:34:38.669090] [uptime-monitor] [system:info] Uptime updated - 1755434078
[2025-08-17 20:39:38.668610] [uptime-monitor] [system:info] Uptime updated - 1755434378
[2025-08-17 20:40:08.248890] [internal] [system:info] mDNS scan result updated
[2025-08-17 20:44:38.669058] [uptime-monitor] [system:info] Uptime updated - 1755434678
[2025-08-17 20:49:38.669340] [uptime-monitor] [system:info] Uptime updated - 1755434978
[2025-08-17 20:54:38.668785] [uptime-monitor] [system:info] Uptime updated - 1755435278
[2025-08-17 20:55:08.247715] [internal] [system:info] mDNS scan result updated
[2025-08-17 20:59:38.668575] [uptime-monitor] [system:info] Uptime updated - 1755435578
[2025-08-17 21:04:38.669637] [uptime-monitor] [system:info] Uptime updated - 1755435878
[2025-08-17 21:09:38.669109] [uptime-monitor] [system:info] Uptime updated - 1755436178
[2025-08-17 21:10:08.247618] [internal] [system:info] mDNS scan result updated
[2025-08-17 21:14:38.668828] [uptime-monitor] [system:info] Uptime updated - 1755436478
[2025-08-17 21:19:38.669091] [uptime-monitor] [system:info] Uptime updated - 1755436778
[2025-08-17 21:24:38.668830] [uptime-monitor] [system:info] Uptime updated - 1755437078
[2025-08-17 21:25:08.246931] [internal] [system:info] mDNS scan result updated
[2025-08-17 21:29:38.673217] [uptime-monitor] [system:info] Uptime updated - 1755437378
[2025-08-17 21:34:38.668883] [uptime-monitor] [system:info] Uptime updated - 1755437678
[2025-08-17 21:39:38.668980] [uptime-monitor] [system:info] Uptime updated - 1755437978
[2025-08-17 21:40:08.266062] [internal] [system:info] mDNS scan result updated
[2025-08-17 21:44:38.669150] [uptime-monitor] [system:info] Uptime updated - 1755438278
[2025-08-17 21:49:38.668640] [uptime-monitor] [system:info] Uptime updated - 1755438578
[2025-08-17 21:54:38.669275] [uptime-monitor] [system:info] Uptime updated - 1755438878
[2025-08-17 21:55:08.266425] [internal] [system:info] mDNS scan result updated
[2025-08-17 21:59:38.668861] [uptime-monitor] [system:info] Uptime updated - 1755439178
[2025-08-17 22:04:38.668840] [uptime-monitor] [system:info] Uptime updated - 1755439478
[2025-08-17 22:09:38.668798] [uptime-monitor] [system:info] Uptime updated - 1755439778
[2025-08-17 22:10:08.266417] [internal] [system:info] mDNS scan result updated
[2025-08-17 22:14:38.669122] [uptime-monitor] [system:info] Uptime updated - 1755440078
[2025-08-17 22:19:38.668810] [uptime-monitor] [system:info] Uptime updated - 1755440378
[2025-08-17 22:21:35.947519] [netstat] [system:info] Netstats listener stopped
[2025-08-17 22:21:35.947519] [internal] [system:info] Shutting down Zoraxy
[2025-08-17 22:21:35.947519] [internal] [system:info] Closing Netstats Listener
[2025-08-17 22:21:35.970526] [plugin-manager] [system:error] plugin com.example.restful-example encounted a fatal error. Disabling plugin...: exit status 0xc000013a
[2025-08-17 22:21:35.970526] [plugin-manager] [system:error] plugin org.aroz.zoraxy.api_call_example encounted a fatal error. Disabling plugin...: exit status 0xc000013a
[2025-08-17 22:21:36.250929] [internal] [system:info] Closing Statistic Collector
[2025-08-17 22:21:36.318808] [internal] [system:info] Stopping mDNS Discoverer (might take a few minutes)
[2025-08-17 22:21:36.319829] [internal] [system:info] Shutting down load balancer
[2025-08-17 22:21:36.319829] [internal] [system:info] Closing Certificates Auto Renewer
[2025-08-17 22:21:36.319829] [internal] [system:info] Closing Access Controller
[2025-08-17 22:21:36.319829] [internal] [system:info] Shutting down plugin manager
[2025-08-17 22:21:36.319829] [internal] [system:info] Cleaning up tmp files
[2025-08-17 22:21:36.328033] [internal] [system:info] Stopping system database
[2025-08-18 20:31:49.673182] [database] [system:info] Using BoltDB as the database backend
[2025-08-18 20:31:49.784069] [auth] [system:info] Authentication session key loaded from database
[2025-08-18 20:31:50.290804] [LoadBalancer] [system:info] Upstream state cache ticker started
[2025-08-18 20:31:50.510300] [static-webserv] [system:info] Static Web Server started. Listeing on :5487
[2025-08-18 20:31:51.017433] [internal] [system:info] Starting ACME handler
[2025-08-18 20:31:51.022545] [cert-renew] [system:info] ACME early renew set to 30 days and check interval set to 86400 seconds
[2025-08-18 20:31:51.073031] [plugin-manager] [system:info] Hot reload ticker started
[2025-08-18 20:31:51.357203] [plugin-manager] [system:info] Loaded plugin: API Call Example Plugin
[2025-08-18 20:31:51.357782] [plugin-manager] [system:info] Generated API key for plugin API Call Example Plugin
[2025-08-18 20:31:51.358293] [plugin-manager] [system:info] Starting plugin API Call Example Plugin at :5974
[2025-08-18 20:31:51.406867] [plugin-manager] [system:info] [API Call Example Plugin:13316] Starting API Call Example Plugin on 127.0.0.1:5974
[2025-08-18 20:31:51.466380] [plugin-manager] [system:info] Plugin list synced from plugin store
[2025-08-18 20:31:51.662866] [plugin-manager] [system:info] Loaded plugin: Restful Example
[2025-08-18 20:31:51.663383] [plugin-manager] [system:info] Starting plugin Restful Example at :5874
[2025-08-18 20:31:51.688641] [plugin-manager] [system:info] [Restful Example:10500] Restful-example started at http://127.0.0.1:5874
[2025-08-18 20:31:51.721309] [plugin-manager] [system:info] Plugin hash generated for: org.aroz.zoraxy.api_call_example
[2025-08-18 20:31:51.777523] [plugin-manager] [system:info] Plugin hash generated for: com.example.restful-example
[2025-08-18 20:31:51.789497] [internal] [system:info] Inbound port not set. Using default (443)
[2025-08-18 20:31:51.789497] [internal] [system:info] TLS mode enabled. Serving proxy request with TLS
[2025-08-18 20:31:51.789497] [internal] [system:info] Development mode enabled. Using no-store Cache Control policy
[2025-08-18 20:31:51.790016] [internal] [system:info] Force latest TLS mode disabled. Minimum TLS version is set to v1.0
[2025-08-18 20:31:51.790016] [internal] [system:info] Port 80 listener enabled
[2025-08-18 20:31:51.790016] [internal] [system:info] Force HTTPS mode enabled
[2025-08-18 20:31:51.825385] [proxy-config] [system:info] *.yami.localhost -> 192.168.0.16:8080 routing rule loaded
[2025-08-18 20:31:51.833567] [proxy-config] [system:info] a.localhost -> imuslab.com routing rule loaded
[2025-08-18 20:31:51.849358] [proxy-config] [system:info] aroz.localhost -> 192.168.0.16:8080 routing rule loaded
[2025-08-18 20:31:51.852977] [proxy-config] [system:info] auth.localhost -> localhost:5488 routing rule loaded
[2025-08-18 20:31:51.866792] [proxy-config] [system:info] debug.localhost -> dc.imuslab.com:8080 routing rule loaded
[2025-08-18 20:31:51.878091] [proxy-config] [system:info] peer.localhost -> 192.168.0.16:8080 routing rule loaded
[2025-08-18 20:31:51.887843] [proxy-config] [system:info] / -> 127.0.0.1:5487 routing rule loaded
[2025-08-18 20:31:51.895039] [proxy-config] [system:info] test.imuslab.com -> 192.168.1.202:8443 routing rule loaded
[2025-08-18 20:31:51.909917] [proxy-config] [system:info] test.imuslab.internal -> 127.0.0.1:80 routing rule loaded
[2025-08-18 20:31:51.922685] [proxy-config] [system:info] test.localhost -> alanyeung.co routing rule loaded
[2025-08-18 20:31:51.937314] [proxy-config] [system:info] webdav.localhost -> 127.0.0.1:80/redirect routing rule loaded
[2025-08-18 20:31:52.239414] [dprouter] [system:info] Starting HTTP-to-HTTPS redirector (port 80)
[2025-08-18 20:31:52.239414] [internal] [system:info] Dynamic Reverse Proxy service started
[2025-08-18 20:31:52.239414] [dprouter] [system:info] Reverse proxy service started in the background (TLS mode)
[2025-08-18 20:31:52.289262] [internal] [system:info] Zoraxy started. Visit control panel at http://localhost:8000
[2025-08-18 20:31:52.289262] [internal] [system:info] Assigned temporary port:36951
[2025-08-18 20:31:54.513995] [internal] [system:info] Uptime Monitor background service started
[2025-08-18 20:32:20.725596] [internal] [system:info] mDNS Startup scan completed
[2025-08-18 20:36:52.239883] [uptime-monitor] [system:info] Uptime updated - 1755520612
[2025-08-18 20:56:14.160166] [router:host-http] [origin:test1.localhost] [client: 127.0.0.1] [useragent: Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:141.0) Gecko/20100101 Firefox/141.0] GET /system/file_system/preference?key=file_explorer/theme 200
[2025-08-18 20:56:14.160166] [router:host-http] [origin:test1.localhost] [client: 127.0.0.1] [useragent: Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:141.0) Gecko/20100101 Firefox/141.0] GET /system/file_system/preference?key=file_explorer/listmode 200
[2025-08-18 20:56:14.160166] [router:host-http] [origin:test1.localhost] [client: 127.0.0.1] [useragent: Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:141.0) Gecko/20100101 Firefox/141.0] GET /system/file_system/preference?key=file_explorer/listmode 200
[2025-08-18 20:56:14.170270] [router:host-http] [origin:test1.localhost] [client: 127.0.0.1] [useragent: Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:141.0) Gecko/20100101 Firefox/141.0] GET /system/file_system/listRoots?user=true 200
[2025-08-18 20:56:14.171752] [router:host-http] [origin:test1.localhost] [client: 127.0.0.1] [useragent: Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:141.0) Gecko/20100101 Firefox/141.0] GET /system/id/requestInfo 200

*/
