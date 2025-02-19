package plugins

import (
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"

	"imuslab.com/zoraxy/mod/netutils"
)

/*
Check if the folder contains a valid plugin in either one of the forms

1. Contain a file that have the same name as its parent directory, either executable or .exe on Windows
2. Contain a start.sh or start.bat file

Return the path of the plugin entry point if found
*/
func (m *Manager) GetPluginEntryPoint(folderpath string) (string, error) {
	info, err := os.Stat(folderpath)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("path is not a directory")
	}
	expectedBinaryPath := filepath.Join(folderpath, filepath.Base(folderpath))
	if runtime.GOOS == "windows" {
		expectedBinaryPath += ".exe"
	}

	if _, err := os.Stat(expectedBinaryPath); err == nil {
		return expectedBinaryPath, nil
	}

	if _, err := os.Stat(filepath.Join(folderpath, "start.sh")); err == nil {
		return filepath.Join(folderpath, "start.sh"), nil
	}

	if _, err := os.Stat(filepath.Join(folderpath, "start.bat")); err == nil {
		return filepath.Join(folderpath, "start.bat"), nil
	}

	return "", errors.New("No valid entry point found")
}

// Log logs a message with an optional error
func (m *Manager) Log(message string, err error) {
	m.Options.Logger.PrintAndLog("plugin-manager", message, err)
}

// getRandomPortNumber generates a random port number between 49152 and 65535
func getRandomPortNumber() int {
	portNo := rand.Intn(65535-49152) + 49152
	//Check if the port is already in use
	for netutils.CheckIfPortOccupied(portNo) {
		portNo = rand.Intn(65535-49152) + 49152
	}
	return portNo
}
