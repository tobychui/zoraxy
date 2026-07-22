//go:build freebsd
// +build freebsd

package hardwareinfo

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"

	"imuslab.com/zoraxy/mod/utils"
)

/*
	System Info
	author: HyperXraft
	date:	2021-02-18

	This module get the CPU information on different platform using
	native terminal commands on FreeBSD platform

	DEFINITIONS
	===========
	CPUModel: Refers to the Marketing name of the CPU, e.g. Intel Xeon E7 8890
	CPUHardware: Refers to the CPUID name, e.g. GenuineIntel-6-3A-9
	CPUArch: Refers to the ISA of the CPU, e.g. aarch64
	CPUFreq: Refers to the CPU frequency in terms of gigahertz, e.g. 0.8GHz
*/

const unknown_string = "??? "
const query_frequency_command = "sysctl hw.model | awk '{print $NF}'"
const query_cpumodel_command = "sysctl hw.model | awk '{for(i=1;++i<=NF-3;) printf $i\" \"; print $(NF-2)}'"
const query_cpuarch_command = "sysctl hw.machine_arch | awk '{print $NF}'"
const query_cpuhardware_command = "sysctl kern.hwpmc.cpuid | awk '{print $NF}'"
const query_netinfo_command = "ifconfig -a"
const query_usbinfo_command = "usbconfig"
const query_memsize_command = "sysctl hw.physmem | awk '{print $NF}'"

// GetCPUFreq() -> String
// Returns the CPU frequency in the terms of MHz
func GetCPUFreq() string {
	shell := exec.Command("bash", "-c", query_frequency_command) // Run command
	freqByteArr, err := shell.CombinedOutput()                   // Response from cmdline
	if err != nil {                                              // If done w/ errors then
		printAndLog(fmt.Sprint(err), nil)
		return unknown_string
	}

	freqStr := strings.ReplaceAll(string(freqByteArr), "GHz", "")
	freqStr = strings.ReplaceAll(freqStr, "\n", "")
	freqStr = strings.ReplaceAll(freqStr, " ", "")
	freqFloat, _ := strconv.ParseFloat(freqStr, 8)
	freqFloat = freqFloat * 1000
	freqStrMHz := strconv.FormatFloat(freqFloat, 'f', -1, 64)

	return freqStrMHz
}

// GetCPUModel -> String
// Returns the CPU model name string
func GetCPUModel() string {
	shell := exec.Command("bash", "-c", query_cpumodel_command) // Run command
	modelStr, err := shell.CombinedOutput()                     // Response from cmdline
	if err != nil {                                             // If done w/ errors then
		printAndLog(fmt.Sprint(err), nil)
		return unknown_string
	}

	return string(modelStr)
}

// GetCPUHardware -> String
// Returns the CPU ID string
func GetCPUHardware() string {
	shell := exec.Command("bash", "-c", query_cpuhardware_command) // Run command
	hwStr, err := shell.CombinedOutput()                           // Response from cmdline
	if err != nil {                                                // If done w/ errors then
		printAndLog(fmt.Sprint(err), nil)
		return unknown_string
	}

	return string(hwStr)
}

// GetCPUArch -> String
// Returns the CPU architecture string
func GetCPUArch() string {
	shell := exec.Command("bash", "-c", query_cpuarch_command) // Run command
	archStr, err := shell.CombinedOutput()                     // Response from cmdline
	if err != nil {                                            // If done w/ errors then
		printAndLog(fmt.Sprint(err), nil)
		return unknown_string
	}

	return string(archStr)
}

// Inherited code from sysinfo_window.go
func GetCPUInfo(w http.ResponseWriter, r *http.Request) {
	CPUInfo := CPUInfo{
		Freq:        GetCPUFreq(),
		Hardware:    GetCPUHardware(),
		Instruction: GetCPUArch(),
		Model:       GetCPUModel(),
		Revision:    "unknown",
	}

	var jsonData []byte
	jsonData, err := json.Marshal(CPUInfo)
	if err != nil {
		printAndLog(fmt.Sprint(err), nil)
	}
	utils.SendTextResponse(w, string(jsonData))
}

// Inherited code from sysinfo.go
func Ifconfig(w http.ResponseWriter, r *http.Request) {
	cmdin := query_netinfo_command
	cmd := exec.Command("bash", "-c", cmdin)
	networkInterfaces, err := cmd.CombinedOutput()
	if err != nil {
		networkInterfaces = []byte{}
	}

	nic := strings.Split(string(networkInterfaces), "\n")

	var arr []string
	for _, info := range nic {
		thisInfo := string(info)
		arr = append(arr, thisInfo)
	}

	var jsonData []byte
	jsonData, err = json.Marshal(arr)
	if err != nil {
		printAndLog(fmt.Sprint(err), nil)
	}
	utils.SendTextResponse(w, string(jsonData))
}

// Inherited code from sysinfo.go
func GetDriveStat(w http.ResponseWriter, r *http.Request) {
	//Get drive status using df command
	cmdin := `df -k | sed -e /Filesystem/d`
	cmd := exec.Command("bash", "-c", cmdin)
	dev, err := cmd.CombinedOutput()
	if err != nil {
		dev = []byte{}
	}

	drives := strings.Split(string(dev), "\n")

	if len(drives) == 0 {
		utils.SendErrorResponse(w, "Invalid disk information")
		return
	}

	var arr []LogicalDisk
	for _, driveInfo := range drives {
		if driveInfo == "" {
			continue
		}
		for strings.Contains(driveInfo, "  ") {
			driveInfo = strings.Replace(driveInfo, "  ", " ", -1)
		}
		driveInfoChunk := strings.Split(driveInfo, " ")
		tmp, _ := strconv.Atoi(driveInfoChunk[3])
		freespaceInByte := int64(tmp)

		LogicalDisk := LogicalDisk{
			DriveLetter: driveInfoChunk[5],
			FileSystem:  driveInfoChunk[0],
			FreeSpace:   strconv.FormatInt(freespaceInByte*1024, 10), //df show disk space in 1KB blocks
		}
		arr = append(arr, LogicalDisk)
	}

	var jsonData []byte
	jsonData, err = json.Marshal(arr)
	if err != nil {
		printAndLog(fmt.Sprint(err), nil)
	}
	utils.SendTextResponse(w, string(jsonData))

}

// GetUSB(ResponseWriter, HttpRequest) -> nil
// Takes in http.ResponseWriter w and *http.Request r,
// Send TextResponse containing USB information extracted from shell in JSON
func GetUSB(w http.ResponseWriter, r *http.Request) {
	cmdin := query_usbinfo_command
	cmd := exec.Command("bash", "-c", cmdin)
	usbd, err := cmd.CombinedOutput()
	if err != nil {
		usbd = []byte{}
	}

	usbDrives := strings.Split(string(usbd), "\n")

	var arr []string
	for _, info := range usbDrives {
		arr = append(arr, info)
	}

	var jsonData []byte
	jsonData, err = json.Marshal(arr)
	if err != nil {
		printAndLog(fmt.Sprint(err), nil)
	}
	utils.SendTextResponse(w, string(jsonData))
}

// GetRamInfo(w ResponseWriter, r *HttpRequest) -> nil
// Takes in http.ResponseWriter w and *http.Request r,
// Send TextResponse containing physical memory size
// extracted from shell in JSON
func GetRamInfo(w http.ResponseWriter, r *http.Request) {
	cmd := exec.Command("bash", "-c", query_memsize_command)
	out, _ := cmd.CombinedOutput()

	strOut := string(out)
	strOut = strings.ReplaceAll(strOut, "\n", "")
	ramSize, _ := strconv.ParseInt(strOut, 10, 64)
	ramSizeInt := ramSize

	var jsonData []byte
	jsonData, err := json.Marshal(ramSizeInt)
	if err != nil {
		printAndLog(fmt.Sprint(err), nil)
	}
	utils.SendTextResponse(w, string(jsonData))
}
