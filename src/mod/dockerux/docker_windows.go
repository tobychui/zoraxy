//go:build windows
// +build windows

package dockerux

/*

	Windows docker UX optimizer dummy

	This is a dummy module for Windows as docker features
	is useless on Windows and create a larger binary size

	docker on Windows build are trimmed to reduce binary size
	and make it compatibile with Windows 7
*/

import (
	"encoding/json"
	"net/http"

	"imuslab.com/zoraxy/mod/utils"
)

// Windows build not support docker
func (d *UXOptimizer) HandleDockerAvailable(w http.ResponseWriter, r *http.Request) {
	js, _ := json.Marshal(d.RunninInDocker)
	utils.SendJSONResponse(w, string(js))
}

func (d *UXOptimizer) HandleDockerContainersList(w http.ResponseWriter, r *http.Request) {
	utils.SendErrorResponse(w, "Platform not supported")
}
