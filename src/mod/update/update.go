package update

/*
	Update.go

	This module handle cross version updates that contains breaking changes
	update command should always exit after the update is completed
*/

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"imuslab.com/zoraxy/mod/utils"
)

// Run config update. Version numbers are int. For example
// to update 3.0.7 to 3.0.8, use RunConfigUpdate(307, 308)
// This function support cross versions updates (e.g. 307 -> 310)
func RunConfigUpdate(fromVersion int, toVersion int) {
	versionFile := "./conf/version"
	isFirstTimeInit, _ := isFirstTimeInitialize("./conf/proxy/")
	if isFirstTimeInit {
		//Create version file and exit
		os.MkdirAll("./conf/", 0775)
		os.WriteFile(versionFile, []byte(strconv.Itoa(toVersion)), 0775)
		return
	}
	if fromVersion == 0 {
		//Run auto previous version detection
		fromVersion = 307
		if utils.FileExists(versionFile) {
			//Read the version file
			previousVersionText, err := os.ReadFile(versionFile)
			if err != nil {
				panic("Unable to read version file at " + versionFile)
			}

			//Convert the version to int
			versionInt, err := strconv.Atoi(strings.TrimSpace(string(previousVersionText)))
			if err != nil {
				panic("Unable to read version file at " + versionFile)
			}

			fromVersion = versionInt
		}

		if fromVersion == toVersion {
			//No need to update
			return
		}
	}

	//Do iterate update
	for i := fromVersion; i < toVersion; i++ {
		oldVersion := fromVersion
		newVersion := fromVersion + 1
		fmt.Println("Updating from v", oldVersion, " to v", newVersion)
		runUpdateRoutineWithVersion(oldVersion, newVersion)
		//Write the updated version to file
		os.WriteFile(versionFile, []byte(strconv.Itoa(newVersion)), 0775)
	}
	fmt.Println("Update completed")
}

func GetVersionIntFromVersionNumber(version string) int {
	versionNumberOnly := strings.ReplaceAll(version, ".", "")
	versionInt, _ := strconv.Atoi(versionNumberOnly)
	return versionInt
}

// Check if the folder "./conf/proxy/" exists and contains files
func isFirstTimeInitialize(path string) (bool, error) {
	// Check if the folder exists
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		// The folder does not exist
		return true, nil
	}
	if err != nil {
		// Some other error occurred
		return false, err
	}

	// Check if it is a directory
	if !info.IsDir() {
		// The path is not a directory
		return false, fmt.Errorf("%s is not a directory", path)
	}

	// Read the directory contents
	files, err := ioutil.ReadDir(path)
	if err != nil {
		return false, err
	}

	// Check if the directory is empty
	if len(files) == 0 {
		// The folder exists but is empty
		return true, nil
	}

	// The folder exists and contains files
	return false, nil
}
