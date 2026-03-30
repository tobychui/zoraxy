package logger

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"imuslab.com/zoraxy/mod/utils"
)

type RotateOption struct {
	Enabled    bool   //Whether log rotation is enabled, default false
	MaxSize    int64  //Maximum size of the log file in bytes before rotation (e.g. 10 * 1024 * 1024 for 10MB)
	MaxBackups int    //Maximum number of backup files to keep
	Compress   bool   //Whether to compress the rotated files
	BackupDir  string //Directory to store backup files, if empty, use the same directory as the log file
}

// Stop the log rotation ticker
func (l *Logger) StopLogRotateTicker() {
	if l.logRotateTicker != nil {
		l.logRotateTicker.Stop()
	}
}

// Check if the log file needs rotation
func (l *Logger) LogNeedRotate(filename string) bool {
	if !l.RotateOption.Enabled {
		return false
	}
	info, err := os.Stat(filename)
	if err != nil {
		return false
	}
	return info.Size() >= l.RotateOption.MaxSize
}

// Handle web request trigger log ratation
func (l *Logger) HandleDebugTriggerLogRotation(w http.ResponseWriter, r *http.Request) {
	err := l.RotateLog()
	if err != nil {
		utils.SendErrorResponse(w, "Log rotation error: "+err.Error())
		return
	}
	l.PrintAndLog("logger", "Log rotation triggered via REST API", nil)
	utils.SendOK(w)
}

// ArchiveLog will archive the given log file, use during month change
func (l *Logger) ArchiveLog(filename string) error {
	if l.RotateOption == nil || !l.RotateOption.Enabled {
		return nil
	}

	// Determine backup directory
	backupDir := l.RotateOption.BackupDir
	if backupDir == "" {
		backupDir = filepath.Dir(filename)
	}

	// Ensure backup directory exists
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return err
	}

	// Generate archived filename with timestamp
	timestamp := time.Now().Format("20060102-150405")
	baseName := filepath.Base(filename)
	baseName = baseName[:len(baseName)-len(filepath.Ext(baseName))]
	archivedName := fmt.Sprintf("%s.%s.log", baseName, timestamp)
	archivedPath := filepath.Join(backupDir, archivedName)

	// Rename current log file to archived file
	if err := os.Rename(filename, archivedPath); err != nil {
		return err
	}

	// Optionally compress the archived file
	if l.RotateOption.Compress {
		if err := compressFile(archivedPath); err != nil {
			return err
		}
		os.Remove(archivedPath)
	}

	return nil
}

// Execute log rotation
func (l *Logger) RotateLog() error {
	if l.RotateOption == nil || !l.RotateOption.Enabled {
		return nil
	}

	needRotate := l.LogNeedRotate(l.CurrentLogFile)
	l.PrintAndLog("logger", fmt.Sprintf("Log rotation check: need rotate = %v", needRotate), nil)
	if !needRotate {
		return nil
	}

	// Close current file with retry on failure
	if l.file != nil {
		var closeErr error
		for i := 0; i < 5; i++ {
			closeErr = l.file.Close()
			if closeErr == nil {
				break
			}
			time.Sleep(1 * time.Second)
		}
		if closeErr != nil {
			return closeErr
		}
	}

	// Determine backup directory
	backupDir := l.RotateOption.BackupDir
	if backupDir == "" {
		backupDir = filepath.Dir(l.CurrentLogFile)
	}

	// Ensure backup directory exists
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return err
	}

	// Generate rotated filename with timestamp
	timestamp := time.Now().Format("20060102-150405")
	baseName := filepath.Base(l.CurrentLogFile)
	baseName = baseName[:len(baseName)-len(filepath.Ext(baseName))]
	rotatedName := fmt.Sprintf("%s.%s.log", baseName, timestamp)
	rotatedPath := filepath.Join(backupDir, rotatedName)

	// Rename current log file to rotated file
	if err := os.Rename(l.CurrentLogFile, rotatedPath); err != nil {
		return err
	}

	// Optionally compress the rotated file
	if l.RotateOption.Compress {
		if err := compressFile(rotatedPath); err != nil {
			return err
		}
		// Remove the uncompressed rotated file after compression
		os.Remove(rotatedPath)
		rotatedPath += ".gz"
	}

	// Remove old backups if exceeding MaxBackups
	if l.RotateOption.MaxBackups > 0 {
		files, err := filepath.Glob(filepath.Join(backupDir, baseName+".*"))
		if err == nil && len(files) > l.RotateOption.MaxBackups {
			sort.Slice(files, func(i, j int) bool {
				return files[i] < files[j]
			})
			for _, old := range files[:len(files)-l.RotateOption.MaxBackups] {
				os.Remove(old)
			}
		}
	}

	// Reopen a new log file
	file, err := os.OpenFile(l.CurrentLogFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	l.file = file
	if l.logger != nil {
		l.logger.SetOutput(file)
	}
	return nil
}

// compressFile compresses the given file using gzip format and creates a .gz file.
func compressFile(filename string) error {
	gzipFilename := filename + ".gz"
	outFile, err := os.Create(gzipFilename)
	if err != nil {
		return err
	}
	defer outFile.Close()

	gzipWriter := gzip.NewWriter(outFile)
	defer gzipWriter.Close()

	fileToCompress, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer fileToCompress.Close()

	_, err = io.Copy(gzipWriter, fileToCompress)
	return err
}
