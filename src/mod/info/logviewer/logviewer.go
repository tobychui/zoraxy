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
	"sort"
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

// LogEntry represents a single parsed log line from an access log file
type LogEntry struct {
	Timestamp  string `json:"timestamp"`
	RouterType string `json:"router_type"`
	Origin     string `json:"origin"`
	ClientIP   string `json:"client_ip"`
	UserAgent  string `json:"user_agent"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	StatusCode int    `json:"status_code"`
}

// FilterParams holds all filtering, sorting and pagination parameters for log queries
type FilterParams struct {
	FilterIP     string
	FilterPath   string
	FilterStatus string
	FilterMethod string
	FilterOrigin string
	TimeStart    string
	TimeEnd      string
	SortField    string
	SortOrder    string
	Page         int
	PageSize     int
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

	// Sanitize log content to prevent XSS attacks
	content = utils.SanitizeLogContent(content)

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
					// Sanitize each field to prevent XSS attacks
					for i := range fieldsWithTimestamp {
						fieldsWithTimestamp[i] = utils.SanitizeLogContent(fieldsWithTimestamp[i])
					}
					errorLines = append(errorLines, fieldsWithTimestamp)
				}
			}
		}
	}

	js, _ := json.Marshal(errorLines)
	utils.SendJSONResponse(w, string(js))
}

// HandleReadLogEntries returns parsed log entries as structured JSON with filtering, sorting and pagination.
// Query parameters: file (required), page, pageSize, sortField, sortOrder,
// filter_ip, filter_path, filter_status, filter_method, filter_origin, time_start, time_end
func (v *Viewer) HandleReadLogEntries(w http.ResponseWriter, r *http.Request) {
	filename, err := utils.GetPara(r, "file")
	if err != nil {
		utils.SendErrorResponse(w, "invalid filename given")
		return
	}

	// Parse pagination params with defaults
	page := 1
	if p, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && p > 0 {
		page = p
	}
	pageSize := 50
	if ps, err := strconv.Atoi(r.URL.Query().Get("pageSize")); err == nil && ps > 0 && ps <= 500 {
		pageSize = ps
	}

	// Parse sort params with defaults
	sortField := r.URL.Query().Get("sortField")
	if sortField == "" {
		sortField = "timestamp"
	}
	sortOrder := r.URL.Query().Get("sortOrder")
	if sortOrder != "asc" {
		sortOrder = "desc"
	}

	// Build FilterParams
	params := FilterParams{
		FilterIP:     r.URL.Query().Get("filter_ip"),
		FilterPath:   r.URL.Query().Get("filter_path"),
		FilterStatus: r.URL.Query().Get("filter_status"),
		FilterMethod: r.URL.Query().Get("filter_method"),
		FilterOrigin: r.URL.Query().Get("filter_origin"),
		TimeStart:    r.URL.Query().Get("time_start"),
		TimeEnd:      r.URL.Query().Get("time_end"),
		SortField:    sortField,
		SortOrder:    sortOrder,
		Page:         page,
		PageSize:     pageSize,
	}

	// Load log file content
 	safeFilename := strings.TrimSpace(filepath.Base(filename))
 	if safeFilename == "" || safeFilename == "." || filepath.IsAbs(safeFilename) || strings.ContainsAny(safeFilename, `/\\`) {
 		utils.SendErrorResponse(w, "invalid filename given")
 		return
 	}
 	content, err := v.LoadLogFile(safeFilename)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	// Parse all router log lines into structured entries
	lines := strings.Split(content, "\n")
	entries := make([]*LogEntry, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, "[router:") {
			continue
		}
		entry, err := v.parseLogLine(line)
		if err != nil {
			continue
		}
		entries = append(entries, entry)
	}

	// Apply filtering, sorting and pagination
	pageEntries, total := filterAndSortEntries(entries, params)

	// Calculate total pages
	totalPages := 0
	if total > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}

	resp := map[string]interface{}{
		"entries":    pageEntries,
		"total":      total,
		"page":       page,
		"pageSize":   pageSize,
		"totalPages": totalPages,
	}

	js, _ := json.Marshal(resp)
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
	filename = filepath.Clean(filename)
	if strings.Contains(filename, "..") {
		return ""
	}
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

func (v *Viewer) isMethodKeyword(part string) bool {
	part = strings.TrimSpace(part)
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}
	for _, method := range methods {
		if strings.HasPrefix(part, method+" ") {
			return true
		}
	}
	return false
}

// parseLogLine parses a single access log line into a LogEntry struct.
// Expected format: [timestamp] [router:type] [origin:host] [client: IP] [useragent: UA] METHOD PATH STATUS
func (v *Viewer) parseLogLine(line string) (*LogEntry, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, errors.New("empty line")
	}

	parts := strings.Split(line, "]")

	entry := &LogEntry{}

	// Extract timestamp from the first part (strip leading '[')
	timestamp := strings.TrimSpace(parts[0])
	timestamp = strings.TrimPrefix(timestamp, "[")
	entry.Timestamp = timestamp

	// Iterate over all parts to extract bracketed metadata and the trailing request fields
	for _, part := range parts {
		part = strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(part, "[router:"):
			entry.RouterType = strings.TrimSpace(strings.TrimPrefix(part, "[router:"))
		case strings.HasPrefix(part, "[origin:"):
			entry.Origin = strings.TrimSpace(strings.TrimPrefix(part, "[origin:"))
		case strings.HasPrefix(part, "[client:"):
			entry.ClientIP = strings.TrimSpace(strings.TrimPrefix(part, "[client:"))
		case strings.HasPrefix(part, "[useragent:"):
			entry.UserAgent = strings.TrimSpace(strings.TrimPrefix(part, "[useragent:"))
		case v.isMethodKeyword(part):
			// This is the trailing part containing method, path and status code
			fields := strings.Fields(part)
			if len(fields) >= 1 {
				entry.Method = fields[0]
			}
			if len(fields) >= 2 {
				entry.Path = fields[1]
			}
			if len(fields) >= 3 {
				if sc, err := strconv.Atoi(fields[2]); err == nil {
					entry.StatusCode = sc
				}
			}
		}
	}

	return entry, nil
}

// filterAndSortEntries applies filtering, sorting and pagination to a slice of LogEntry.
// It returns the paginated slice and the total count before pagination.
func filterAndSortEntries(entries []*LogEntry, params FilterParams) ([]*LogEntry, int) {
	// Step 1: Filter
	filtered := make([]*LogEntry, 0, len(entries))
	for _, entry := range entries {
		if params.FilterIP != "" && !strings.Contains(entry.ClientIP, params.FilterIP) {
			continue
		}
		if params.FilterPath != "" && !strings.Contains(entry.Path, params.FilterPath) {
			continue
		}
		if params.FilterStatus != "" {
			statusStr := strconv.Itoa(entry.StatusCode)
			if statusStr != params.FilterStatus {
				continue
			}
		}
		if params.FilterMethod != "" && !strings.EqualFold(entry.Method, params.FilterMethod) {
			continue
		}
		if params.FilterOrigin != "" && !strings.Contains(entry.Origin, params.FilterOrigin) {
			continue
		}
 		ts := entry.Timestamp
 		if len(ts) == 19 {
 			ts += ".000000"
 		}
 		timeStart := params.TimeStart
 		if len(timeStart) == 19 {
 			timeStart += ".000000"
 		}
 		timeEnd := params.TimeEnd
 		if len(timeEnd) == 19 {
 			timeEnd += ".999999"
 		}
 		if timeStart != "" && ts < timeStart {
			continue
		}
 		if timeEnd != "" && ts > timeEnd {
			continue
		}
		filtered = append(filtered, entry)
	}

	// Step 2: Sort
	sort.Slice(filtered, func(i, j int) bool {
		var less, equal bool
		switch params.SortField {
		case "origin":
			less = filtered[i].Origin < filtered[j].Origin
			equal = filtered[i].Origin == filtered[j].Origin
		case "client_ip":
			less = filtered[i].ClientIP < filtered[j].ClientIP
			equal = filtered[i].ClientIP == filtered[j].ClientIP
		case "method":
			less = filtered[i].Method < filtered[j].Method
			equal = filtered[i].Method == filtered[j].Method
		case "path":
			less = filtered[i].Path < filtered[j].Path
			equal = filtered[i].Path == filtered[j].Path
		case "status_code":
			less = filtered[i].StatusCode < filtered[j].StatusCode
			equal = filtered[i].StatusCode == filtered[j].StatusCode
		case "user_agent":
			less = filtered[i].UserAgent < filtered[j].UserAgent
			equal = filtered[i].UserAgent == filtered[j].UserAgent
		default:
			less = filtered[i].Timestamp < filtered[j].Timestamp
			equal = filtered[i].Timestamp == filtered[j].Timestamp
		}

		if equal {
			return false
		}
		if params.SortOrder == "desc" {
			return !less
		}
		return less
	})

	total := len(filtered)

	// Step 3: Paginate
	if params.Page < 1 {
		params.Page = 1
	}
	start := (params.Page - 1) * params.PageSize
	if start >= len(filtered) {
		return []*LogEntry{}, total
	}
	end := start + params.PageSize
	if end > len(filtered) {
		end = len(filtered)
	}

	return filtered[start:end], total
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

			//Check for new log format or older one
			datePart := strings.TrimSpace(parts[0][1:]) // old format, start with [
			if parts[0] != "" && !strings.HasPrefix(parts[0], "[") {
				datePart = strings.TrimSpace(parts[0])     // new format, no starting [
				datePart = strings.Split(datePart, " ")[0] // Get only the date part
			}
			date := datePart[:10] // Get the date part (YYYY-MM-DD)

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
					userAgent = strings.TrimSpace(userAgent)
				} else if v.isMethodKeyword(part) {
					// This is likely the HTTP method (GET, POST, etc.)
					fields := strings.Fields(part)
					method = fields[0]
				}
			}

			// Track origin hits for TopOrigins
			if origin != "" {
				// Sanitize origin to prevent XSS
				origin = utils.SanitizeLogContent(origin)
				summary.TopOrigins[origin]++
			}

			// Extract path, usually at the last part after ]
			fields := strings.Fields(line)
			if len(fields) > 1 {
				lastPart := fields[len(fields)-2]
				if strings.HasPrefix(lastPart, "/") {
					path = lastPart
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
				// Sanitize user agent to prevent XSS
				userAgent = utils.SanitizeLogContent(userAgent)
				summary.TopUserAgents[userAgent]++
			}

			if path != "" {
				if idx := strings.IndexAny(path, "?#"); idx != -1 {
					path = path[:idx]
				}
				// Sanitize path to prevent XSS
				path = utils.SanitizeLogContent(path)
				summary.TopPaths[path]++
			}

			if method != "" {
				summary.RequestMethods[method]++
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

		// Sort and limit TopOrigins to top 20 by hit count
		type originHit struct {
			origin string
			hits   int64
		}
		originList := make([]originHit, 0, len(summary.TopOrigins))
		for origin, hits := range summary.TopOrigins {
			originList = append(originList, originHit{origin, hits})
		}
		sort.Slice(originList, func(i, j int) bool {
			return originList[i].hits > originList[j].hits
		})

		// Keep only top 20 origins
		summary.TopOrigins = make(map[string]int64)
		limit := 20
		if len(originList) < limit {
			limit = len(originList)
		}
		for i := 0; i < limit; i++ {
			summary.TopOrigins[originList[i].origin] = originList[i].hits
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
