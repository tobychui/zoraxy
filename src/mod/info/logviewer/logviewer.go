package logviewer

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
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

	content, err := v.LoadLogFile(strings.TrimSpace(filepath.Base(filename)))
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendTextResponse(w, content)
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
