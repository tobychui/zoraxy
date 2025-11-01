package logviewer

import (
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io"
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

	linesParam, err := utils.GetPara(r, "lines")
	if err != nil {
		linesParam = "all"
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

	// Apply lines limit after filtering
	if linesParam != "all" {
		if lineLimit, err := strconv.Atoi(linesParam); err == nil && lineLimit > 0 {
			allLines := strings.Split(content, "\n")
			if len(allLines) > lineLimit {
				// Keep only the last lineLimit lines
				allLines = allLines[len(allLines)-lineLimit:]
				content = strings.Join(allLines, "\n")
			}
		}
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

func (v *Viewer) HandleLogErrorSummary(w http.ResponseWriter, r *http.Request) {
	filename, err := utils.GetPara(r, "file")
	if err != nil {
		utils.SendErrorResponse(w, "invalid filename given")
		return
	}

	content, err := v.LoadLogFile(strings.TrimSpace(filepath.Base(filename)))
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	//Generate the error summary for log that is request and non 100 - 200 range status code
	errorLines := [][]string{}
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Only process router logs with a status code not in 1xx or 2xx
		if strings.Contains(line, "[router:") {
			//Extract date time from the line
			timestamp := ""
			if strings.HasPrefix(line, "[") && strings.Contains(line, "]") {
				timestamp = line[1:strings.Index(line, "]")]
			}

			//Trim out the request metadata
			line = line[strings.LastIndex(line, "]")+1:]
			fields := strings.Fields(strings.TrimSpace(line))

			if len(fields) >= 3 {
				statusStr := fields[2]
				if len(statusStr) == 3 && (statusStr[0] != '1' && statusStr[0] != '2' && statusStr[0] != '3') {
					fieldsWithTimestamp := append([]string{timestamp}, strings.Fields(strings.TrimSpace(line))...)
					errorLines = append(errorLines, fieldsWithTimestamp)
				}
			}
		}
	}

	js, _ := json.Marshal(errorLines)
	utils.SendJSONResponse(w, string(js))
}

/*
	Log Access Functions
*/

func (v *Viewer) ListLogFiles(showFullpath bool) map[string][]*LogFile {
	result := map[string][]*LogFile{}
	filepath.WalkDir(v.option.RootFolder, func(path string, di fs.DirEntry, err error) error {
		if filepath.Ext(path) == ".log" || strings.HasSuffix(path, ".log.gz") {
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

			filename := filepath.Base(path)
			filename = strings.TrimSuffix(filename, ".log") //to handle cases where the filename ends of .log.gz

			logList = append(logList, &LogFile{
				Title:    strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
				Filename: filename,
				Fullpath: fullpath,
				Filesize: st.Size(),
			})

			result[catergory] = logList
		}

		return nil
	})
	return result
}

// readLogFileContent reads a log file, handling both compressed (.gz) and uncompressed files
func (v *Viewer) readLogFileContent(filepath string) ([]byte, error) {
	file, err := os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Check if file is compressed
	if strings.HasSuffix(filepath, ".gz") {
		gzipReader, err := gzip.NewReader(file)
		if err != nil {
			// Try zip reader for older logs that use zip compression despite .gz extension
			zipReader, err := zip.OpenReader(filepath)
			if err != nil {
				return nil, err
			}
			defer zipReader.Close()
			if len(zipReader.File) == 0 {
				return nil, errors.New("zip file is empty")
			}
			zipFile := zipReader.File[0]
			rc, err := zipFile.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()
			return io.ReadAll(rc)

		}
		defer gzipReader.Close()

		return io.ReadAll(gzipReader)
	}

	// Regular file
	return io.ReadAll(file)
}

func (v *Viewer) senatizeLogFilenameInput(filename string) string {
	filename = strings.TrimSuffix(filename, ".log.gz")
	filename = strings.TrimSuffix(filename, ".log")
	filename = filepath.ToSlash(filename)
	filename = strings.ReplaceAll(filename, "../", "")
	//Check if .log.gz or .log exists
	if utils.FileExists(filepath.Join(v.option.RootFolder, filename+".log")) {
		return filepath.Join(v.option.RootFolder, filename+".log")
	}
	if utils.FileExists(filepath.Join(v.option.RootFolder, filename+".log.gz")) {
		return filepath.Join(v.option.RootFolder, filename+".log.gz")
	}
	return filepath.Join(v.option.RootFolder, filename)
}

func (v *Viewer) LoadLogFile(filename string) (string, error) {
	// filename might be in (no extension), .log or .log.gz format
	// so we trim those first before proceeding
	logFilepath := v.senatizeLogFilenameInput(filename)
	if utils.FileExists(logFilepath) {
		//Load it
		content, err := v.readLogFileContent(logFilepath)
		if err != nil {
			return "", err
		}

		return string(content), nil
	}

	//Also check .log.gz
	logFilepathGz := logFilepath + ".gz"
	if utils.FileExists(logFilepathGz) {
		content, err := v.readLogFileContent(logFilepathGz)
		if err != nil {
			return "", err
		}

		return string(content), nil
	} else {
		return "", errors.New("log file not found")
	}
}

func (v *Viewer) LoadLogSummary(filename string) (string, error) {
	logFilepath := v.senatizeLogFilenameInput(filename)
	if utils.FileExists(logFilepath) {
		//Load it
		content, err := v.readLogFileContent(logFilepath)
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
