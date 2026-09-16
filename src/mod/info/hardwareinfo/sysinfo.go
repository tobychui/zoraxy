//go:build linux
// +build linux

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

func Ifconfig(w http.ResponseWriter, r *http.Request) {
	cmdin := `ip link show`
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

//GetDriveStat is shared by all unix platforms, see drivestat_unix.go

func GetUSB(w http.ResponseWriter, r *http.Request) {
	cmdin := `lsusb`
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

func GetCPUInfo(w http.ResponseWriter, r *http.Request) {
	cmdin := `cat /proc/cpuinfo | grep -m1 "model name"`
	cmd := exec.Command("bash", "-c", cmdin)
	hardware, err := cmd.CombinedOutput()
	if err != nil {
		hardware = []byte("??? ")
	}

	cmdin = `lscpu | grep -m1 "Model name"`
	cmd = exec.Command("bash", "-c", cmdin)
	cpuModel, err := cmd.CombinedOutput()
	if err != nil {
		cpuModel = []byte("Generic Processor")
	}

	cmdin = `lscpu | grep "CPU max MHz"`
	cmd = exec.Command("bash", "-c", cmdin)
	speed, err := cmd.CombinedOutput()
	if err != nil {
		cmdin = `cat /proc/cpuinfo | grep -m1 "cpu MHz"`
		cmd = exec.Command("bash", "-c", cmdin)
		intelSpeed, err := cmd.CombinedOutput()
		if err != nil {
			speed = []byte("??? ")
		}
		speed = intelSpeed
	}

	cmdin = `cat /proc/cpuinfo | grep -m1 "Hardware"`
	cmd = exec.Command("bash", "-c", cmdin)
	cpuhardware, err := cmd.CombinedOutput()
	if err != nil {

	} else {
		hardware = cpuhardware
	}

	//On ARM
	cmdin = `cat /proc/cpuinfo | grep -m1 "Revision"`
	cmd = exec.Command("bash", "-c", cmdin)
	revision, err := cmd.CombinedOutput()
	if err != nil {
		//On x64
		cmdin = `cat /proc/cpuinfo | grep -m1 "family"`
		cmd = exec.Command("bash", "-c", cmdin)
		intelrev, err := cmd.CombinedOutput()
		if err != nil {
			revision = []byte("??? ")
		} else {
			revision = intelrev
		}
	}

	//Get Arch
	cmdin = `uname --m`
	cmd = exec.Command("bash", "-c", cmdin)
	arch, err := cmd.CombinedOutput()
	if err != nil {
		arch = []byte("??? ")
	}

	CPUInfo := CPUInfo{
		Freq:        filterGrepResults(string(speed), ":"),
		Hardware:    filterGrepResults(string(hardware), ":"),
		Instruction: filterGrepResults(string(arch), ":"),
		Model:       filterGrepResults(string(cpuModel), ":"),
		Revision:    filterGrepResults(string(revision), ":"),
	}

	var jsonData []byte
	jsonData, err = json.Marshal(CPUInfo)
	if err != nil {
		printAndLog(fmt.Sprint(err), nil)
	}
	utils.SendTextResponse(w, string(jsonData))
}

func GetRamInfo(w http.ResponseWriter, r *http.Request) {
	cmd := exec.Command("grep", "MemTotal", "/proc/meminfo")
	out, _ := cmd.CombinedOutput()
	strOut := string(out)
	strOut = strings.ReplaceAll(strOut, "MemTotal:", "")
	strOut = strings.ReplaceAll(strOut, "kB", "")
	strOut = strings.ReplaceAll(strOut, " ", "")
	strOut = strings.ReplaceAll(strOut, "\n", "")
	ramSize, _ := strconv.ParseInt(strOut, 10, 64)
	ramSizeInt := ramSize * 1000

	var jsonData []byte
	jsonData, err := json.Marshal(ramSizeInt)
	if err != nil {
		printAndLog(fmt.Sprint(err), nil)
	}
	utils.SendTextResponse(w, string(jsonData))
}
