//go:build windows
// +build windows

package hardwareinfo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"imuslab.com/zoraxy/mod/utils"
)

func GetCPUInfo(w http.ResponseWriter, r *http.Request) {

	CPUInfo := CPUInfo{
		Freq:        wmicGetinfo("cpu", "CurrentClockSpeed")[0],
		Hardware:    "unknown",
		Instruction: wmicGetinfo("cpu", "Caption")[0],
		Model:       wmicGetinfo("cpu", "Name")[0],
		Revision:    "unknown",
	}

	var jsonData []byte
	jsonData, err := json.Marshal(CPUInfo)
	if err != nil {
		printAndLog(fmt.Sprint(err), nil)
	}
	utils.SendTextResponse(w, string(jsonData))
}

func Ifconfig(w http.ResponseWriter, r *http.Request) {
	var arr []string
	for _, info := range wmicGetinfo("nic", "ProductName") {
		arr = append(arr, info)
	}

	var jsonData []byte
	jsonData, err := json.Marshal(arr)
	if err != nil {
		printAndLog(fmt.Sprint(err), nil)
	}
	utils.SendTextResponse(w, string(jsonData))
}

func GetDriveStat(w http.ResponseWriter, r *http.Request) {

	var DeviceID []string = wmicGetinfo("logicaldisk", "DeviceID")
	var FileSystem []string = wmicGetinfo("logicaldisk", "FileSystem")
	var FreeSpace []string = wmicGetinfo("logicaldisk", "FreeSpace")

	//These are three independent wmic queries and they do not always return the
	//same number of rows: a drive with no media (e.g. an empty optical drive or
	//card reader) reports a DeviceID but omits FileSystem and FreeSpace. Index
	//each slice defensively so a mismatch degrades to "unknown" instead of
	//panicking with an index out of range.
	valueAt := func(values []string, i int) string {
		if i < len(values) {
			return values[i]
		}
		return "unknown"
	}

	var arr []LogicalDisk
	for i, info := range DeviceID {
		LogicalDisk := LogicalDisk{
			DriveLetter: info,
			FileSystem:  valueAt(FileSystem, i),
			FreeSpace:   valueAt(FreeSpace, i),
		}
		arr = append(arr, LogicalDisk)
	}

	var jsonData []byte
	jsonData, err := json.Marshal(arr)
	if err != nil {
		printAndLog(fmt.Sprint(err), nil)
	}
	utils.SendTextResponse(w, string(jsonData))
}

func GetUSB(w http.ResponseWriter, r *http.Request) {
	var arr []string
	for _, info := range wmicGetinfo("Win32_USBHub", "Description") {
		arr = append(arr, info)
	}

	var jsonData []byte
	jsonData, err := json.Marshal(arr)
	if err != nil {
		printAndLog(fmt.Sprint(err), nil)
	}
	utils.SendTextResponse(w, string(jsonData))
}

func GetRamInfo(w http.ResponseWriter, r *http.Request) {
	var RAMsize int = 0
	for _, info := range wmicGetinfo("memorychip", "Capacity") {
		DIMMCapacity, _ := strconv.Atoi(info)
		RAMsize += DIMMCapacity
	}

	var jsonData []byte
	jsonData, err := json.Marshal(RAMsize)
	if err != nil {
		printAndLog(fmt.Sprint(err), nil)
	}
	utils.SendTextResponse(w, string(jsonData))
}
