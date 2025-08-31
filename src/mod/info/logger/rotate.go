package logger

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type RotateOption struct {
	Enabled    bool   //Whether log rotation is enabled
	MaxSize    int64  //Maximum size of the log file in bytes before rotation (e.g. 10 * 1024 * 1024 for 10MB)
	MaxBackups int    //Maximum number of backup files to keep
	Compress   bool   //Whether to compress the rotated files
	BackupDir  string //Directory to store backup files, if empty, use the same directory as the log file
}

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

func (l *Logger) RotateLog() error {
	if !l.RotateOption.Enabled {
		return nil
	}

	needRotate := l.LogNeedRotate(l.CurrentLogFile)
	if !needRotate {
		return nil
	}

	//Close current file
	if l.file != nil {
		l.file.Close()
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
	rotatedName := fmt.Sprintf("%s.%s", baseName, timestamp)
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

	return nil
}

// compressFile compresses the given file using zip format and creates a .gz file.
func compressFile(filename string) error {
	zipFilename := filename + ".gz"
	outFile, err := os.Create(zipFilename)
	if err != nil {
		return err
	}
	defer outFile.Close()

	zipWriter := zip.NewWriter(outFile)
	defer zipWriter.Close()

	fileToCompress, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer fileToCompress.Close()

	w, err := zipWriter.Create(filepath.Base(filename))
	if err != nil {
		return err
	}

	_, err = io.Copy(w, fileToCompress)
	return err
}
