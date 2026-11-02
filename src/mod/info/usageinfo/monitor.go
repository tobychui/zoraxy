package usageinfo

import (
	"sync"
	"time"
)

// monitorCache holds the most recently sampled CPU and RAM data.
type monitorCache struct {
	mu       sync.RWMutex
	cpuUsage float64
	usedRAM  string
	totalRAM string
	ramUsage float64
	ready    bool
}

var globalCache = &monitorCache{}

// StartBackgroundMonitor launches a goroutine that samples CPU and RAM once
// per second and stores results in a shared cache. Call this once at startup.
// The HTTP handler can then return cached data without blocking.
func StartBackgroundMonitor() {
	go func() {
		prevStats, err := getCPUStats()
		if err != nil {
			// /proc/stat not available (Windows or unsupported platform):
			// fall back to the blocking platform-specific method.
			for {
				cpu := GetCPUUsage()
				usedRAM, totalRAM, ramPct := GetRAMUsage()
				globalCache.mu.Lock()
				globalCache.cpuUsage = cpu
				globalCache.usedRAM = usedRAM
				globalCache.totalRAM = totalRAM
				globalCache.ramUsage = ramPct
				globalCache.ready = true
				globalCache.mu.Unlock()
				time.Sleep(time.Second)
			}
		}

		// Linux / macOS path: compare two /proc/stat snapshots 1 s apart.
		for {
			time.Sleep(time.Second)

			currentStats, err := getCPUStats()
			if err != nil {
				continue
			}

			cpu := calculateCPUUsage(prevStats, currentStats)
			prevStats = currentStats

			usedRAM, totalRAM, ramPct := GetRAMUsage()

			globalCache.mu.Lock()
			globalCache.cpuUsage = cpu
			globalCache.usedRAM = usedRAM
			globalCache.totalRAM = totalRAM
			globalCache.ramUsage = ramPct
			globalCache.ready = true
			globalCache.mu.Unlock()
		}
	}()
}

// GetCachedStats returns the most recent sampled stats without blocking.
// ready is false for the first ~1 s after startup; callers may return zero
// values until then.
func GetCachedStats() (cpuUsage float64, usedRAM, totalRAM string, ramUsage float64, ready bool) {
	globalCache.mu.RLock()
	defer globalCache.mu.RUnlock()
	return globalCache.cpuUsage, globalCache.usedRAM, globalCache.totalRAM, globalCache.ramUsage, globalCache.ready
}
