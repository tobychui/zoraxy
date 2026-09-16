//go:build linux || darwin || freebsd
// +build linux darwin freebsd

package hardwareinfo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"

	"imuslab.com/zoraxy/mod/utils"
)

// GetDriveStat returns the free space of each mounted filesystem.
// Shared by linux, darwin and freebsd; see drivestat.go for the parser.
func GetDriveStat(w http.ResponseWriter, r *http.Request) {
	//Get drive status using df command.
	cmd := exec.Command("bash", "-c", `df -kP`)
	dev, err := cmd.Output()
	if err != nil {
		printAndLog("unable to query drive statistics", err)
		dev = []byte{}
	}

	arr := parseDfOutput(string(dev))

	jsonData, err := json.Marshal(arr)
	if err != nil {
		printAndLog(fmt.Sprint(err), nil)
		utils.SendErrorResponse(w, "Invalid disk information")
		return
	}

	utils.SendTextResponse(w, string(jsonData))
}
