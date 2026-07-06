package usageinfo

import (
	"fmt"
	"io/ioutil"
	"strconv"
	"strings"
	"time"
)

type CPUStats struct {
	user    uint64
	nice    uint64
	system  uint64
	idle    uint64
	iowait  uint64
	irq     uint64
	softirq uint64
}

// getCPUStats reads and parses the CPU stats from /proc/stat
func getCPUStats() (CPUStats, error) {
	data, err := ioutil.ReadFile("/proc/stat")
	if err != nil {
		return CPUStats{}, err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "cpu ") {
			fields := strings.Fields(line)
			if len(fields) < 8 {
				return CPUStats{}, fmt.Errorf("unexpected format in /proc/stat")
			}

			// Parse the CPU fields into the CPUStats struct
			user, _ := strconv.ParseUint(fields[1], 10, 64)
			nice, _ := strconv.ParseUint(fields[2], 10, 64)
			system, _ := strconv.ParseUint(fields[3], 10, 64)
			idle, _ := strconv.ParseUint(fields[4], 10, 64)
			iowait, _ := strconv.ParseUint(fields[5], 10, 64)
			irq, _ := strconv.ParseUint(fields[6], 10, 64)
			softirq, _ := strconv.ParseUint(fields[7], 10, 64)

			return CPUStats{
				user:    user,
				nice:    nice,
				system:  system,
				idle:    idle,
				iowait:  iowait,
				irq:     irq,
				softirq: softirq,
			}, nil
		}
	}

	return CPUStats{}, fmt.Errorf("could not find CPU stats")
}

// calculateCPUUsage calculates the percentage of CPU usage
func calculateCPUUsage(prev, current CPUStats) float64 {
	prevTotal := prev.user + prev.nice + prev.system + prev.idle + prev.iowait + prev.irq + prev.softirq
	currentTotal := current.user + current.nice + current.system + current.idle + current.iowait + current.irq + current.softirq

	totalDiff := currentTotal - prevTotal
	idleDiff := current.idle - prev.idle

	if totalDiff == 0 {
		return 0.0
	}

	usage := (float64(totalDiff-idleDiff) / float64(totalDiff)) * 100.0
	return usage
}

// GetCPUUsage returns the current CPU usage as a percentage
// Note this is blocking and will sleep for 1 second
func GetCPUUsageUsingProcStat() (float64, error) {
	// Get initial CPU stats
	prevStats, err := getCPUStats()
	if err != nil {
		return 0, err
	}

	// Sleep for 1 second to compare stats over time
	time.Sleep(1 * time.Second)

	// Get current CPU stats after 1 second
	currentStats, err := getCPUStats()
	if err != nil {
		return 0, err
	}

	// Calculate and print the CPU usage
	cpuUsage := calculateCPUUsage(prevStats, currentStats)
	return cpuUsage, nil
}
