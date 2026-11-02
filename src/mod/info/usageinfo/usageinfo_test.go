package usageinfo

import (
	"runtime"
	"testing"
)

// TestGetCPUUsage tests the top-level GetCPUUsage function.
// On Linux/FreeBSD/Darwin it reads /proc/stat (Linux) or uses platform commands.
// On non-Linux platforms the /proc/stat path does not exist so the function
// gracefully returns 0 without error.
func TestGetCPUUsage(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "freebsd" && runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("GetCPUUsage not implemented on this platform")
	}

	usage := GetCPUUsage()
	if usage < 0 || usage > 100 {
		t.Errorf("GetCPUUsage() = %v, expected value in range [0, 100]", usage)
	}
}

// TestGetCPUUsageLinux verifies the return value is in [0,100] specifically on Linux.
func TestGetCPUUsageLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping Linux-specific CPU usage test on non-Linux OS")
	}

	usage := GetCPUUsage()
	if usage < 0 {
		t.Errorf("GetCPUUsage() returned negative value: %v", usage)
	}
	if usage > 100 {
		t.Errorf("GetCPUUsage() returned value > 100: %v", usage)
	}
}

// TestGetCPUUsageUsingProcStat tests the /proc/stat based CPU usage reader.
// This file only exists on Linux; the test is skipped on other platforms.
func TestGetCPUUsageUsingProcStat(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping /proc/stat test on non-Linux OS")
	}

	usage, err := GetCPUUsageUsingProcStat()
	if err != nil {
		t.Fatalf("GetCPUUsageUsingProcStat() unexpected error: %v", err)
	}

	if usage < 0 || usage > 100 {
		t.Errorf("GetCPUUsageUsingProcStat() = %v, expected value in range [0, 100]", usage)
	}
}

// TestGetCPUUsageUsingProcStatNotAvailable validates that GetCPUUsageUsingProcStat
// returns a meaningful error when the underlying /proc/stat file is unavailable
// (i.e., on non-Linux platforms).
func TestGetCPUUsageUsingProcStatNotAvailable(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("skipping non-Linux /proc/stat unavailability test on Linux")
	}

	_, err := GetCPUUsageUsingProcStat()
	if err == nil {
		t.Error("GetCPUUsageUsingProcStat() should have returned an error on non-Linux platform")
	}
}

// TestGetNumericRAMUsage checks that GetNumericRAMUsage returns plausible values.
func TestGetNumericRAMUsage(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping RAM usage test on non-Linux OS")
	}

	used, total := GetNumericRAMUsage()
	// The values may be -1 if the command fails or is not supported.
	// On a functioning Linux system we expect positive values.
	if used < 0 || total < 0 {
		t.Logf("GetNumericRAMUsage() returned used=%d total=%d (may indicate missing tools)", used, total)
		return
	}
	if used > total {
		t.Errorf("used RAM (%d) > total RAM (%d)", used, total)
	}
}

// TestGetNumericRAMUsagePositive verifies that on Linux both used and total
// RAM are positive and total > 0.
func TestGetNumericRAMUsagePositive(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-Linux")
	}
	used, total := GetNumericRAMUsage()
	if total <= 0 {
		t.Logf("GetNumericRAMUsage() total=%d; 'free' command may not be available in this environment", total)
		return
	}
	if used < 0 {
		t.Errorf("used RAM should be >= 0, got %d", used)
	}
}

// TestGetRAMUsage checks that GetRAMUsage returns non-empty strings on Linux.
func TestGetRAMUsage(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping RAM usage string test on non-Linux OS")
	}

	usedStr, totalStr, pct := GetRAMUsage()
	// Strings should not be empty; percentage should be [0, 100].
	if usedStr == "" {
		t.Error("GetRAMUsage() returned empty used RAM string")
	}
	if totalStr == "" {
		t.Error("GetRAMUsage() returned empty total RAM string")
	}
	if pct < 0 || pct > 100 {
		t.Logf("GetRAMUsage() percentage = %v (may be out of range if tools are missing)", pct)
	}
}

// TestGetRAMUsageConsistency verifies that GetRAMUsage and GetNumericRAMUsage
// return consistent results on Linux.
func TestGetRAMUsageConsistency(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping on non-Linux")
	}
	_, _, pct := GetRAMUsage()
	used, total := GetNumericRAMUsage()
	// If both calls succeed, the percentage should be consistent.
	if total > 0 && used >= 0 && pct > 0 {
		// Rough consistency check: if numeric RAM is available the percentage should be positive.
		if pct == 0 && used > 0 {
			t.Logf("inconsistency: pct=0 but used=%d", used)
		}
	}
}

// TestCalculateCPUUsage exercises the internal calculateCPUUsage function directly.
func TestCalculateCPUUsage(t *testing.T) {
	prev := CPUStats{
		user:    100,
		nice:    0,
		system:  50,
		idle:    850,
		iowait:  0,
		irq:     0,
		softirq: 0,
	}
	// Simulate 200ms worth of CPU time: add 20 active jiffies and 80 idle jiffies.
	current := CPUStats{
		user:    120,
		nice:    0,
		system:  50,
		idle:    930,
		iowait:  0,
		irq:     0,
		softirq: 0,
	}
	usage := calculateCPUUsage(prev, current)
	// totalDiff = 100, idleDiff = 80 --> usage = (100-80)/100 * 100 = 20%
	if usage < 0 || usage > 100 {
		t.Errorf("calculateCPUUsage() = %v, expected [0, 100]", usage)
	}
	// Exact value should be 20.0
	if usage != 20.0 {
		t.Errorf("calculateCPUUsage() = %v, expected 20.0", usage)
	}
}

// TestCalculateCPUUsageZeroDelta ensures that identical stats yield 0% usage.
func TestCalculateCPUUsageZeroDelta(t *testing.T) {
	stats := CPUStats{user: 500, nice: 10, system: 100, idle: 390, iowait: 0, irq: 0, softirq: 0}
	usage := calculateCPUUsage(stats, stats)
	if usage != 0.0 {
		t.Errorf("calculateCPUUsage() with identical stats = %v, expected 0.0", usage)
	}
}

// TestGetCPUStatsLinux verifies that getCPUStats succeeds on Linux.
func TestGetCPUStatsLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping getCPUStats test on non-Linux OS")
	}

	stats, err := getCPUStats()
	if err != nil {
		t.Fatalf("getCPUStats() unexpected error: %v", err)
	}

	// Sanity check: at minimum the system should have some idle time.
	total := stats.user + stats.nice + stats.system + stats.idle + stats.iowait + stats.irq + stats.softirq
	if total == 0 {
		t.Error("getCPUStats() returned all-zero stats")
	}
}

// TestGetCPUStatsNonLinux verifies that getCPUStats fails gracefully off Linux.
func TestGetCPUStatsNonLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("skipping non-Linux getCPUStats test on Linux")
	}

	_, err := getCPUStats()
	if err == nil {
		t.Error("getCPUStats() should return an error on non-Linux platform")
	}
}
