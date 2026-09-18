package logger

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupOldBackupsMatchesOlderMonthlyLogs(t *testing.T) {
	tempDir := t.TempDir()
	currentLog := filepath.Join(tempDir, "zr_2026-9.log")

	logger := &Logger{
		Prefix:         "zr",
		CurrentLogFile: currentLog,
		RotateOption: &RotateOption{
			Enabled:    true,
			MaxBackups: 2,
		},
	}

	writeTestFile(t, currentLog, time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC))
	oldestBackup := filepath.Join(tempDir, "zr_2026-7.log.20260701-010101.log.gz")
	middleBackup := filepath.Join(tempDir, "zr_2026-8.log.20260801-010101.log.gz")
	newestBackup := filepath.Join(tempDir, "zr_2026-9.log.20260901-010101.log.gz")
	otherPrefix := filepath.Join(tempDir, "access_2026-7.log.20260701-010101.log.gz")

	writeTestFile(t, oldestBackup, time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC))
	writeTestFile(t, middleBackup, time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC))
	writeTestFile(t, newestBackup, time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC))
	writeTestFile(t, otherPrefix, time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC))

	if err := logger.cleanupOldBackups(tempDir, currentLog); err != nil {
		t.Fatalf("cleanupOldBackups returned error: %v", err)
	}

	assertFileExists(t, currentLog)
	assertFileMissing(t, oldestBackup)
	assertFileExists(t, middleBackup)
	assertFileExists(t, newestBackup)
	assertFileExists(t, otherPrefix)

	remaining, err := filepath.Glob(filepath.Join(tempDir, "zr_*.log*"))
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	if len(remaining) != 3 {
		t.Fatalf("expected current log plus two backups, got %d entries: %v", len(remaining), remaining)
	}
}

func writeTestFile(t *testing.T, filename string, modTime time.Time) {
	t.Helper()

	if err := os.WriteFile(filename, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", filename, err)
	}
	if err := os.Chtimes(filename, modTime, modTime); err != nil {
		t.Fatalf("failed to set time on %s: %v", filename, err)
	}
}

func assertFileExists(t *testing.T, filename string) {
	t.Helper()

	if _, err := os.Stat(filename); err != nil {
		t.Fatalf("expected %s to exist: %v", filename, err)
	}
}

func assertFileMissing(t *testing.T, filename string) {
	t.Helper()

	if _, err := os.Stat(filename); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be removed, got err=%v", filename, err)
	}
}
