package filemanager

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

// isValidFilename checks if a given filename is safe and valid.
func isValidFilename(filename string) bool {
	// Define a list of disallowed characters and reserved names
	disallowedChars := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}                                                                                                                            // Add more if needed
	reservedNames := []string{"CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9"} // Add more if needed

	// Check for disallowed characters
	for _, char := range disallowedChars {
		if strings.Contains(filename, char) {
			return false
		}
	}

	// Check for reserved names (case-insensitive)
	lowerFilename := strings.ToUpper(filename)
	for _, reserved := range reservedNames {
		if lowerFilename == reserved {
			return false
		}
	}

	// Check for empty filename
	if filename == "" {
		return false
	}

	// The filename is considered valid
	return true
}

// sanitizeFilename sanitizes a given filename by removing disallowed characters.
func sanitizeFilename(filename string) string {
	disallowedChars := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"} // Add more if needed

	// Replace disallowed characters with underscores
	for _, char := range disallowedChars {
		filename = strings.ReplaceAll(filename, char, "_")
	}

	return filename
}

// copyFile copies a single file from source to destination
func copyFile(srcPath, destPath string) error {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	destFile, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, srcFile)
	if err != nil {
		return err
	}

	return nil
}

// copyDirectory recursively copies a directory and its contents from source to destination
func copyDirectory(srcPath, destPath string) error {
	// Create the destination directory
	err := os.MkdirAll(destPath, os.ModePerm)
	if err != nil {
		return err
	}

	entries, err := os.ReadDir(srcPath)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcEntryPath := filepath.Join(srcPath, entry.Name())
		destEntryPath := filepath.Join(destPath, entry.Name())

		if entry.IsDir() {
			err := copyDirectory(srcEntryPath, destEntryPath)
			if err != nil {
				return err
			}
		} else {
			err := copyFile(srcEntryPath, destEntryPath)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// isDir checks if the given path is a directory
func isDir(path string) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fileInfo.IsDir()
}

// calculateDirectorySize calculates the total size of a directory and its contents
func calculateDirectorySize(dirPath string) (int64, error) {
	var totalSize int64
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		totalSize += info.Size()
		return nil
	})
	if err != nil {
		return 0, err
	}
	return totalSize, nil
}

// countSubFilesAndFolders counts the number of sub-files and sub-folders within a directory
func countSubFilesAndFolders(dirPath string) (int, int, error) {
	var numSubFiles, numSubFolders int

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			numSubFolders++
		} else {
			numSubFiles++
		}

		return nil
	})

	if err != nil {
		return 0, 0, err
	}

	// Subtract 1 from numSubFolders to exclude the root directory itself
	return numSubFiles, numSubFolders - 1, nil
}
