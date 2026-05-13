package updateutil

import (
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ConfigFolder is the base folder for Zoraxy config data used by update routines.
// Defaults to ./conf to preserve existing behavior.
var ConfigFolder = "./conf"

// SetConfigFolder overrides the base config folder used by update routines.
func SetConfigFolder(path string) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		ConfigFolder = "./conf"
		return
	}
	ConfigFolder = filepath.Clean(trimmed)
}

// ConfigPath joins the config folder with the provided path parts.
func ConfigPath(parts ...string) string {
	if len(parts) == 0 {
		return ConfigFolder
	}
	items := append([]string{ConfigFolder}, parts...)
	return filepath.Join(items...)
}

// Helper function to copy files
func CopyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destinationFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destinationFile.Close()

	_, err = io.Copy(destinationFile, sourceFile)
	return err
}
